package main

import (
    "context"
    "flag"
    "log/slog"
    "os"
    "os/signal"
    "syscall"
    "time"

    "toggl-scraper/internal/app"
    "toggl-scraper/internal/config"
)

func main() {
    // Flags
    once := flag.Bool("once", false, "Run a single sync and exit")
    interval := flag.Duration("interval", 15*time.Minute, "Sync interval when not running once")
    daily := flag.Bool("daily", false, "Run at local midnight each day (uses SYNC_TZ, default UTC)")
    from := flag.String("from", "", "ISO8601 start time (optional, default: now - 24h)")
    to := flag.String("to", "", "ISO8601 end time (optional, default: now)")
    verbose := flag.Bool("v", false, "Enable verbose logging")
    flag.Parse()

    // Logger
    level := slog.LevelInfo
    if *verbose {
        level = slog.LevelDebug
    }
    handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
    logger := slog.New(handler)
    slog.SetDefault(logger)

    // Config
    cfg, err := config.Load()
    if err != nil {
        logger.Error("failed to load config", slog.String("error", err.Error()))
        os.Exit(1)
    }

    // Parse time window flags (accept RFC3339 or date-only YYYY-MM-DD)
    var (
        fromTime time.Time
        toTime   time.Time
    )
    now := time.Now().UTC()
    toTime = parseEnd(*to, now, logger)
    fromTime = parseStart(*from, toTime.Add(-24*time.Hour), logger)

    // App
    application, err := app.New(logger, cfg)
    if err != nil {
        logger.Error("failed to initialize app", slog.String("error", err.Error()))
        os.Exit(1)
    }

    // Context with signal handling
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    if *once {
        if err := application.RunOnce(ctx, fromTime, toTime); err != nil {
            logger.Error("sync failed", slog.String("error", err.Error()))
            os.Exit(1)
        }
        logger.Info("sync completed")
        return
    }

    // Daily-at-midnight mode (default for container)
    if *daily {
        loc, err := time.LoadLocation(cfg.Sync.Timezone)
        if err != nil {
            logger.Error("invalid SYNC_TZ", slog.String("tz", cfg.Sync.Timezone), slog.String("error", err.Error()))
            os.Exit(1)
        }
        logger.Info("starting daily sync at midnight", slog.String("tz", cfg.Sync.Timezone))
        for {
            // Compute next local midnight
            nowLoc := time.Now().In(loc)
            next := nextMidnight(nowLoc)
            dur := time.Until(next)
            logger.Info("sleeping until next midnight", slog.Time("next", next), slog.Duration("sleep", dur))
            select {
            case <-ctx.Done():
                logger.Info("shutting down")
                return
            case <-time.After(dur):
                // Define window as [midnight-24h, midnight) in local tz, expressed in UTC
                endUTC := next.UTC()
                startUTC := endUTC.Add(-24 * time.Hour)
                if err := application.RunOnce(ctx, startUTC, endUTC); err != nil {
                    logger.Error("daily sync failed", slog.String("error", err.Error()))
                } else {
                    logger.Info("daily sync completed", slog.Time("from", startUTC), slog.Time("to", endUTC))
                }
            }
        }
    }

    // Periodic mode (legacy)
    ticker := time.NewTicker(*interval)
    defer ticker.Stop()
    logger.Info("starting periodic sync", slog.Duration("interval", *interval))
    // Kick off immediately
    if err := application.RunOnce(ctx, fromTime, toTime); err != nil {
        logger.Error("initial sync failed", slog.String("error", err.Error()))
    }
    for {
        select {
        case <-ctx.Done():
            logger.Info("shutting down")
            return
        case <-ticker.C:
            end := time.Now().UTC()
            start := end.Add(-24 * time.Hour)
            if err := application.RunOnce(ctx, start, end); err != nil {
                logger.Error("periodic sync failed", slog.String("error", err.Error()))
            }
        }
    }
}

// parseStart parses a start boundary that may be RFC3339 or YYYY-MM-DD.
// If empty, defaultVal is returned.
func parseStart(val string, defaultVal time.Time, log *slog.Logger) time.Time {
    if val == "" {
        return defaultVal
    }
    if t, err := time.Parse(time.RFC3339, val); err == nil {
        return t
    }
    // Try date-only in UTC at 00:00
    if d, err := time.Parse("2006-01-02", val); err == nil {
        return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
    }
    log.Error("invalid --from, expected RFC3339 or YYYY-MM-DD")
    os.Exit(1)
    return time.Time{}
}

// parseEnd parses an end boundary that may be RFC3339 or YYYY-MM-DD.
// Date-only form is treated as inclusive by converting to next-day 00:00 UTC.
// If empty, defaultVal is returned.
func parseEnd(val string, defaultVal time.Time, log *slog.Logger) time.Time {
    if val == "" {
        return defaultVal
    }
    if t, err := time.Parse(time.RFC3339, val); err == nil {
        return t
    }
    if d, err := time.Parse("2006-01-02", val); err == nil {
        next := d.Add(24 * time.Hour)
        return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, time.UTC)
    }
    log.Error("invalid --to, expected RFC3339 or YYYY-MM-DD")
    os.Exit(1)
    return time.Time{}
}

// nextMidnight returns the next midnight after t in t's location.
func nextMidnight(t time.Time) time.Time {
    y, m, d := t.Date()
    // If t is already midnight, schedule for the next day to avoid double run
    midnight := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
    if !t.After(midnight) {
        // t is <= midnight of today; schedule to today's midnight if strictly after, otherwise next day
        if t.Equal(midnight) {
            return midnight.Add(24 * time.Hour)
        }
        return midnight
    }
    return midnight.Add(24 * time.Hour)
}
