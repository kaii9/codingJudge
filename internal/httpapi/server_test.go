package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaii9/codingJudge/internal/auth"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/httpapi"
	"github.com/kaii9/codingJudge/internal/ratelimit"
	"github.com/kaii9/codingJudge/internal/store"
)

// fakeSubmissionMetrics records SubmissionCreated calls for testing.
type fakeSubmissionMetrics struct {
	mu        sync.Mutex
	languages []string
}

type fakeSubmissionControlMetrics struct {
	mu          sync.Mutex
	idempotency []string
	rateLimited int
}

func (f *fakeSubmissionControlMetrics) SubmissionIdempotency(result string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idempotency = append(f.idempotency, result)
}

func (f *fakeSubmissionControlMetrics) SubmissionRateLimited() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rateLimited++
}

type fakeSubmissionLimiter struct {
	mu        sync.Mutex
	decisions []ratelimit.Decision
	err       error
	calls     int
}

func (f *fakeSubmissionLimiter) Allow(context.Context, string) (ratelimit.Decision, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return ratelimit.Decision{}, f.err
	}
	if len(f.decisions) == 0 {
		return ratelimit.Decision{Allowed: true, Limit: 10, Remaining: 9}, nil
	}
	decision := f.decisions[0]
	if len(f.decisions) > 1 {
		f.decisions = f.decisions[1:]
	}
	return decision, nil
}

func (f *fakeSubmissionMetrics) SubmissionCreated(language string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.languages = append(f.languages, language)
}

type fakeHTTPObjectStore struct {
	objects map[string][]byte
}

func (s fakeHTTPObjectStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), s.objects[key]...), nil
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %q, want ok", body["status"])
	}
}

func TestReadyzReportsDependencyFailure(t *testing.T) {
	t.Parallel()

	server := httpapi.NewServer(
		store.NewMemoryStore(testProblems()),
		httpapi.WithReadinessCheck("redis", func(context.Context) error {
			return context.DeadlineExceeded
		}),
	)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "not_ready" || body["component"] != "redis" {
		t.Fatalf("body = %#v", body)
	}
}

func TestReadyzSucceedsWhenDependenciesAreHealthy(t *testing.T) {
	t.Parallel()

	server := httpapi.NewServer(
		store.NewMemoryStore(testProblems()),
		httpapi.WithReadinessCheck("store", func(context.Context) error { return nil }),
	)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestSecureCookieOptionAppliesToSessionCookies(t *testing.T) {
	t.Parallel()

	server := httpapi.NewServer(store.NewMemoryStore(testProblems()), httpapi.WithSecureCookies(true))
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{"username":"secure_user","password":"correct-password"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("cookies = %+v, want Secure session cookie", cookies)
	}
}

func TestListProblemsDoesNotExposeTestCases(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/problems", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("problem count = %d, want 1", len(body))
	}
	if _, ok := body[0]["testCases"]; ok {
		t.Fatal("problem list should not expose hidden test cases")
	}
	if body[0]["id"] != "sum" {
		t.Fatalf("problem id = %q, want sum", body[0]["id"])
	}
	if body[0]["difficulty"] != "easy" || body[0]["collection"] != "starter" {
		t.Fatalf("problem metadata = %#v", body[0])
	}
}

