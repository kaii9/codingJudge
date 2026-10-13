package platform

import (
	"context"
	"encoding/json"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/queue"
	"log/slog"
	"time"
)

const CompletionStream = "judge:results"
const CompletionGroup = "platform-projectors"

type ProjectionStore interface {
	ApplyCompletionEvent(context.Context, domain.CompletionEvent) error
}

func RunProjector(ctx context.Context, st ProjectionStore, q *queue.RedisStreamsQueue) error {
	if err := q.Init(ctx); err != nil {
		return err
	}
	for {
		job, err := q.Dequeue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("completion dequeue failed", "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
			continue
		}
		var event domain.CompletionEvent
		if err := json.Unmarshal([]byte(job.Payload), &event); err != nil || event.ID == "" {
			if err := q.DeadLetter(ctx, job, job.Attempts, err); err != nil {
				slog.Warn("malformed completion dead letter failed", "error", err)
			}
			continue
		}
		if err := st.ApplyCompletionEvent(ctx, event); err != nil {
			slog.Warn("completion projection failed", "submission_id", job.SubmissionID, "error", err)
			continue
		}
		if err := q.Ack(ctx, job); err != nil {
			slog.Warn("completion ack failed", "error", err)
		}
	}
}
