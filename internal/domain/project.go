package domain

import "time"

// Project represents a Toggl project in the domain layer.
type Project struct {
	ID          int64
	WorkspaceID int64
	Name        string
	Active      bool
	Private     bool
	Color       string
	ClientID    *int64
	At          time.Time // Last update timestamp from Toggl
}