func TestCreateSubmissionPersistsQueuedSubmission(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore(testProblems())
	server := httpapi.NewServer(st)
	cookie := registerTestUser(t, server, "kai")

	payload := []byte(`{"problemId":"sum","language":"go","code":"package main\nfunc main(){}"}`)
	req := httptest.NewRequest(http.MethodPost, "/submissions", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var created domain.Submission
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID == "" || created.Status != domain.StatusQueued {
		t.Fatalf("created submission = %+v", created)
	}
	if created.UserID == "" {
		t.Fatalf("created submission should include user id: %+v", created)
	}

	stored, ok, err := st.GetSubmission(context.Background(), created.ID)
	if err != nil || !ok || stored.Status != domain.StatusQueued {
		t.Fatalf("stored submission = %+v, %v, %v", stored, ok, err)
	}
	if stored.UserID != created.UserID {
		t.Fatalf("stored user id = %q, want %q", stored.UserID, created.UserID)
	}
}

func TestCreateSubmissionReplaysIdempotentRequestWithoutConsumingAnotherToken(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore(testProblems())
	limiter := &fakeSubmissionLimiter{decisions: []ratelimit.Decision{
		{Allowed: true, Limit: 10, Remaining: 2},
		{Allowed: false, Limit: 10, RetryAfter: 6 * time.Second},
	}}
	controlMetrics := &fakeSubmissionControlMetrics{}
	server := httpapi.NewServer(st,
		httpapi.WithSubmissionLimiter(limiter),
		httpapi.WithSubmissionControlMetrics(controlMetrics),
	)
	cookie := registerTestUser(t, server, "idempotent_user")
	payload := []byte(`{"problemId":"sum","language":"go","code":"package main\nfunc main(){}"}`)

	create := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/submissions", bytes.NewReader(payload))
		req.AddCookie(cookie)
		req.Header.Set("Idempotency-Key", "attempt-123")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}
	first := create()
	second := create()
	if first.Code != http.StatusAccepted || second.Code != http.StatusAccepted {
		t.Fatalf("statuses = %d, %d; bodies=%s / %s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	var firstSub, secondSub domain.Submission
	if err := json.NewDecoder(first.Body).Decode(&firstSub); err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(second.Body).Decode(&secondSub); err != nil {
		t.Fatal(err)
	}
	if firstSub.ID != secondSub.ID || second.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("first=%+v second=%+v replay=%q", firstSub, secondSub, second.Header().Get("Idempotency-Replayed"))
	}
	limiter.mu.Lock()
	if limiter.calls != 1 {
		t.Fatalf("limiter calls = %d, want 1", limiter.calls)
	}
	limiter.mu.Unlock()
	submissions, err := st.ListSubmissions(context.Background())
	if err != nil || len(submissions) != 1 {
		t.Fatalf("submissions=%+v err=%v, want one", submissions, err)
	}
	controlMetrics.mu.Lock()
	defer controlMetrics.mu.Unlock()
	if strings.Join(controlMetrics.idempotency, ",") != "created,replayed" {
		t.Fatalf("idempotency metrics = %v", controlMetrics.idempotency)
	}
}

func TestCreateSubmissionRejectsIdempotencyKeyReuseWithDifferentPayload(t *testing.T) {
	t.Parallel()
	st := store.NewMemoryStore(testProblems())
	server := httpapi.NewServer(st)
	cookie := registerTestUser(t, server, "conflict_user")

	first := httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader(`{"problemId":"sum","language":"go","code":"first"}`))
	first.AddCookie(cookie)
	first.Header.Set("Idempotency-Key", "same-key")
	firstRec := httptest.NewRecorder()
	server.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusAccepted {
		t.Fatalf("first status=%d body=%s", firstRec.Code, firstRec.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader(`{"problemId":"sum","language":"go","code":"different"}`))
	second.AddCookie(cookie)
	second.Header.Set("Idempotency-Key", "same-key")
	secondRec := httptest.NewRecorder()
	server.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("second status=%d body=%s", secondRec.Code, secondRec.Body.String())
	}
	var body map[string]map[string]string
	if err := json.NewDecoder(secondRec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"]["code"] != "idempotency_conflict" {
		t.Fatalf("error body=%+v", body)
	}
}

