package judgeworker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/store"
)

var ErrLeaseLost = errors.New("judge lease lost")

type WorkerQueue interface {
	Dequeue(context.Context) (domain.Job, error)
	Touch(context.Context, domain.Job) error
	Ack(context.Context, domain.Job) error
	RetryJob(context.Context, domain.Job, int, error) error
	DeadLetter(context.Context, domain.Job, int, error) error
}

type Judge interface {
	Evaluate(context.Context, domain.Problem, domain.Language, string) (domain.JudgeResult, error)
}

// WorkerMetrics records worker-level observations.
type WorkerMetrics interface {
	WorkerJobStarted()
	WorkerJobFinished(language, result string, duration time.Duration)
	WorkerRetry()
	WorkerDeadLetter()
	WorkerLeaseTakeover()
}

type Config struct {
	WorkerID          string
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	MaxAttempts       int
	Now               func() time.Time
	Token             func() (string, error)
	Metrics           WorkerMetrics
}

type Processor struct {
	store   store.LeaseStore
	queue   WorkerQueue
	judge   Judge
	config  Config
	metrics WorkerMetrics
}

func NewProcessor(st store.LeaseStore, queue WorkerQueue, judge Judge, config Config) *Processor {
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = 30 * time.Second
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 10 * time.Second
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.Token == nil {
		config.Token = randomToken
	}
	return &Processor{store: st, queue: queue, judge: judge, config: config, metrics: config.Metrics}
}

func (p *Processor) Run(ctx context.Context) error {
	return p.RunGraceful(ctx, ctx)
}

func (p *Processor) RunGraceful(acquireCtx, workCtx context.Context) error {
	for {
		job, err := p.queue.Dequeue(acquireCtx)
		if err != nil {
			if acquireCtx.Err() != nil {
				return acquireCtx.Err()
			}
			slog.Warn("judge job failed", "worker_id", p.config.WorkerID, "error", err)
			continue
		}
		if err := p.ProcessJob(workCtx, job); err != nil {
			if workCtx.Err() != nil {
				return workCtx.Err()
			}
			slog.Warn("judge job failed", "worker_id", p.config.WorkerID, "error", err)
		}
	}
}

func (p *Processor) ProcessOne(ctx context.Context) error {
	job, err := p.queue.Dequeue(ctx)
	if err != nil {
		return err
	}
	return p.ProcessJob(ctx, job)
}

func (p *Processor) ProcessJob(ctx context.Context, job domain.Job) error {
	started := p.config.Now()
	token, err := p.config.Token()
	if err != nil {
		return fmt.Errorf("generate judge token: %w", err)
	}
	claim, err := p.store.ClaimSubmission(ctx, job.SubmissionID, p.config.WorkerID, token, job.Receipt, p.config.Now(), p.config.LeaseDuration)
	if err != nil {
		return fmt.Errorf("claim submission: %w", err)
	}
	switch claim.State {
	case domain.ClaimTerminal, domain.ClaimActiveOtherReceipt:
		return p.queue.Ack(ctx, job)
	case domain.ClaimActiveSameReceipt:
		return nil
	case domain.ClaimMissing:
		if p.metrics != nil {
			p.metrics.WorkerDeadLetter()
		}
		return p.queue.DeadLetter(ctx, job, job.Attempts, fmt.Errorf("submission %q not found", job.SubmissionID))
	case domain.ClaimAcquired:
		if p.metrics != nil {
			p.metrics.WorkerJobStarted()
			if claim.LeaseTakeover {
				p.metrics.WorkerLeaseTakeover()
			}
		}
		err := p.processClaim(ctx, job, claim)
		if p.metrics != nil {
			language := string(claim.Submission.Language)
			result := metricResult(claim.Submission, err)
			p.metrics.WorkerJobFinished(language, result, p.config.Now().Sub(started))
		}
		return err
	default:
		return fmt.Errorf("unknown claim state %q", claim.State)
	}
}

