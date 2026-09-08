package executor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/judge"
)

type fakeBatchRunner struct {
	request judge.RunRequest
	inputs  []string
	results []judge.RunResult
	err     error
}

type blockingBatchRunner struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingBatchRunner) RunBatch(ctx context.Context, _ judge.RunRequest, _ []string) ([]judge.RunResult, error) {
	r.started <- struct{}{}
	select {
	case <-r.release:
		return []judge.RunResult{{Stage: judge.StageRun}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *fakeBatchRunner) RunBatch(_ context.Context, request judge.RunRequest, inputs []string) ([]judge.RunResult, error) {
	f.request = request
	f.inputs = append([]string(nil), inputs...)
	return f.results, f.err
}

func TestClientServerRoundTrip(t *testing.T) {
	runner := &fakeBatchRunner{results: []judge.RunResult{{Stdout: "3\n", Stage: judge.StageRun}}}
	server, err := NewServer(runner, "internal-secret", 2, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client, err := NewClient(httpServer.URL, "internal-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := judge.RunRequest{Language: domain.LanguagePython, Code: "print(3)", TimeLimitMS: 2000, MemoryLimitMB: 64}
	results, err := client.RunBatch(context.Background(), request, []string{"1 2\n"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Stdout != "3\n" || results[0].Stage != judge.StageRun {
		t.Fatalf("results = %+v", results)
	}
	if runner.request != request || len(runner.inputs) != 1 || runner.inputs[0] != "1 2\n" {
		t.Fatalf("runner request = %+v, inputs = %v", runner.request, runner.inputs)
	}
}

func TestServerRejectsMissingTokenBeforeExecution(t *testing.T) {
	runner := &fakeBatchRunner{}
	server, err := NewServer(runner, "internal-secret", 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"language":"python","code":"print(1)","inputs":[""],"timeLimitMs":1000,"memoryLimitMb":64}`
	request := httptest.NewRequest(http.MethodPost, "/v1/run-batch", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if runner.request.Code != "" {
		t.Fatal("runner was invoked for an unauthorized request")
	}
}

func TestServerRejectsInvalidLimits(t *testing.T) {
	runner := &fakeBatchRunner{}
	server, err := NewServer(runner, "internal-secret", 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"language":"python","code":"print(1)","inputs":[""],"timeLimitMs":0,"memoryLimitMb":64}`
	request := httptest.NewRequest(http.MethodPost, "/v1/run-batch", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer internal-secret")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestServerBoundsConcurrentExecutions(t *testing.T) {
	runner := &blockingBatchRunner{started: make(chan struct{}, 2), release: make(chan struct{}, 2)}
	server, err := NewServer(runner, "internal-secret", 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	call := func(result chan<- error) {
		body := `{"language":"python","code":"print(1)","inputs":[""],"timeLimitMs":1000,"memoryLimitMb":64}`
		request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/run-batch", strings.NewReader(body))
		if err != nil {
			result <- err
			return
		}
		request.Header.Set("Authorization", "Bearer internal-secret")
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				err = errors.New(response.Status)
			}
		}
		result <- err
	}

	results := make(chan error, 2)
	go call(results)
	<-runner.started
	go call(results)
	select {
	case <-runner.started:
		t.Fatal("second execution started before the first released its slot")
	case <-time.After(100 * time.Millisecond):
	}
	runner.release <- struct{}{}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("second execution did not acquire the released slot")
	}
	runner.release <- struct{}{}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestServerReadinessFailure(t *testing.T) {
	server, err := NewServer(&fakeBatchRunner{}, "internal-secret", 1, nil, func(context.Context) error {
		return errors.New("docker unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body["status"] != "not_ready" {
		t.Fatalf("body = %q, err = %v", recorder.Body.String(), err)
	}
}

func TestClientRejectsMismatchedResultCount(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, batchResponse{})
	}))
	defer httpServer.Close()
	client, err := NewClient(httpServer.URL, "internal-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.RunBatch(context.Background(), judge.RunRequest{}, []string{"one"})
	if err == nil || !strings.Contains(err.Error(), "invalid result count 0 for 1 inputs") {
		t.Fatalf("error = %v", err)
	}
}

func TestClientAcceptsShortCircuitResult(t *testing.T) {
	runner := &fakeBatchRunner{results: []judge.RunResult{{Stage: judge.StageCompile, ExitCode: 1}}}
	server, err := NewServer(runner, "internal-secret", 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client, err := NewClient(httpServer.URL, "internal-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := judge.RunRequest{Language: domain.LanguageGo, Code: "invalid", TimeLimitMS: 1000, MemoryLimitMB: 64}
	results, err := client.RunBatch(context.Background(), request, []string{"one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Stage != judge.StageCompile {
		t.Fatalf("results = %+v", results)
	}
}
