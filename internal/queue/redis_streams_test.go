package queue

import (
	"errors"
	"testing"

	"github.com/kai/codingjudge/internal/domain"
)

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
