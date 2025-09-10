package app

import (
    "context"
    "encoding/json"
    "log/slog"
    "net/http"
    "time"
)

// HTTPServer returns a configured http.Server that exposes endpoints to trigger syncs.
// Call ListenAndServe on the returned server in a goroutine and Shutdown it on exit.
func (a *App) HTTPServer(addr string) *http.Server {
    mux := http.NewServeMux()

    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })

    // /sync?from=...&to=...
    // from/to accept RFC3339 or YYYY-MM-DD. If omitted, defaults to [now-24h, now].
    mux.HandleFunc("/sync", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet && r.Method != http.MethodPost {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }

        q := r.URL.Query()
        now := time.Now().UTC()
        toStr := q.Get("to")
        fromStr := q.Get("from")

        toTime := parseEndHTTP(toStr, now)
        fromTime := parseStartHTTP(fromStr, toTime.Add(-24*time.Hour))

        // Optional timeout override: ?timeout=5m
        ctx := r.Context()
        if tStr := q.Get("timeout"); tStr != "" {
            if d, err := time.ParseDuration(tStr); err == nil && d > 0 {
                var cancel func()
                ctx, cancel = context.WithTimeout(ctx, d)
                defer cancel()
            }
        }

        // Run sync
        err := a.RunOnce(ctx, fromTime, toTime)
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        if err != nil {
            // Distinguish concurrent run from other errors by message
            status := http.StatusInternalServerError
            if err.Error() == "sync already running" {
                status = http.StatusConflict
            }
            w.WriteHeader(status)
            _ = json.NewEncoder(w).Encode(map[string]any{
                "status": "error",
                "error":  err.Error(),
                "from":   fromTime.Format(time.RFC3339),
                "to":     toTime.Format(time.RFC3339),
            })
            return
        }
        w.WriteHeader(http.StatusOK)
        _ = json.NewEncoder(w).Encode(map[string]any{
            "status": "ok",
            "from":   fromTime.Format(time.RFC3339),
            "to":     toTime.Format(time.RFC3339),
        })
    })

    srv := &http.Server{Addr: addr, Handler: loggingMiddleware(a.log, mux)}
    a.log.Info("http trigger server configured", slog.String("addr", addr))
    return srv
}

// loggingMiddleware provides basic request logging.
func loggingMiddleware(log *slog.Logger, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        log.Info("http request",
            slog.String("method", r.Method),
            slog.String("path", r.URL.Path),
            slog.String("remote", r.RemoteAddr),
            slog.Duration("dur", time.Since(start)),
        )
    })
}

// parseStartHTTP parses a start boundary that may be RFC3339 or YYYY-MM-DD.
// If empty, defaultVal is returned.
func parseStartHTTP(val string, defaultVal time.Time) time.Time {
    if val == "" {
        return defaultVal
    }
    if t, err := time.Parse(time.RFC3339, val); err == nil {
        return t
    }
    if d, err := time.Parse("2006-01-02", val); err == nil {
        return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
    }
    // On invalid input, fall back to default to avoid hard failures.
    return defaultVal
}

// parseEndHTTP parses an end boundary that may be RFC3339 or YYYY-MM-DD.
// Date-only form is treated as inclusive by converting to next-day 00:00 UTC.
// If empty, defaultVal is returned.
func parseEndHTTP(val string, defaultVal time.Time) time.Time {
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
    return defaultVal
}
