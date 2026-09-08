package executor

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/judge"
)

func TestClientPoolUsesEndpointsRoundRobin(t *testing.T) {
	first := newExecutorTestServer(t, "first")
	defer first.Close()
	second := newExecutorTestServer(t, "second")
	defer second.Close()

	pool, err := NewClientPool([]string{first.URL, second.URL}, "internal-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := judge.RunRequest{
		Language:      domain.LanguagePython,
		Code:          "print(1)",
		TimeLimitMS:   1000,
		MemoryLimitMB: 64,
	}
	want := []string{"first", "second", "first", "second"}
	for index, expected := range want {
		results, err := pool.RunBatch(context.Background(), request, []string{""})
		if err != nil {
			t.Fatalf("call %d: %v", index+1, err)
		}
		if len(results) != 1 || results[0].Stdout != expected {
			t.Fatalf("call %d results = %+v, want stdout %q", index+1, results, expected)
		}
	}
}

func TestClientPoolRequiresEndpoint(t *testing.T) {
	if _, err := NewClientPool(nil, "internal-secret", time.Second); err == nil {
		t.Fatal("NewClientPool should reject an empty endpoint list")
	}
}

func TestClientPoolBalancesConcurrentCalls(t *testing.T) {
	first := newExecutorTestServer(t, "first")
	defer first.Close()
	second := newExecutorTestServer(t, "second")
	defer second.Close()

	pool, err := NewClientPool([]string{first.URL, second.URL}, "internal-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}

	const calls = 100
	var counts [2]atomic.Int64
	errors := make(chan error, calls)
	var wait sync.WaitGroup
	for range calls {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results, err := pool.RunBatch(context.Background(), judge.RunRequest{
				Language:      domain.LanguagePython,
				Code:          "print(1)",
				TimeLimitMS:   1000,
				MemoryLimitMB: 64,
			}, []string{""})
			if err != nil {
				errors <- err
				return
			}
			switch results[0].Stdout {
			case "first":
				counts[0].Add(1)
			case "second":
				counts[1].Add(1)
			default:
				errors <- fmt.Errorf("unexpected executor output %q", results[0].Stdout)
			}
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if counts[0].Load() != calls/2 || counts[1].Load() != calls/2 {
		t.Fatalf("executor call counts = [%d %d], want [%d %d]", counts[0].Load(), counts[1].Load(), calls/2, calls/2)
	}
}

func newExecutorTestServer(t *testing.T, output string) *httptest.Server {
	t.Helper()
	runner := &fakeBatchRunner{results: []judge.RunResult{{Stdout: output, Stage: judge.StageRun}}}
	server, err := NewServer(runner, "internal-secret", 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(server.Handler())
}
