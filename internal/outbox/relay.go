package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/store"
)

type Publisher interface {
	Enqueue(context.Context, domain.Job) error
}

// Metrics records outbox publication outcomes.
type Metrics interface {
	ObserveOutboxPublish(result string, duration time.Duration)
}

type Config struct {
	RelayID       string
	BatchSize     int
	ClaimDuration time.Duration
	PollInterval  time.Duration
	Metrics       Metrics
}

type Relay struct {
	store     store.OutboxStore
	publisher Publisher
	config    Config
}

func New(st store.OutboxStore, publisher Publisher, config Config) *Relay {
	if config.RelayID == "" {
		config.RelayID = "api-outbox"
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.ClaimDuration <= 0 {
		config.ClaimDuration = 30 * time.Second
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 250 * time.Millisecond
	}
	return &Relay{store: st, publisher: publisher, config: config}
}

func (r *Relay) PublishBatch(ctx context.Context, now time.Time) error {
	events, err := r.store.ClaimOutbox(ctx, r.config.RelayID, now, r.config.ClaimDuration, r.config.BatchSize)
	if err != nil {
		return fmt.Errorf("claim outbox: %w", err)
	}
	var batchErr error
	for _, event := range events {
		started := time.Now()
		job := domain.Job{SubmissionID: event.SubmissionID, OutboxID: event.ID}
		if err := r.publisher.Enqueue(ctx, job); err != nil {
			nextAttempt := now.Add(publishBackoff(event.PublishAttempts))
			if _, markErr := r.store.MarkOutboxFailed(ctx, event.ID, event.ClaimToken, nextAttempt, err.Error()); markErr != nil {
				batchErr = errors.Join(batchErr, fmt.Errorf("publish outbox %d: %w", event.ID, err), fmt.Errorf("mark outbox %d failed: %w", event.ID, markErr))
			} else {
				batchErr = errors.Join(batchErr, fmt.Errorf("publish outbox %d: %w", event.ID, err))
			}
			if r.config.Metrics != nil {
				r.config.Metrics.ObserveOutboxPublish("error", time.Since(started))
			}
			continue
		}
		ok, err := r.store.MarkOutboxPublished(ctx, event.ID, event.ClaimToken, now)
		if err != nil {
			batchErr = errors.Join(batchErr, fmt.Errorf("mark outbox %d published: %w", event.ID, err))
			if r.config.Metrics != nil {
				r.config.Metrics.ObserveOutboxPublish("error", time.Since(started))
			}
		} else if !ok {
			batchErr = errors.Join(batchErr, fmt.Errorf("mark outbox %d published: claim lost", event.ID))
			if r.config.Metrics != nil {
				r.config.Metrics.ObserveOutboxPublish("claim_lost", time.Since(started))
			}
		} else {
			if r.config.Metrics != nil {
				r.config.Metrics.ObserveOutboxPublish("success", time.Since(started))
			}
		}
	}
	return batchErr
}

func (r *Relay) Run(ctx context.Context) error {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			if err := r.PublishBatch(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
				slog.Warn("outbox publish batch failed", "error", err)
			}
			timer.Reset(r.config.PollInterval)
		}
	}
}

func publishBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 250 * time.Millisecond
	for i := 1; i < attempt && delay < 30*time.Second; i++ {
		delay *= 2
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
	}
	return delay
}
