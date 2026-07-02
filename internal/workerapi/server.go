package workerapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/kai/codingjudge/internal/domain"
)

type Evaluator interface {
	Evaluate(context.Context, domain.Problem, domain.Language, string) (domain.JudgeResult, error)
}

type Server struct {
	evaluator Evaluator
}

func NewServer(evaluator Evaluator) *Server {
	return &Server{evaluator: evaluator}
}

type JudgeRequest struct {
	SubmissionID string          `json:"submissionId"`
	Problem      domain.Problem  `json:"problem"`
	Language     domain.Language `json:"language"`
	Code         string          `json:"code"`
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/judge" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	var req JudgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Problem.ID == "" || req.Language == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "problem, language and code are required"})
		return
	}

	result, err := s.evaluator.Evaluate(r.Context(), req.Problem, req.Language, req.Code)
	if err != nil {
		http.Error(w, "judge unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