func (p *Processor) processClaim(ctx context.Context, job domain.Job, claim domain.SubmissionClaim) error {
	problem, ok, err := p.store.GetProblem(ctx, claim.Submission.ProblemID)
	if err != nil {
		return p.handleInfrastructureError(ctx, job, claim, fmt.Errorf("get problem: %w", err))
	}
	if !ok {
		result := domain.JudgeResult{Status: domain.StatusInternalError, Stderr: "problem not found"}
		completed, err := p.store.CompleteSubmission(ctx, claim.Submission.ID, claim.Token, p.config.Now(), result)
		if err != nil {
			return err
		}
		if !completed {
			return ErrLeaseLost
		}
		return p.queue.Ack(ctx, job)
	}

	result, err := p.evaluateWithHeartbeat(ctx, job, claim, problem)
	if err != nil {
		if errors.Is(err, ErrLeaseLost) {
			return err
		}
		return p.handleInfrastructureError(ctx, job, claim, err)
	}
	completed, err := p.store.CompleteSubmission(ctx, claim.Submission.ID, claim.Token, p.config.Now(), result)
	if err != nil {
		return fmt.Errorf("complete submission: %w", err)
	}
	if !completed {
		return ErrLeaseLost
	}
	if err := p.queue.Ack(ctx, job); err != nil {
		return fmt.Errorf("acknowledge completed submission: %w", err)
	}
	return nil
}

func (p *Processor) evaluateWithHeartbeat(ctx context.Context, job domain.Job, claim domain.SubmissionClaim, problem domain.Problem) (domain.JudgeResult, error) {
	judgeCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	heartbeatResult := make(chan error, 1)
	go func() {
		err := p.heartbeat(judgeCtx, done, job, claim)
		if err != nil {
			cancel()
		}
		heartbeatResult <- err
	}()
	result, evaluateErr := p.judge.Evaluate(judgeCtx, problem, claim.Submission.Language, claim.Submission.Code)
	close(done)
	cancel()
	heartbeatErr := <-heartbeatResult
	if heartbeatErr != nil {
		return domain.JudgeResult{}, heartbeatErr
	}
	if evaluateErr != nil {
		return domain.JudgeResult{}, fmt.Errorf("judge submission: %w", evaluateErr)
	}
	return result, nil
}

func (p *Processor) heartbeat(ctx context.Context, done <-chan struct{}, job domain.Job, claim domain.SubmissionClaim) error {
	ticker := time.NewTicker(p.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			renewed, err := p.store.RenewSubmissionLease(ctx, claim.Submission.ID, claim.Token, p.config.Now(), p.config.LeaseDuration)
			if err != nil {
				return fmt.Errorf("renew judge lease: %w", err)
			}
			if !renewed {
				return ErrLeaseLost
			}
			if err := p.queue.Touch(ctx, job); err != nil {
				return fmt.Errorf("touch pending job: %w", err)
			}
		}
	}
}

func (p *Processor) handleInfrastructureError(ctx context.Context, job domain.Job, claim domain.SubmissionClaim, cause error) error {
	if claim.Attempts >= p.config.MaxAttempts {
		if p.metrics != nil {
			p.metrics.WorkerDeadLetter()
		}
		result := domain.JudgeResult{Status: domain.StatusInternalError, Stderr: cause.Error()}
		completed, err := p.store.CompleteSubmission(ctx, claim.Submission.ID, claim.Token, p.config.Now(), result)
		if err != nil {
			return errors.Join(cause, err)
		}
		if !completed {
			return errors.Join(cause, ErrLeaseLost)
		}
		if err := p.queue.DeadLetter(ctx, job, claim.Attempts, cause); err != nil {
			return errors.Join(cause, err)
		}
		return cause
	}
	if p.metrics != nil {
		p.metrics.WorkerRetry()
	}
	released, err := p.store.ReleaseSubmission(ctx, claim.Submission.ID, claim.Token, p.config.Now(), cause.Error())
	if err != nil {
		return errors.Join(cause, err)
	}
	if !released {
		return errors.Join(cause, ErrLeaseLost)
	}
	if err := p.queue.RetryJob(ctx, job, claim.Attempts, cause); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

// metricResult 将 JudgeResult 状态映射为 worker metrics 的 result 标签值。
func metricResult(sub domain.Submission, processErr error) string {
	if processErr != nil {
		if errors.Is(processErr, ErrLeaseLost) {
			return "lease_lost"
		}
		return "infrastructure_error"
	}
	if sub.Result == nil {
		return "accepted"
	}
	switch sub.Result.Status {
	case domain.StatusAccepted:
		return "accepted"
	case domain.StatusWrongAnswer:
		return "wrong_answer"
	case domain.StatusRuntimeError:
		return "runtime_error"
	case domain.StatusTimeLimitExceeded:
		return "time_limit_exceeded"
	case domain.StatusInternalError:
		return "internal_error"
	default:
		return "accepted"
	}
}

func randomToken() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}
