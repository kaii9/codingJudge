package queue

import (
	"errors"
	"sync"
	"testing"

	"github.com/kai/codingjudge/internal/domain"
)

type fakeQueueMetrics struct {
	mu      sync.Mutex
	actions []string
	results []string
}

func (m *fakeQueueMetrics) ObserveQueueOperation(action, result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actions = append(m.actions, action)
	m.results = append(m.results, result)
}

func TestQueueMetricsOptionCompiles(t *testing.T) {
	// 验证 NewRedisStreamsQueue 选项模式编译通过。
	metrics := &fakeQueueMetrics{}
	q := NewRedisStreamsQueue(nil, "stream", "group", "consumer", WithMetrics(metrics))
	if q == nil {
		t.Fatal("expected non-nil queue")
	}
}

func TestRedisStreamJobMappingRoundTrip(t *testing.T) {
	t.Parallel()

	job := domain.Job{SubmissionID: "sub-42", OutboxID: 7, Attempts: 2}
	values := redisStreamValues(job)
	got, err := redisStreamJob(map[string]any{
		"submission_id": values["submission_id"],
		"outbox_id":     values["outbox_id"],
		"attempt":       values["attempt"],
	})
	if err != nil {
		t.Fatalf("redisStreamJob returned error: %v", err)
	}
	if got != job {
		t.Fatalf("job = %+v, want %+v", got, job)
	}
}

func TestRetryStreamValuesIncludeLastError(t *testing.T) {
	t.Parallel()

	values := retryStreamValues(domain.Job{SubmissionID: "sub-42", Attempts: 2}, errors.New("worker unavailable"))
	if values["last_error"] != "worker unavailable" {
		t.Fatalf("last_error = %#v", values["last_error"])
	}
}

func TestRetryTargetUsesDeadLetterStreamAfterMaximumAttempts(t *testing.T) {
	t.Parallel()

	stream, attempts := retryTarget(domain.Job{Attempts: 1})
	if stream != DefaultJudgeStream || attempts != 2 {
		t.Fatalf("retryTarget before limit = %q, %d", stream, attempts)
	}

	stream, attempts = retryTarget(domain.Job{Attempts: DefaultMaxAttempts - 1})
	if stream != DefaultDeadLetterStream || attempts != DefaultMaxAttempts {
		t.Fatalf("retryTarget at limit = %q, %d", stream, attempts)
	}
}