func TestCreateSubmissionReturnsRateLimitHeaders(t *testing.T) {
	t.Parallel()
	st := store.NewMemoryStore(testProblems())
	limiter := &fakeSubmissionLimiter{decisions: []ratelimit.Decision{{
		Allowed: false, Limit: 10, Remaining: 0, RetryAfter: 5500 * time.Millisecond,
	}}}
	metrics := &fakeSubmissionControlMetrics{}
	server := httpapi.NewServer(st,
		httpapi.WithSubmissionLimiter(limiter),
		httpapi.WithSubmissionControlMetrics(metrics),
	)
	cookie := registerTestUser(t, server, "limited_user")
	req := httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader(`{"problemId":"sum","language":"go","code":"code"}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("RateLimit-Limit") != "10" || rec.Header().Get("RateLimit-Remaining") != "0" || rec.Header().Get("Retry-After") != "6" {
		t.Fatalf("rate limit headers=%v", rec.Header())
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if metrics.rateLimited != 1 {
		t.Fatalf("rate limited metric=%d, want 1", metrics.rateLimited)
	}
}

func TestCreateSubmissionFailsClosedWhenRateLimiterIsUnavailable(t *testing.T) {
	t.Parallel()
	st := store.NewMemoryStore(testProblems())
	limiter := &fakeSubmissionLimiter{err: errors.New("redis unavailable")}
	server := httpapi.NewServer(st, httpapi.WithSubmissionLimiter(limiter))
	cookie := registerTestUser(t, server, "limiter_error_user")
	req := httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader(`{"problemId":"sum","language":"go","code":"code"}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"]["code"] != "rate_limit_unavailable" {
		t.Fatalf("error body=%+v", body)
	}
}

func TestCreateSubmissionRequiresLogin(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	payload := []byte(`{"problemId":"sum","language":"go","code":"package main\nfunc main(){}"}`)
	req := httptest.NewRequest(http.MethodPost, "/submissions", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCreateSubmissionRejectsOversizedCode(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	payload := `{"problemId":"sum","language":"go","code":"` + strings.Repeat("x", httpapi.MaxCodeBytes+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader(payload))
	req.AddCookie(registerTestUser(t, server, "kai"))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	var body map[string]map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"]["code"] != "request_too_large" {
		t.Fatalf("error code = %q, want request_too_large", body["error"]["code"])
	}
}

func TestCreateSubmissionReturnsStructuredError(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader(`{"problemId":"sum"}`))
	req.AddCookie(registerTestUser(t, server, "kai"))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var body map[string]map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"]["code"] != "invalid_request" || body["error"]["message"] == "" {
		t.Fatalf("error body = %+v", body)
	}
}

func TestGetSubmissionReturnsStoredSubmission(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore(testProblems())
	user, err := st.CreateUser(context.Background(), "kai", "hash")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	sub, err := st.CreateSubmission(context.Background(), domain.Submission{
		UserID:    user.ID,
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "code",
	})
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}
	server := httpapi.NewServer(st)
	cookie := registerExistingUserSession(t, st, user.ID)

	req := httptest.NewRequest(http.MethodGet, "/submissions/"+sub.ID, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got domain.Submission
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != sub.ID || got.Code != "" {
		t.Fatalf("submission response = %+v", got)
	}
}

func TestListSubmissionsReturnsHistoryWithoutCode(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore(testProblems())
	user, err := st.CreateUser(context.Background(), "kai", "hash")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	other, err := st.CreateUser(context.Background(), "lin", "hash")
	if err != nil {
		t.Fatalf("CreateUser other returned error: %v", err)
	}
	first, err := st.CreateSubmission(context.Background(), domain.Submission{
		UserID:    user.ID,
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "first secret code",
	})
	if err != nil {
		t.Fatalf("CreateSubmission first returned error: %v", err)
	}
	second, err := st.CreateSubmission(context.Background(), domain.Submission{
		UserID:    user.ID,
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "second secret code",
	})
	if err != nil {
		t.Fatalf("CreateSubmission second returned error: %v", err)
	}
	if _, err := st.CreateSubmission(context.Background(), domain.Submission{
		UserID:    other.ID,
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "other secret code",
	}); err != nil {
		t.Fatalf("CreateSubmission other returned error: %v", err)
	}
	server := httpapi.NewServer(st)
	cookie := registerExistingUserSession(t, st, user.ID)

	req := httptest.NewRequest(http.MethodGet, "/submissions", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []domain.Submission
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("submission count = %d, want 2", len(got))
	}
	if got[0].ID != second.ID || got[1].ID != first.ID {
		t.Fatalf("submission order = [%s, %s], want [%s, %s]", got[0].ID, got[1].ID, second.ID, first.ID)
	}
	if got[0].Code != "" || got[1].Code != "" {
		t.Fatalf("submission history should not expose code: %+v", got)
	}
}

