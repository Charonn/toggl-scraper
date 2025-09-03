package domain

import "time"

// TimeEntry represents a Toggl time entry in the domain.
type TimeEntry struct {
	ID          int64
	Description string
	ProjectID   *int64
	WorkspaceID *int64
	Tags        []string
	Start       time.Time
	Stop        *time.Time
	DurationSec int64 // Negative means running in Toggl API semantics
}
