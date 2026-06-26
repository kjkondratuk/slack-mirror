// Package health exposes liveness/readiness and lightweight metrics for the
// Cloud Run *service* fallback. The worker-pool deployment ignores PORT and does
// not need this, but it is cheap to always provide.
package health

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type State struct {
	connected atomic.Bool

	mu        sync.Mutex
	lastEvent time.Time

	EventsProcessed atomic.Int64
	WriteErrors     atomic.Int64
}

func (s *State) SetConnected(v bool) { s.connected.Store(v) }

func (s *State) MarkEvent(t time.Time) {
	s.mu.Lock()
	s.lastEvent = t
	s.mu.Unlock()
	s.EventsProcessed.Add(1)
}

func (s *State) lastEventAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastEvent
}

// Handler returns a mux serving /healthz (liveness), /readyz (readiness), and
// /metrics (plain text). readyz is unhealthy if disconnected or if the last
// event is older than staleAfter (when any event has been seen).
func Handler(s *State, staleAfter time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !s.connected.Load() {
			http.Error(w, "socketmode disconnected", http.StatusServiceUnavailable)
			return
		}
		if last := s.lastEventAt(); !last.IsZero() && time.Since(last) > staleAfter {
			http.Error(w, "no events recently", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "events_processed_total %d\n", s.EventsProcessed.Load())
		fmt.Fprintf(w, "db_write_errors_total %d\n", s.WriteErrors.Load())
	})
	return mux
}
