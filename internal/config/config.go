package config

import (
    "errors"
    "os"
    "strconv"
)

// Config holds environment-driven configuration.
type Config struct {
    Toggl struct {
        APIToken    string
        WorkspaceID int64
        BaseURL     string // default: https://api.track.toggl.com
    }
    MySQL struct {
        DSN string // e.g., user:pass@tcp(host:3306)/dbname?parseTime=true&multiStatements=true
    }
    Sync struct {
        Timezone string // e.g., UTC (default), Europe/Berlin
    }
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
    var cfg Config

    cfg.Toggl.APIToken = os.Getenv("TOGGL_API_TOKEN")
    if cfg.Toggl.APIToken == "" {
        return cfg, errors.New("TOGGL_API_TOKEN is required")
    }
    if ws := os.Getenv("TOGGL_WORKSPACE_ID"); ws != "" {
        if v, err := strconv.ParseInt(ws, 10, 64); err == nil {
            cfg.Toggl.WorkspaceID = v
        } else {
            return cfg, errors.New("TOGGL_WORKSPACE_ID must be an integer")
        }
    }
    cfg.Toggl.BaseURL = os.Getenv("TOGGL_BASE_URL")
    if cfg.Toggl.BaseURL == "" {
        cfg.Toggl.BaseURL = "https://api.track.toggl.com"
    }

    cfg.MySQL.DSN = os.Getenv("MYSQL_DSN")

    cfg.Sync.Timezone = os.Getenv("SYNC_TZ")
    if cfg.Sync.Timezone == "" {
        cfg.Sync.Timezone = "UTC"
    }

    return cfg, nil
}
