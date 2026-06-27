package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/httpapi"
	"github.com/kai/codingjudge/internal/queue"
	"github.com/kai/codingjudge/internal/store"
)

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
}

func TestCreateSubmissionQueuesJudgeJob(t *testing.T) {
	t.Parallel()

	q := queue.NewMemoryQueue(1)
	st := store.NewMemoryStore(testProblems())
	server := httpapi.NewServer(st, q)

	payload := []byte(`{"problemId":"sum","language":"go","code":"package main\nfunc main(){}"}`)
	req := httptest.NewRequest(http.MethodPost, "/submissions", bytes.NewReader(payload))
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	job, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue returned error: %v", err)
	}
	if job.SubmissionID != created.ID {
		t.Fatalf("job submission = %q, want %q", job.SubmissionID, created.ID)
	}
}

func TestCreateSubmissionRejectsOversizedCode(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	payload := `{"problemId":"sum","language":"go","code":"` + strings.Repeat("x", httpapi.MaxCodeBytes+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader(payload))
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
	sub, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "code",
	})
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}
	server := httpapi.NewServer(st, queue.NewMemoryQueue(1))

	req := httptest.NewRequest(http.MethodGet, "/submissions/"+sub.ID, nil)
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
	first, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "first secret code",
	})
	if err != nil {
		t.Fatalf("CreateSubmission first returned error: %v", err)
	}
	second, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "second secret code",
	})
	if err != nil {
		t.Fatalf("CreateSubmission second returned error: %v", err)
	}
	server := httpapi.NewServer(st, queue.NewMemoryQueue(1))

	req := httptest.NewRequest(http.MethodGet, "/submissions", nil)
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

func newTestServer() http.Handler {
	return httpapi.NewServer(store.NewMemoryStore(testProblems()), queue.NewMemoryQueue(1))
}

func testProblems() []domain.Problem {
	return []domain.Problem{{
		ID:            "sum",
		Title:         "A+B",
		Description:   "Read two integers and print their sum.",
		Language:      domain.LanguageGo,
		TimeLimitMS:   1000,
		MemoryLimitMB: 64,
		TestCases: []domain.TestCase{
			{Input: "1 2\n", ExpectedOutput: "3\n"},
		},
	}}
}
