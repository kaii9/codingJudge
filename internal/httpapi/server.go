package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/kai/codingjudge/internal/domain"
)

const MaxCodeBytes = 64 * 1024

type ProblemStore interface {
	ListProblems(context.Context) ([]domain.Problem, error)
	GetProblem(context.Context, string) (domain.Problem, bool, error)
	CreateSubmission(context.Context, domain.Submission) (domain.Submission, error)
	ListSubmissions(context.Context) ([]domain.Submission, error)
	GetSubmission(context.Context, string) (domain.Submission, bool, error)
}

type JobQueue interface {
	Enqueue(context.Context, domain.Job) error
}

type Server struct {
	store  ProblemStore
	queue  JobQueue
	router http.Handler
}

func NewServer(store ProblemStore, queue JobQueue) *Server {
	s := &Server{store: store, queue: queue}
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/problems", s.listProblems)
	r.Get("/problems/{id}", s.getProblem)
	r.Post("/submissions", s.createSubmission)
	r.Get("/submissions", s.listSubmissions)
	r.Get("/submissions/{id}", s.getSubmission)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	s.router = r
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) listProblems(w http.ResponseWriter, r *http.Request) {
	problems, err := s.store.ListProblems(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list problems")
		return
	}
	for i := range problems {
		problems[i].TestCases = nil
	}
	writeJSON(w, http.StatusOK, problems)
}

func (s *Server) getProblem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusNotFound, "problem not found")
		return
	}
	problem, ok, err := s.store.GetProblem(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get problem")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "problem not found")
		return
	}
	problem.TestCases = nil
	writeJSON(w, http.StatusOK, problem)
}

func (s *Server) createSubmission(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxCodeBytes+1024)
	var req struct {
		ProblemID string          `json:"problemId"`
		Language  domain.Language `json:"language"`
		Code      string          `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "request body too large") {
			writeErrorCode(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large")
			return
		}
		writeErrorCode(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	if req.ProblemID == "" || req.Language == "" || strings.TrimSpace(req.Code) == "" {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "problemId, language and code are required")
		return
	}
	if len(req.Code) > MaxCodeBytes {
		writeErrorCode(w, http.StatusRequestEntityTooLarge, "request_too_large", "code is too large")
		return
	}
	if !domain.IsSupportedLanguage(req.Language) {
		writeErrorCode(w, http.StatusBadRequest, "unsupported_language", "unsupported language")
		return
	}
	if _, ok, err := s.store.GetProblem(r.Context(), req.ProblemID); err != nil {
		writeError(w, http.StatusInternalServerError, "get problem")
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "problem not found")
		return
	}

	sub, err := s.store.CreateSubmission(r.Context(), domain.Submission{
		ProblemID: req.ProblemID,
		Language:  req.Language,
		Code:      req.Code,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create submission")
		return
	}
	if err := s.queue.Enqueue(r.Context(), domain.Job{SubmissionID: sub.ID}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "queue submission")
		return
	}
	sub.Code = ""
	writeJSON(w, http.StatusAccepted, sub)
}

func (s *Server) listSubmissions(w http.ResponseWriter, r *http.Request) {
	submissions, err := s.store.ListSubmissions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list submissions")
		return
	}
	for i := range submissions {
		submissions[i].Code = ""
	}
	writeJSON(w, http.StatusOK, submissions)
}

func (s *Server) getSubmission(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusNotFound, "submission not found")
		return
	}
	sub, ok, err := s.store.GetSubmission(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get submission")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "submission not found")
		return
	}
	sub.Code = ""
	writeJSON(w, http.StatusOK, sub)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	if message == "" {
		message = http.StatusText(status)
	}
	writeErrorCode(w, status, defaultErrorCode(status), message)
}

func writeErrorCode(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]map[string]string{
		"error": {
			"code":    code,
			"message": message,
		},
	})
}

func defaultErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	default:
		return "internal_error"
	}
}

var ErrNotFound = errors.New("not found")
