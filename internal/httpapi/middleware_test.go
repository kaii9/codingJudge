package httpapi_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kai/codingjudge/internal/httpapi"
)

func TestAccessLogRecordsRequestMetadata(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{}))
	handler := httpapi.AccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}), logger)

	req := httptest.NewRequest(http.MethodPost, "/submissions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := logs.String()
	for _, want := range []string{"method=POST", "path=/submissions", "status=201"} {
		if !strings.Contains(got, want) {
			t.Fatalf("access log missing %q in %q", want, got)
		}
	}
}

// fakeHTTPMetrics records ObserveHTTP calls for testing.
type fakeHTTPMetrics struct {
	mu    sync.Mutex
	calls []observeHTTPCall
}

type observeHTTPCall struct {
	method      string
	route       string
	status      int
	durationSet bool
}

func (f *fakeHTTPMetrics) ObserveHTTP(method, route string, status int, duration time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, observeHTTPCall{
		method:      method,
		route:       route,
		status:      status,
		durationSet: duration > 0,
	})
}

func TestObserveHTTPMatchesChiPattern(t *testing.T) {
	metrics := &fakeHTTPMetrics{}

	r := chi.NewRouter()
	r.Use(httpapi.ObserveHTTP(metrics))
	r.Get("/submissions/{id}", func(w http.ResponseWriter, r *http.Request) {})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Known route should record chi template.
	req := httptest.NewRequest(http.MethodGet, "/submissions/sub-123", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	metrics.mu.Lock()
	if len(metrics.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(metrics.calls))
	}
	call := metrics.calls[0]
	metrics.mu.Unlock()

	if call.method != "GET" {
		t.Errorf("method = %q, want GET", call.method)
	}
	if call.route != "/submissions/{id}" {
		t.Errorf("route = %q, want /submissions/{id}", call.route)
	}
	if call.status != 200 {
		t.Errorf("status = %d, want 200", call.status)
	}
	if !call.durationSet {
		t.Error("duration should be positive")
	}
}

func TestObserveHTTP404UsesUnmatched(t *testing.T) {
	metrics := &fakeHTTPMetrics{}

	r := chi.NewRouter()
	r.Use(httpapi.ObserveHTTP(metrics))
	r.Get("/problems", func(w http.ResponseWriter, r *http.Request) {})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/no-such-path", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	if len(metrics.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(metrics.calls))
	}
	if metrics.calls[0].route != "unmatched" {
		t.Errorf("route = %q, want unmatched", metrics.calls[0].route)
	}
}

func TestObserveHTTPSkipsMetricsPath(t *testing.T) {
	metrics := &fakeHTTPMetrics{}

	r := chi.NewRouter()
	r.Use(httpapi.ObserveHTTP(metrics))
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {})
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	if len(metrics.calls) != 0 {
		t.Errorf("expected metrics path to be skipped, got %d calls", len(metrics.calls))
	}
}
