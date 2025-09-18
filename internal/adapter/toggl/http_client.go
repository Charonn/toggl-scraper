package toggl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"toggl-scraper/internal/domain"
)

// Client implements ports.TogglClient using the Toggl Track API v9.
type Client struct {
	baseURL   string
	apiToken  string
	http      *http.Client
	workspace int64
	log       *slog.Logger
}

func NewClient(baseURL, apiToken string, workspaceID int64, log *slog.Logger) *Client {
	if baseURL == "" {
		baseURL = "https://api.track.toggl.com"
	}
	return &Client{
		baseURL:   baseURL,
		apiToken:  apiToken,
		workspace: workspaceID,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		log: log,
	}
}

// ListTimeEntries fetches entries in [from, to].
// Toggl v9: GET /api/v9/me/time_entries?start_date=...&end_date=...
func (c *Client) ListTimeEntries(ctx context.Context, from, to time.Time) ([]domain.TimeEntry, error) {
	if c.apiToken == "" {
		return nil, errors.New("missing api token")
	}
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/v9/me/time_entries"
	q := u.Query()
	q.Set("start_date", from.Format(time.RFC3339))
	q.Set("end_date", to.Format(time.RFC3339))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	// Basic auth: token:api_token
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.apiToken, "api_token")))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("toggl: unexpected status %d: %s", resp.StatusCode, string(body))
	}
	dec := json.NewDecoder(resp.Body)
	var raw []rawTimeEntry
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	// Map to domain
	out := make([]domain.TimeEntry, 0, len(raw))
	for _, r := range raw {
		var stopPtr *time.Time
		if r.Stop != nil {
			stop := *r.Stop
			stopPtr = &stop
		}
		var projectPtr *int64
		if r.ProjectID != nil {
			p := *r.ProjectID
			projectPtr = &p
		}
		var wsPtr *int64
		if r.WorkspaceID != nil {
			w := *r.WorkspaceID
			wsPtr = &w
		}
		out = append(out, domain.TimeEntry{
			ID:          r.ID,
			Description: r.Description,
			ProjectID:   projectPtr,
			WorkspaceID: wsPtr,
			Tags:        r.Tags,
			Start:       r.Start,
			Stop:        stopPtr,
			DurationSec: r.Duration,
		})
	}
	return out, nil
}

// ListProjects fetches projects accessible to the configured token.
// If a workspace ID is configured, it scopes the request to that workspace.
func (c *Client) ListProjects(ctx context.Context) ([]domain.Project, error) {
	if c.apiToken == "" {
		return nil, errors.New("missing api token")
	}
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	if c.workspace != 0 {
		u.Path = fmt.Sprintf("/api/v9/workspaces/%d/projects", c.workspace)
	} else {
		u.Path = "/api/v9/me/projects"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.apiToken, "api_token")))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("toggl: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	dec := json.NewDecoder(resp.Body)
	var raw []rawProject
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}

	out := make([]domain.Project, 0, len(raw))
	for _, p := range raw {
		var clientID *int64
		if p.ClientID != nil {
			id := *p.ClientID
			clientID = &id
		}
		out = append(out, domain.Project{
			ID:          p.ID,
			WorkspaceID: p.WorkspaceID,
			Name:        p.Name,
			Active:      p.Active,
			Private:     p.Private,
			Color:       p.Color,
			ClientID:    clientID,
			At:          p.At,
		})
	}
	return out, nil
}

// rawTimeEntry mirrors the JSON from Toggl v9.
type rawTimeEntry struct {
	ID          int64      `json:"id"`
	Description string     `json:"description"`
	ProjectID   *int64     `json:"project_id"`
	WorkspaceID *int64     `json:"workspace_id"`
	Tags        []string   `json:"tags"`
	Start       time.Time  `json:"start"`
	Stop        *time.Time `json:"stop"`
	Duration    int64      `json:"duration"`
}

type rawProject struct {
	ID          int64     `json:"id"`
	WorkspaceID int64     `json:"workspace_id"`
	Name        string    `json:"name"`
	Active      bool      `json:"active"`
	Private     bool      `json:"is_private"`
	Color       string    `json:"color"`
	ClientID    *int64    `json:"client_id"`
	At          time.Time `json:"at"`
}
