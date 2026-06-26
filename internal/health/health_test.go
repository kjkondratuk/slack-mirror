package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadyzStaleness(t *testing.T) {
	st := &State{}
	st.SetConnected(true)
	st.MarkEvent(time.Now())

	h := Handler(st, time.Minute)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("fresh: code = %d, want 200", rec.Code)
	}

	st.SetConnected(false)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("disconnected: code = %d, want 503", rec.Code)
	}
}

func TestHealthzAlwaysOK(t *testing.T) {
	st := &State{}
	h := Handler(st, time.Minute)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz code = %d, want 200", rec.Code)
	}
}

func TestMetricsIncludesLastEvent(t *testing.T) {
	st := &State{}
	st.MarkEvent(time.Unix(1700000000, 0))
	h := Handler(st, time.Minute)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "events_processed_total 1") {
		t.Fatalf("missing events_processed_total: %q", body)
	}
	if !strings.Contains(body, "last_event_timestamp_seconds 1700000000") {
		t.Fatalf("missing last_event_timestamp_seconds: %q", body)
	}
}
