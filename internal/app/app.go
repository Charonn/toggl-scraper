package app

import (
    "context"
    "log/slog"
    "time"

    msql "toggl-scraper/internal/adapter/mysql"
    tg "toggl-scraper/internal/adapter/toggl"
    "toggl-scraper/internal/config"
    "toggl-scraper/internal/migrate"
    "toggl-scraper/internal/usecase"
)

// App wires adapters and use cases.
type App struct {
    log  *slog.Logger
    uc   *usecase.SyncUseCase
}

func New(log *slog.Logger, cfg config.Config) (*App, error) {
    togglClient := tg.NewClient(cfg.Toggl.BaseURL, cfg.Toggl.APIToken, cfg.Toggl.WorkspaceID, log)
    // Run migrations before opening the sink for use
    if err := migrate.Run(context.Background(), cfg.MySQL.DSN, log); err != nil {
        return nil, err
    }
    sink, err := msql.NewClient(context.Background(), cfg.MySQL.DSN, log)
    if err != nil {
        return nil, err
    }

    uc := &usecase.SyncUseCase{
        Log:   log,
        Toggl: togglClient,
        Sink:  sink,
    }

    return &App{log: log, uc: uc}, nil
}

func (a *App) RunOnce(ctx context.Context, from, to time.Time) error {
    return a.uc.Run(ctx, from, to)
}
