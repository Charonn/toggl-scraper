package migrate

import (
    "context"
    "database/sql"
    "embed"
    "fmt"
    "io/fs"
    "log/slog"
    "path/filepath"
    "sort"
    "strconv"
    "strings"
    "time"

    _ "github.com/go-sql-driver/mysql"
)

//go:embed sql/*.sql
var migrationsFS embed.FS

// Run applies pending migrations found under internal/migrate/sql.
// Migrations must be named like 0001_description.sql and will be executed
// in lexicographic order. The entire file is executed as a single statement
// batch; the MySQL DSN should include multiStatements=true.
func Run(ctx context.Context, dsn string, log *slog.Logger) error {
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        return err
    }
    defer db.Close()

    c, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    if err := db.PingContext(c); err != nil {
        return err
    }

    if err := ensureMigrationsTable(ctx, db); err != nil {
        return err
    }

    files, err := fs.Glob(migrationsFS, "sql/*.sql")
    if err != nil {
        return err
    }
    sort.Strings(files)

    applied, err := loadApplied(ctx, db)
    if err != nil {
        return err
    }

    for _, f := range files {
        base := filepath.Base(f)
        ver, err := parseVersion(base)
        if err != nil {
            return fmt.Errorf("invalid migration filename %q: %w", base, err)
        }
        if applied[ver] {
            log.Debug("migration already applied", slog.Int("version", ver), slog.String("file", base))
            continue
        }
        b, err := fs.ReadFile(migrationsFS, f)
        if err != nil {
            return err
        }
        log.Info("applying migration", slog.Int("version", ver), slog.String("file", base))
        if _, err := db.ExecContext(ctx, string(b)); err != nil {
            return fmt.Errorf("applying %s: %w", base, err)
        }
        if err := recordApplied(ctx, db, ver); err != nil {
            return err
        }
    }
    return nil
}

func ensureMigrationsTable(ctx context.Context, db *sql.DB) error {
    const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
        version BIGINT PRIMARY KEY,
        applied_at DATETIME(6) NOT NULL
    ) ENGINE=InnoDB;`
    _, err := db.ExecContext(ctx, ddl)
    return err
}

func loadApplied(ctx context.Context, db *sql.DB) (map[int]bool, error) {
    rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    m := make(map[int]bool)
    for rows.Next() {
        var v int
        if err := rows.Scan(&v); err != nil {
            return nil, err
        }
        m[v] = true
    }
    return m, rows.Err()
}

func recordApplied(ctx context.Context, db *sql.DB, version int) error {
    _, err := db.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)", version, time.Now().UTC())
    return err
}

func parseVersion(name string) (int, error) {
    // Expect prefix like 0001_...
    i := strings.IndexByte(name, '_')
    if i <= 0 {
        return 0, fmt.Errorf("missing prefix number")
    }
    v, err := strconv.Atoi(name[:i])
    if err != nil {
        return 0, err
    }
    return v, nil
}

