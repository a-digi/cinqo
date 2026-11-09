package chromeproc

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// LimitSessions wraps a handler that drives Chrome: the request takes a session slot before it runs (waiting up to QueueWait in line) and gives
// it back when it ends, whatever way it ends (a return, a panic, the client leaving). A request that finds no free slot in time is answered
// with 503 and `{"error":"too_many_sessions", ...}`. A request held longer than StuckAfter is cancelled by the watchdog.
func LimitSessions(h http.HandlerFunc) http.HandlerFunc { return limitWith(sessions, QueueWait, h) }

func limitWith(l *Limiter, wait time.Duration, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		release, err := l.Acquire(ctx, r.URL.Path, wait, cancel)
		if err != nil {
			if ctx.Err() != nil && r.Context().Err() != nil {
				return // the client went away while waiting: nobody to answer
			}
			st := l.Status()
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   "too_many_sessions",
				"message": strconv.Itoa(st.Running) + " of " + strconv.Itoa(st.Limit) + " crawl sessions are running; try again shortly",
				"limit":   st.Limit,
				"running": st.Running,
				"waiting": st.Waiting,
			})
			return
		}
		defer release()
		h(w, r.WithContext(ctx))
	}
}
