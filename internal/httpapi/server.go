package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kai/codingjudge/internal/auth"
	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/store"
)

const MaxCodeBytes = 64 * 1024
const sessionCookieName = "gojudge_session"
const sessionTTL = 7 * 24 * time.Hour

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

type ProblemStore interface {
	ListProblems(context.Context) ([]domain.Problem, error)
	GetProblem(context.Context, string) (domain.Problem, bool, error)
	CreateSubmission(context.Context, domain.Submission) (domain.Submission, error)
	ListSubmissions(context.Context) ([]domain.Submission, error)
	GetSubmission(context.Context, string) (domain.Submission, bool, error)
	CreateUser(context.Context, string, string) (domain.User, error)
	GetUserByUsername(context.Context, string) (domain.User, bool, error)
	GetPasswordHashByUsername(context.Context, string) (string, domain.User, bool, error)
	CreateSession(context.Context, string, string, time.Time) (domain.Session, error)
	GetUserBySessionTokenHash(context.Context, string, time.Time) (domain.User, bool, error)
	DeleteSession(context.Context, string) error
	ListSubmissionsByUser(context.Context, string) ([]domain.Submission, error)
	GetSubmissionForUser(context.Context, string, string) (domain.Submission, bool, error)
	ListLeaderboard(context.Context, int) ([]domain.LeaderboardEntry, error)
}

type Server struct {
	store             ProblemStore
	router            http.Handler
	metricsHandler    http.Handler
	httpMetrics       HTTPMetrics
	submissionMetrics SubmissionMetrics
}

// HTTPMetrics records HTTP-level observations.
type HTTPMetrics interface {
	ObserveHTTP(method, route string, status int, duration time.Duration)
}

// SubmissionMetrics records submission creation events.
type SubmissionMetrics interface {
	SubmissionCreated(language string)
}

// Option configures a Server.
type Option func(*Server)

// WithMetricsHandler adds a GET /metrics endpoint served by the given handler.
func WithMetricsHandler(handler http.Handler) Option {
	return func(s *Server) {
		s.metricsHandler = handler
	}
}

// WithHTTPMetrics sets the HTTP observer; when set, ObserveHTTP middleware is
// applied inside the chi routing chain so route patterns are available.
func WithHTTPMetrics(m HTTPMetrics) Option {
	return func(s *Server) {
		s.httpMetrics = m
	}
}

// WithSubmissionMetrics sets the submission creation recorder.
func WithSubmissionMetrics(m SubmissionMetrics) Option {
	return func(s *Server) {
		s.submissionMetrics = m
	}
}

func NewServer(store ProblemStore, options ...Option) *Server {
	s := &Server{store: store}
	for _, opt := range options {
		opt(s)
	}
	r := chi.NewRouter()
	if s.httpMetrics != nil {
		r.Use(ObserveHTTP(s.httpMetrics))
	}
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	if s.metricsHandler != nil {
		r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
			s.metricsHandler.ServeHTTP(w, r)
		})
	}
	r.Post("/auth/register", s.register)
	r.Post("/auth/login", s.login)
	r.Post("/auth/logout", s.logout)
	r.Get("/auth/me", s.me)
	r.Get("/problems", s.listProblems)
	r.Get("/problems/{id}", s.getProblem)
	r.Post("/submissions", s.createSubmission)
	r.Get("/submissions", s.listSubmissions)
	r.Get("/submissions/{id}", s.getSubmission)
	r.Get("/leaderboard", s.listLeaderboard)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	s.router = r
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeAuthRequest(w, r, &req) {
		return
	}
	if !validUsername(req.Username) || !validPassword(req.Password) {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "username or password is invalid")
		return
	}
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash password")
		return
	}
	user, err := s.store.CreateUser(r.Context(), strings.TrimSpace(req.Username), passwordHash)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, "conflict", "username already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "create user")
		return
	}
	if err := s.issueSession(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "create session")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeAuthRequest(w, r, &req) {
		return
	}
	passwordHash, user, ok, err := s.store.GetPasswordHashByUsername(r.Context(), req.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get user")
		return
	}
	if !ok || !auth.CheckPassword(passwordHash, req.Password) {
		writeErrorCode(w, http.StatusUnauthorized, "unauthenticated", "invalid username or password")
		return
	}
	if err := s.issueSession(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "create session")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if err := s.store.DeleteSession(r.Context(), auth.HashSessionToken(cookie.Value)); err != nil {
			writeError(w, http.StatusInternalServerError, "delete session")
			return
		}
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, ok, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get current user")
		return
	}
	if !ok {
		writeErrorCode(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	writeJSON(w, http.StatusOK, user)
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
	user, ok, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get current user")
		return
	}
	if !ok {
		writeErrorCode(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}

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
		UserID:    user.ID,
		ProblemID: req.ProblemID,
		Language:  req.Language,
		Code:      req.Code,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create submission")
		return
	}
	if s.submissionMetrics != nil {
		s.submissionMetrics.SubmissionCreated(string(req.Language))
	}
	sub.Code = ""
	writeJSON(w, http.StatusAccepted, sub)
}

func (s *Server) listSubmissions(w http.ResponseWriter, r *http.Request) {
	user, ok, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get current user")
		return
	}
	if !ok {
		writeErrorCode(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	submissions, err := s.store.ListSubmissionsByUser(r.Context(), user.ID)
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
	user, userOK, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get current user")
		return
	}
	if !userOK {
		writeErrorCode(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	sub, ok, err := s.store.GetSubmissionForUser(r.Context(), id, user.ID)
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

func (s *Server) listLeaderboard(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListLeaderboard(r.Context(), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func decodeAuthRequest(w http.ResponseWriter, r *http.Request, req any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return false
	}
	return true
}

func validUsername(username string) bool {
	return usernamePattern.MatchString(strings.TrimSpace(username))
}

func validPassword(password string) bool {
	return len(password) >= 8 && len(password) <= 72
}

func (s *Server) currentUser(r *http.Request) (domain.User, bool, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return domain.User{}, false, nil
	}
	return s.store.GetUserBySessionTokenHash(r.Context(), auth.HashSessionToken(cookie.Value), time.Now().UTC())
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID string) error {
	token, err := auth.NewSessionToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(sessionTTL)
	if _, err := s.store.CreateSession(r.Context(), userID, auth.HashSessionToken(token), expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
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
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusConflict:
		return "conflict"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	default:
		return "internal_error"
	}
}

var ErrNotFound = errors.New("not found")
