//go:build e2e

package e2e

import (
    "context"
    "database/sql"
    "fmt"
    "log/slog"
    "os"
    "testing"
    "time"

    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"

    msql "toggl-scraper/internal/adapter/mysql"
    "toggl-scraper/internal/domain"
    "toggl-scraper/internal/ports"
    "toggl-scraper/internal/migrate"
    "toggl-scraper/internal/usecase"
)

type fakeToggl struct{ entries []domain.TimeEntry }

func (f fakeToggl) ListTimeEntries(ctx context.Context, from, to time.Time) ([]domain.TimeEntry, error) {
    return f.entries, nil
}

func TestSyncToMySQL_UpsertsEntries(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping in short mode")
    }
    ctx := context.Background()

    // Start MySQL container
    req := testcontainers.ContainerRequest{
        Image:        "mysql:8.0",
        ExposedPorts: []string{"3306/tcp"},
        Env: map[string]string{
            "MYSQL_DATABASE":      "testdb",
            "MYSQL_ROOT_PASSWORD": "secret",
            "MYSQL_USER":          "test",
            "MYSQL_PASSWORD":      "pass",
        },
        WaitingFor: wait.ForListeningPort("3306/tcp").WithStartupTimeout(90 * time.Second),
    }
    mysqlC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    if err != nil {
        t.Fatalf("failed to start mysql container: %v", err)
    }
    t.Cleanup(func() { _ = mysqlC.Terminate(context.Background()) })

    host, err := mysqlC.Host(ctx)
    if err != nil {
        t.Fatalf("host: %v", err)
    }
    port, err := mysqlC.MappedPort(ctx, "3306/tcp")
    if err != nil {
        t.Fatalf("mapped port: %v", err)
    }
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true", "test", "pass", host, port.Port(), "testdb")

    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
    if err := migrate.Run(ctx, dsn, logger); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    sink, err := msql.NewClient(ctx, dsn, logger)
    if err != nil {
        t.Fatalf("mysql client: %v", err)
    }
    t.Cleanup(func() { _ = sink.Close() })

    // Prepare fake entries
    start := time.Date(2025, 8, 1, 9, 0, 0, 0, time.UTC)
    stop := start.Add(90 * time.Minute)
    projectID := int64(123)
    workspaceID := int64(456)
    fake := fakeToggl{entries: []domain.TimeEntry{
        {ID: 1, Description: "Dev work", ProjectID: &projectID, WorkspaceID: &workspaceID, Tags: []string{"dev", "feature"}, Start: start, Stop: &stop, DurationSec: 5400},
        {ID: 2, Description: "Meeting", ProjectID: nil, WorkspaceID: &workspaceID, Tags: []string{"meeting"}, Start: start.Add(2 * time.Hour), Stop: &stop, DurationSec: 3600},
    }}

    uc := &usecase.SyncUseCase{Log: logger, Toggl: ports.TogglClient(fake), Sink: sink}
    if err := uc.Run(ctx, start.Add(-time.Hour), start.Add(4*time.Hour)); err != nil {
        t.Fatalf("sync run: %v", err)
    }

    // Verify rows
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        t.Fatalf("sql open: %v", err)
    }
    defer db.Close()

    var count int
    if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM toggl_time_entries").Scan(&count); err != nil {
        t.Fatalf("count: %v", err)
    }
    if count != 2 {
        t.Fatalf("expected 2 rows, got %d", count)
    }

    // Run again to assert idempotency (upsert)
    if err := uc.Run(ctx, start.Add(-time.Hour), start.Add(4*time.Hour)); err != nil {
        t.Fatalf("sync run 2: %v", err)
    }
    if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM toggl_time_entries").Scan(&count); err != nil {
        t.Fatalf("count 2: %v", err)
    }
    if count != 2 {
        t.Fatalf("expected 2 rows after upsert, got %d", count)
    }
}
