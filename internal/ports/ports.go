package ports

import (
	"context"
	"time"

	"toggl-scraper/internal/domain"
)

// TogglClient defines methods to fetch time entries from Toggl.
type TogglClient interface {
	ListTimeEntries(ctx context.Context, from, to time.Time) ([]domain.TimeEntry, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
}

// Sink receives entries and persists them to a target system.
// In this project, the primary target is Metabase-adjacent storage, but the
// interface is intentionally generic to support other sinks.
type Sink interface {
	SyncEntries(ctx context.Context, entries []domain.TimeEntry) error
	SyncProjects(ctx context.Context, projects []domain.Project) error
}