func TestServerRecordsSubmissionLanguage(t *testing.T) {
	st := store.NewMemoryStore(testProblems())
	metrics := &fakeSubmissionMetrics{}
	server := httpapi.NewServer(st, httpapi.WithSubmissionMetrics(metrics))
	cookie := registerTestUser(t, server, "kai")

	payload := []byte(`{"problemId":"sum","language":"go","code":"package main\nfunc main(){}"}`)
	req := httptest.NewRequest(http.MethodPost, "/submissions", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	if len(metrics.languages) != 1 {
		t.Fatalf("expected 1 submission metric, got %d", len(metrics.languages))
	}
	if metrics.languages[0] != "go" {
		t.Errorf("language = %q, want go", metrics.languages[0])
	}
}

func TestServerDoesNotRecordFailedSubmission(t *testing.T) {
	st := store.NewMemoryStore(testProblems())
	metrics := &fakeSubmissionMetrics{}
	server := httpapi.NewServer(st, httpapi.WithSubmissionMetrics(metrics))
	cookie := registerTestUser(t, server, "kai")

	// Missing required fields — should return 400 without recording.
	req := httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader(`{}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if len(metrics.languages) != 0 {
		t.Errorf("expected 0 submissions recorded for failed request, got %d: %v", len(metrics.languages), metrics.languages)
	}
}

func TestAuthRegisterSetsCookieAndMeReturnsUser(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	cookie := registerTestUser(t, server, "Kai_01")
	if !cookie.HttpOnly {
		t.Fatal("session cookie should be HttpOnly")
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var user domain.User
	if err := json.NewDecoder(rec.Body).Decode(&user); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if user.ID == "" || user.Username != "Kai_01" {
		t.Fatalf("user = %+v", user)
	}
}

func TestAuthLoginRejectsWrongPassword(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	_ = registerTestUser(t, server, "kai")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"kai","password":"wrong-password"}`))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthLogoutClearsSession(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	cookie := registerTestUser(t, server, "kai")

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	cleared := rec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("logout cookie = %+v, want clearing cookie", cleared)
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestLeaderboardReturnsAcceptedCounts(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore(testProblems())
	ctx := context.Background()
	kai, err := st.CreateUser(ctx, "kai", "hash")
	if err != nil {
		t.Fatalf("CreateUser kai returned error: %v", err)
	}
	lin, err := st.CreateUser(ctx, "lin", "hash")
	if err != nil {
		t.Fatalf("CreateUser lin returned error: %v", err)
	}
	completeHTTPTestSubmission(t, st, domain.Submission{UserID: kai.ID, ProblemID: "sum", Language: domain.LanguageGo, Code: "ok"}, domain.StatusAccepted)
	completeHTTPTestSubmission(t, st, domain.Submission{UserID: kai.ID, ProblemID: "sum", Language: domain.LanguageGo, Code: "ok again"}, domain.StatusAccepted)
	completeHTTPTestSubmission(t, st, domain.Submission{UserID: lin.ID, ProblemID: "sum", Language: domain.LanguageGo, Code: "ok"}, domain.StatusAccepted)
	server := httpapi.NewServer(st)

	req := httptest.NewRequest(http.MethodGet, "/leaderboard", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var entries []domain.LeaderboardEntry
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if entries[0].Username != "kai" || entries[0].Solved != 1 || entries[0].AcceptedSubmissions != 2 {
		t.Fatalf("first entry = %+v, want kai solved=1 accepted=2", entries[0])
	}
}

func TestSubmissionArtifactsRequireOwnerAndCanDownloadObject(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore(testProblems())
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "kai", "hash")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateUser(ctx, "lin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := st.CreateSubmission(ctx, domain.Submission{UserID: user.ID, ProblemID: "sum", Language: domain.LanguageGo, Code: "code"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claim, err := st.ClaimSubmission(ctx, sub.ID, "worker", "token", "1-0", now, time.Minute)
	if err != nil || claim.State != domain.ClaimAcquired {
		t.Fatalf("ClaimSubmission = %+v, %v", claim, err)
	}
	artifact := domain.SubmissionArtifact{
		Attempt:   1,
		Kind:      domain.ArtifactKindStderr,
		ObjectKey: "artifacts/sub/attempt-1/token/stderr.txt",
		SHA256:    "sha",
		SizeBytes: 6,
	}
	if ok, err := st.SaveSubmissionArtifacts(ctx, sub.ID, "token", now.Add(time.Second), []domain.SubmissionArtifact{artifact}); err != nil || !ok {
		t.Fatalf("SaveSubmissionArtifacts = %v, %v", ok, err)
	}
	if ok, err := st.CompleteSubmission(ctx, sub.ID, "token", now.Add(2*time.Second), domain.JudgeResult{Status: domain.StatusRuntimeError}); err != nil || !ok {
		t.Fatalf("CompleteSubmission = %v, %v", ok, err)
	}
	server := httpapi.NewServer(st, httpapi.WithObjectGetter(fakeHTTPObjectStore{objects: map[string][]byte{
		artifact.ObjectKey: []byte("panic\n"),
	}}))

	req := httptest.NewRequest(http.MethodGet, "/submissions/"+sub.ID+"/artifacts", nil)
	req.AddCookie(registerExistingUserSession(t, st, user.ID))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var artifacts []domain.SubmissionArtifact
	if err := json.NewDecoder(rec.Body).Decode(&artifacts); err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].ObjectKey != "" || artifacts[0].Token != "" || artifacts[0].Kind != domain.ArtifactKindStderr {
		t.Fatalf("artifacts response = %+v", artifacts)
	}

	req = httptest.NewRequest(http.MethodGet, "/submissions/"+sub.ID+"/artifacts/"+strconv.FormatInt(artifacts[0].ID, 10), nil)
	req.AddCookie(registerExistingUserSession(t, st, user.ID))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "panic\n" {
		t.Fatalf("download status=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/submissions/"+sub.ID+"/artifacts", nil)
	req.AddCookie(registerExistingUserSession(t, st, other.ID))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("other user list status = %d, want 404", rec.Code)
	}
}

func newTestServer() http.Handler {
	return httpapi.NewServer(store.NewMemoryStore(testProblems()))
}

func testProblems() []domain.Problem {
	return []domain.Problem{{
		ID:            "sum",
		Title:         "A+B",
		Description:   "Read two integers and print their sum.",
		Language:      domain.LanguageGo,
		TimeLimitMS:   1000,
		MemoryLimitMB: 64,
		Difficulty:    domain.DifficultyEasy, Collection: domain.CollectionStarter, SortOrder: 1, Tags: []string{"starter"},
		TestCases: []domain.TestCase{
			{Input: "1 2\n", ExpectedOutput: "3\n"},
		},
	}}
}

func registerTestUser(t *testing.T, server http.Handler, username string) *http.Cookie {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{"username":"`+username+`","password":"correct-password"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "gojudge_session" {
			return cookie
		}
	}
	t.Fatalf("session cookie not set: %+v", rec.Result().Cookies())
	return nil
}

func registerExistingUserSession(t *testing.T, st *store.MemoryStore, userID string) *http.Cookie {
	t.Helper()

	token := "test-token-" + userID
	_, err := st.CreateSession(context.Background(), userID, auth.HashSessionToken(token), time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	return &http.Cookie{Name: "gojudge_session", Value: token}
}

func completeHTTPTestSubmission(t *testing.T, st *store.MemoryStore, sub domain.Submission, status domain.SubmissionStatus) {
	t.Helper()

	created, err := st.CreateSubmission(context.Background(), sub)
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}
	now := time.Now().UTC()
	token := "token-" + created.ID
	claim, err := st.ClaimSubmission(context.Background(), created.ID, "worker", token, "1-0", now, time.Minute)
	if err != nil || claim.State != domain.ClaimAcquired {
		t.Fatalf("ClaimSubmission = %+v, %v", claim, err)
	}
	if ok, err := st.CompleteSubmission(context.Background(), created.ID, token, now.Add(time.Second), domain.JudgeResult{Status: status}); err != nil || !ok {
		t.Fatalf("CompleteSubmission = %v, %v", ok, err)
	}
}
