package queue

import (
	"context"

	"github.com/kai/codingjudge/internal/domain"
)

type MemoryQueue struct {
	jobs chan domain.Job
}

func NewMemoryQueue(buffer int) *MemoryQueue {
	return &MemoryQueue{jobs: make(chan domain.Job, buffer)}
}

func (q *MemoryQueue) Enqueue(ctx context.Context, job domain.Job) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.jobs <- job:
		return nil
	}
}

func (q *MemoryQueue) Dequeue(ctx context.Context) (domain.Job, error) {
	select {
	case <-ctx.Done():
		return domain.Job{}, ctx.Err()
	case job := <-q.jobs:
		return job, nil
	}
}

func (q *MemoryQueue) Ack(context.Context, domain.Job) error {
	return nil
}

func (q *MemoryQueue) Touch(context.Context, domain.Job) error {
	return nil
}

func (q *MemoryQueue) RetryJob(ctx context.Context, job domain.Job, attempt int, _ error) error {
	job.Attempts = attempt
	return q.Enqueue(ctx, job)
}

func (q *MemoryQueue) DeadLetter(context.Context, domain.Job, int, error) error {
	return nil
}

func (q *MemoryQueue) Retry(ctx context.Context, job domain.Job, _ error) (bool, error) {
	job.Attempts++
	return false, q.Enqueue(ctx, job)
}
