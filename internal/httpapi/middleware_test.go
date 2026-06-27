package httpapi_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
