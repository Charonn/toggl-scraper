package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"toggl-scraper/internal/domain"
)

// Client implements ports.Sink by writing to a MySQL table.
type Client struct {
	db  *sql.DB
	log *slog.Logger
}

// NewClient opens a MySQL connection using the provided DSN.
// Example DSN: user:pass@tcp(host:3306)/dbname?parseTime=true&multiStatements=true
func NewClient(ctx context.Context, dsn string, log *slog.Logger) (*Client, error) {
	if dsn == "" {
		return nil, errors.New("mysql: DSN is required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	// Conservative pool defaults; can be adjusted via env later.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(c); err != nil {
		db.Close()
		return nil, err
	}
	return &Client{db: db, log: log}, nil
}

// SyncEntries upserts entries into the MySQL table.
func (c *Client) SyncEntries(ctx context.Context, entries []domain.TimeEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	// Use ON DUPLICATE KEY UPDATE to perform upserts.
	const q = `
INSERT INTO toggl_time_entries
  (id, description, project_id, workspace_id, tags, start, stop, duration_sec)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  description=VALUES(description),
  project_id=VALUES(project_id),
  workspace_id=VALUES(workspace_id),
  tags=VALUES(tags),
  start=VALUES(start),
  stop=VALUES(stop),
  duration_sec=VALUES(duration_sec);
`
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		// Marshal tags as JSON for readability; stored as TEXT.
		tagsJSON, _ := json.Marshal(e.Tags)
		var project, workspace interface{}
		if e.ProjectID != nil {
			project = *e.ProjectID
		} else {
			project = nil
		}
		if e.WorkspaceID != nil {
			workspace = *e.WorkspaceID
		} else {
			workspace = nil
		}
		var stop interface{}
		if e.Stop != nil {
			stop = e.Stop.UTC()
		} else {
			stop = nil
		}
		if _, err := stmt.ExecContext(
			ctx,
			e.ID,
			e.Description,
			project,
			workspace,
			string(tagsJSON),
			e.Start.UTC(),
			stop,
			e.DurationSec,
		); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	c.log.Info("mysql sink upserted entries", slog.Int("count", len(entries)))
	return nil
}

// SyncProjects upserts projects into the MySQL table.
func (c *Client) SyncProjects(ctx context.Context, projects []domain.Project) error {
	if len(projects) == 0 {
		return nil
	}
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	const q = `
INSERT INTO toggl_projects
  (id, workspace_id, name, active, is_private, color, client_id, at)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  workspace_id=VALUES(workspace_id),
  name=VALUES(name),
  active=VALUES(active),
  is_private=VALUES(is_private),
  color=VALUES(color),
  client_id=VALUES(client_id),
  at=VALUES(at);
`
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, p := range projects {
		var client interface{}
		if p.ClientID != nil {
			client = *p.ClientID
		} else {
			client = nil
		}
		if _, err := stmt.ExecContext(
			ctx,
			p.ID,
			p.WorkspaceID,
			p.Name,
			p.Active,
			p.Private,
			p.Color,
			client,
			p.At.UTC(),
		); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	c.log.Info("mysql sink upserted projects", slog.Int("count", len(projects)))
	return nil
}

// Close closes the underlying DB. Not wired via interface to keep ports minimal.
func (c *Client) Close() error { return c.db.Close() }
