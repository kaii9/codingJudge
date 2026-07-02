package workerapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/workerapi"
)

type fakeEvaluator struct {
	gotProblem domain.Problem
	gotCode    string
}

func (f *fakeEvaluator) Evaluate(ctx context.Context, problem domain.Problem, language domain.Language, code string) (domain.JudgeResult, error) {
	f.gotProblem = problem
	f.gotCode = code
	return domain.JudgeResult{Status: domain.StatusAccepted}, nil
}

func TestWorkerJudgeEndpointEvaluatesRequest(t *testing.T) {
	t.Parallel()

	evaluator := &fakeEvaluator{}
	server := workerapi.NewServer(evaluator)
	body := []byte(`{
		"submissionId":"sub-1",
		"language":"go",
		"code":"package main",
		"problem":{
			"id":"sum",
			"title":"A+B",
			"testCases":[{"Input":"1 2\n","ExpectedOutput":"3\n"}]
		}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/judge", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var result domain.JudgeResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Status != domain.StatusAccepted {
		t.Fatalf("status = %q, want %q", result.Status, domain.StatusAccepted)
	}
	if evaluator.gotProblem.ID != "sum" || evaluator.gotCode != "package main" {
		t.Fatalf("evaluator inputs = problem %+v code %q", evaluator.gotProblem, evaluator.gotCode)
	}
}
