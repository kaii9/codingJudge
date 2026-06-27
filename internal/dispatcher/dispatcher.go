package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kai/codingjudge/internal/domain"
)

type Store interface {
	GetProblem(context.Context, string) (domain.Problem, bool, error)
	GetSubmission(context.Context, string) (domain.Submission, bool, error)
	UpdateSubmissionStatus(context.Context, string, domain.SubmissionStatus) error
	UpdateSubmissionResult(context.Context, string, domain.JudgeResult) error
}

type Queue interface {
	Dequeue(context.Context) (domain.Job, error)
	Ack(context.Context, domain.Job) error
	Retry(context.Context, domain.Job, error) (bool, error)
}

type JudgeClient interface {
	Judge(context.Context, JudgeRequest) (domain.JudgeResult, error)
}

type JudgeRequest struct {
	SubmissionID string          `json:"submissionId"`
	Problem      domain.Problem  `json:"problem"`
	Language     domain.Language `json:"language"`
	Code         string          `json:"code"`
}

type Dispatcher struct {
	store  Store
	queue  Queue
	client JudgeClient
}

func New(store Store, queue Queue, client JudgeClient) *Dispatcher {
	return &Dispatcher{store: store, queue: queue, client: client}
}

func (d *Dispatcher) Run(ctx context.Context) error {
	for {
		if err := d.ProcessOne(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("judge dispatch failed", "error", err)
		}
	}
}

func (d *Dispatcher) ProcessOne(ctx context.Context) error {
	job, err := d.queue.Dequeue(ctx)
	if err != nil {
		return err
	}
	if err := d.processJob(ctx, job); err != nil {
		deadLettered, retryErr := d.queue.Retry(ctx, job, err)
		if retryErr != nil {
			return errors.Join(err, fmt.Errorf("retry job: %w", retryErr))
		}
		if deadLettered {
			result := domain.JudgeResult{Status: domain.StatusInternalError, Stderr: err.Error()}
			if resultErr := d.store.UpdateSubmissionResult(ctx, job.SubmissionID, result); resultErr != nil {
				return errors.Join(err, fmt.Errorf("store dead-letter result: %w", resultErr))
			}
		} else if statusErr := d.store.UpdateSubmissionStatus(ctx, job.SubmissionID, domain.StatusQueued); statusErr != nil {
			return errors.Join(err, fmt.Errorf("reset submission status: %w", statusErr))
		}
		return err
	}
	if err := d.queue.Ack(ctx, job); err != nil {
		return fmt.Errorf("acknowledge job: %w", err)
	}
	return nil
}

func (d *Dispatcher) processJob(ctx context.Context, job domain.Job) error {
	sub, ok, err := d.store.GetSubmission(ctx, job.SubmissionID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("submission %q not found", job.SubmissionID)
	}
	if isTerminalStatus(sub.Status) {
		return nil
	}
	if err := d.store.UpdateSubmissionStatus(ctx, sub.ID, domain.StatusRunning); err != nil {
		return err
	}

	problem, ok, err := d.store.GetProblem(ctx, sub.ProblemID)
	if err != nil {
		return err
	}
	if !ok {
		result := domain.JudgeResult{Status: domain.StatusInternalError, Stderr: "problem not found"}
		return d.store.UpdateSubmissionResult(ctx, sub.ID, result)
	}

	result, err := d.client.Judge(ctx, JudgeRequest{
		SubmissionID: sub.ID,
		Problem:      problem,
		Language:     sub.Language,
		Code:         sub.Code,
	})
	if err != nil {
		return fmt.Errorf("judge submission: %w", err)
	}
	return d.store.UpdateSubmissionResult(ctx, sub.ID, result)
}

func isTerminalStatus(status domain.SubmissionStatus) bool {
	switch status {
	case domain.StatusAccepted,
		domain.StatusWrongAnswer,
		domain.StatusRuntimeError,
		domain.StatusTimeLimitExceeded,
		domain.StatusInternalError:
		return true
	default:
		return false
	}
}
