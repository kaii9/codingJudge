package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/queue"
)

func TestMemoryQueueDequeuesSubmittedJob(t *testing.T) {
	t.Parallel()

	q := queue.NewMemoryQueue(1)
	want := domain.Job{SubmissionID: "sub-1"}

	if err := q.Enqueue(context.Background(), want); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}

	got, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("Dequeue returned error: %v", err)
	}
	if got != want {
		t.Fatalf("job = %+v, want %+v", got, want)
	}
}

func TestMemoryQueueReturnsContextErrorWhenEmpty(t *testing.T) {
	t.Parallel()

	q := queue.NewMemoryQueue(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := q.Dequeue(ctx)
	if err == nil {
		t.Fatal("Dequeue should return an error when context is canceled")
	}
}

func TestMemoryQueueRetryMakesJobAvailableAgain(t *testing.T) {
	t.Parallel()

	q := queue.NewMemoryQueue(1)
	want := domain.Job{SubmissionID: "sub-retry"}
	if err := q.Enqueue(context.Background(), want); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	job, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("Dequeue returned error: %v", err)
	}
	if _, err := q.Retry(context.Background(), job, nil); err != nil {
		t.Fatalf("Retry returned error: %v", err)
	}

	got, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("second Dequeue returned error: %v", err)
	}
	if got.SubmissionID != want.SubmissionID || got.Attempts != 1 {
		t.Fatalf("retried job = %+v, want submission %q attempt 1", got, want.SubmissionID)
	}
}
