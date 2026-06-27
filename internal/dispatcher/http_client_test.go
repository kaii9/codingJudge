package dispatcher_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kai/codingjudge/internal/dispatcher"
	"github.com/kai/codingjudge/internal/domain"
)

func TestHTTPJudgeClientPostsJudgeRequest(t *testing.T) {
	t.Parallel()

	var got dispatcher.JudgeRequest
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/judge" {
			t.Fatalf("request = %s %s, want POST /judge", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"status":"accepted"}`)),
		}, nil
	})}

	judgeClient := dispatcher.NewHTTPJudgeClient("http://worker:8081", client)
	result, err := judgeClient.Judge(context.Background(), dispatcher.JudgeRequest{
		SubmissionID: "sub-1",
		Problem:      domain.Problem{ID: "sum"},
		Language:     domain.LanguageGo,
		Code:         "package main",
	})
	if err != nil {
		t.Fatalf("Judge returned error: %v", err)
	}
	if result.Status != domain.StatusAccepted {
		t.Fatalf("status = %q, want %q", result.Status, domain.StatusAccepted)
	}
	if got.SubmissionID != "sub-1" || got.Problem.ID != "sum" {
		t.Fatalf("request = %+v", got)
	}
}

func TestHTTPJudgeClientIncludesHiddenTestCasesForWorker(t *testing.T) {
	t.Parallel()

	var raw map[string]any
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"status":"accepted"}`)),
		}, nil
	})}

	judgeClient := dispatcher.NewHTTPJudgeClient("http://worker:8081", client)
	_, err := judgeClient.Judge(context.Background(), dispatcher.JudgeRequest{
		SubmissionID: "sub-1",
		Problem: domain.Problem{
			ID: "sum",
			TestCases: []domain.TestCase{
				{Input: "1 2\n", ExpectedOutput: "3\n"},
			},
		},
		Language: domain.LanguageGo,
		Code:     "package main",
	})
	if err != nil {
		t.Fatalf("Judge returned error: %v", err)
	}

	problem, ok := raw["problem"].(map[string]any)
	if !ok {
		t.Fatalf("problem payload = %#v", raw["problem"])
	}
	testCases, ok := problem["testCases"].([]any)
	if !ok || len(testCases) != 1 {
		t.Fatalf("testCases payload = %#v", problem["testCases"])
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
