package executor

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/kaii9/codingJudge/internal/judge"
)

type ReadinessCheck func(context.Context) error

type Server struct {
	runner           judge.BatchRunner
	token            string
	slots            chan struct{}
	metrics          http.Handler
	executionMetrics executionMetrics
	readiness        ReadinessCheck
}

type ServerOption func(*Server)

func NewServer(runner judge.BatchRunner, token string, maxConcurrency int, metrics http.Handler, readiness ReadinessCheck, options ...ServerOption) (*Server, error) {
	if runner == nil {
		return nil, errors.New("executor runner is required")
	}
	if token == "" {
		return nil, errors.New("executor token is required")
	}
	if maxConcurrency < 1 {
		return nil, errors.New("executor max concurrency must be positive")
	}
	server := &Server{runner: runner, token: token, slots: make(chan struct{}, maxConcurrency), metrics: metrics, readiness: readiness}
	for _, option := range options {
		option(server)
	}
	return server, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("POST /v1/run-batch", s.runBatch)
	if s.metrics != nil {
		mux.Handle("GET /metrics", s.metrics)
	}
	return mux
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if s.readiness != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.readiness(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) runBatch(w http.ResponseWriter, r *http.Request) {
	if !validBearer(r.Header.Get("Authorization"), s.token) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}
	body := http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var request batchRequest
	if err := decoder.Decode(&request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "request_too_large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_request"})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_request"})
		return
	}
	if err := request.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	waitStarted := time.Now()
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-r.Context().Done():
		return
	}
	executionResult := "error"
	if s.executionMetrics != nil {
		s.executionMetrics.executionStarted(time.Since(waitStarted))
		defer func() { s.executionMetrics.executionFinished(executionResult) }()
	}
	results, err := s.runner.RunBatch(r.Context(), request.judgeRequest(), request.Inputs)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			executionResult = "canceled"
		}
		slog.Error("executor sandbox failed", "language", request.Language, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "execution_failed"})
		return
	}
	executionResult = "success"
	writeJSON(w, http.StatusOK, batchResponse{Results: results})
}

func validBearer(header, token string) bool {
	expected := "Bearer " + token
	return subtle.ConstantTimeCompare([]byte(header), []byte(expected)) == 1
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
