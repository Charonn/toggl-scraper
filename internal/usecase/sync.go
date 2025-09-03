package usecase

import (
    "context"
    "errors"
    "log/slog"
    "time"

    "toggl-scraper/internal/ports"
)

// SyncUseCase coordinates fetching from Toggl and syncing to a Sink.
type SyncUseCase struct {
    Log   *slog.Logger
    Toggl ports.TogglClient
    Sink  ports.Sink
}

func (uc *SyncUseCase) Run(ctx context.Context, from, to time.Time) error {
    if uc.Toggl == nil || uc.Sink == nil {
        return errors.New("usecase not initialized: missing dependencies")
    }
    uc.Log.Info("fetching time entries", slog.Time("from", from), slog.Time("to", to))

    entries, err := uc.Toggl.ListTimeEntries(ctx, from, to)
    if err != nil {
        return err
    }
    uc.Log.Info("fetched time entries", slog.Int("count", len(entries)))

    if len(entries) == 0 {
        uc.Log.Info("no entries to sync")
        return nil
    }

    if err := uc.Sink.SyncEntries(ctx, entries); err != nil {
        return err
    }
    uc.Log.Info("sync completed", slog.Int("count", len(entries)))
    return nil
}

