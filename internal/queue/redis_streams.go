package queue

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/redis/go-redis/v9"
)

const (
	DefaultJudgeStream      = "judge:submissions"
	DefaultDeadLetterStream = "judge:submissions:dead"
	DefaultJudgeGroup       = "judge-workers"
	DefaultMaxAttempts      = 3
)

// Metrics records Redis stream queue operations.
type Metrics interface {
	ObserveQueueOperation(action, result string)
}

// RedisOption configures a RedisStreamsQueue.
type RedisOption func(*RedisStreamsQueue)

// WithMetrics sets the queue operation recorder. When nil, no metrics are recorded.
func WithMetrics(m Metrics) RedisOption {
	return func(q *RedisStreamsQueue) {
		q.metrics = m
	}
}

type RedisStreamsQueue struct {
	client       *redis.Client
	stream       string
	group        string
	consumer     string
	claimMinIdle time.Duration
	metrics      Metrics
}

func NewRedisStreamsQueue(client *redis.Client, stream, group, consumer string, options ...RedisOption) *RedisStreamsQueue {
	if stream == "" {
		stream = DefaultJudgeStream
	}
	if group == "" {
		group = DefaultJudgeGroup
	}
	if consumer == "" {
		consumer = "judge-worker"
	}
	q := &RedisStreamsQueue{
		client:       client,
		stream:       stream,
		group:        group,
		consumer:     consumer,
		claimMinIdle: 30 * time.Second,
	}
	for _, opt := range options {
		opt(q)
	}
	return q
}

func (q *RedisStreamsQueue) Init(ctx context.Context) error {
	err := q.client.XGroupCreateMkStream(ctx, q.stream, q.group, "0").Err()
	if err != nil && !isRedisBusyGroup(err) {
		return err
	}
	return nil
}

func (q *RedisStreamsQueue) Enqueue(ctx context.Context, job domain.Job) error {
	err := q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: q.stream,
		Values: redisStreamValues(job),
	}).Err()
	q.record("enqueue", err)
	return err
}

func (q *RedisStreamsQueue) Dequeue(ctx context.Context) (domain.Job, error) {
	for {
		if job, ok, err := q.claimPending(ctx); err != nil {
			return domain.Job{}, err
		} else if ok {
			return job, nil
		}

		streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    q.group,
			Consumer: q.consumer,
			Streams:  []string{q.stream, ">"},
			Count:    1,
			Block:    time.Second,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			q.record("dequeue", err)
			return domain.Job{}, err
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				job, err := redisMessageJob(message)
				if err != nil {
					if deadErr := q.deadLetterMalformed(ctx, message, err); deadErr != nil {
						return domain.Job{}, errors.Join(err, deadErr)
					}
					continue
				}
				q.record("dequeue", nil)
				return job, nil
			}
		}
	}
}

func (q *RedisStreamsQueue) Touch(ctx context.Context, job domain.Job) error {
	if job.Receipt == "" {
		return fmt.Errorf("redis stream job missing receipt")
	}
	messages, err := q.client.XClaim(ctx, &redis.XClaimArgs{
		Stream: q.stream, Group: q.group, Consumer: q.consumer,
		MinIdle: 0, Messages: []string{job.Receipt},
	}).Result()
	q.record("touch", err)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		return fmt.Errorf("redis stream job %q no longer pending", job.Receipt)
	}
	return nil
}

func (q *RedisStreamsQueue) Ack(ctx context.Context, job domain.Job) error {
	if job.Receipt == "" {
		return fmt.Errorf("redis stream job missing receipt")
	}
	err := q.client.XAck(ctx, q.stream, q.group, job.Receipt).Err()
	q.record("ack", err)
	return err
}

func (q *RedisStreamsQueue) Retry(ctx context.Context, job domain.Job, cause error) (bool, error) {
	if job.Receipt == "" {
		return false, fmt.Errorf("redis stream job missing receipt")
	}
	_, attempts := retryTarget(job)
	deadLettered := attempts >= DefaultMaxAttempts
	if deadLettered {
		return true, q.DeadLetter(ctx, job, attempts, cause)
	}
	return false, q.RetryJob(ctx, job, attempts, cause)
}

func (q *RedisStreamsQueue) RetryJob(ctx context.Context, job domain.Job, attempt int, cause error) error {
	err := q.moveAndAck(ctx, q.stream, job, attempt, cause)
	// 记录 retry 指标——覆盖所有 RetryJob 调用路径（包括 processor 直接调用）。
	if q.metrics != nil {
		result := "success"
		if err != nil {
			result = "error"
		}
		q.metrics.ObserveQueueOperation("retry", result)
	}
	return err
}

func (q *RedisStreamsQueue) DeadLetter(ctx context.Context, job domain.Job, attempt int, cause error) error {
	err := q.moveAndAck(ctx, q.stream+":dead", job, attempt, cause)
	q.record("dead_letter", err)
	return err
}

func (q *RedisStreamsQueue) moveAndAck(ctx context.Context, target string, job domain.Job, attempt int, cause error) error {
	if job.Receipt == "" {
		return fmt.Errorf("redis stream job missing receipt")
	}
	receipt := job.Receipt
	job.Attempts = attempt
	job.Receipt = ""

	pipe := q.client.TxPipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{Stream: target, Values: retryStreamValues(job, cause)})
	pipe.XAck(ctx, q.stream, q.group, receipt)
	_, err := pipe.Exec(ctx)
	return err
}

func (q *RedisStreamsQueue) claimPending(ctx context.Context) (domain.Job, bool, error) {
	messages, _, err := q.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   q.stream,
		Group:    q.group,
		Consumer: q.consumer,
		MinIdle:  q.claimMinIdle,
		Start:    "0-0",
		Count:    1,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		q.record("claim_pending", err)
		return domain.Job{}, false, err
	}
	if len(messages) == 0 {
		return domain.Job{}, false, nil
	}
	job, err := redisMessageJob(messages[0])
	if err != nil {
		if deadErr := q.deadLetterMalformed(ctx, messages[0], err); deadErr != nil {
			return domain.Job{}, false, errors.Join(err, deadErr)
		}
		return domain.Job{}, false, nil
	}
	q.record("claim_pending", nil)
	return job, true, nil
}

func (q *RedisStreamsQueue) deadLetterMalformed(ctx context.Context, message redis.XMessage, cause error) error {
	values := make(map[string]any, len(message.Values)+1)
	for key, value := range message.Values {
		values[key] = value
	}
	values["parse_error"] = cause.Error()
	pipe := q.client.TxPipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{Stream: q.stream + ":dead", Values: values})
	pipe.XAck(ctx, q.stream, q.group, message.ID)
	_, err := pipe.Exec(ctx)
	q.record("dead_letter_malformed", err)
	return err
}

func redisStreamValues(job domain.Job) map[string]any {
	return map[string]any{
		"submission_id": job.SubmissionID,
		"outbox_id":     job.OutboxID,
		"attempt":       job.Attempts,
	}
}

func retryStreamValues(job domain.Job, cause error) map[string]any {
	values := redisStreamValues(job)
	if cause != nil {
		values["last_error"] = cause.Error()
	}
	return values
}

func redisStreamJob(values map[string]any) (domain.Job, error) {
	raw, ok := values["submission_id"]
	if !ok {
		return domain.Job{}, fmt.Errorf("redis stream job missing submission_id")
	}
	submissionID, ok := raw.(string)
	if !ok || submissionID == "" {
		return domain.Job{}, fmt.Errorf("redis stream job has invalid submission_id")
	}
	attempts, err := redisStreamAttempts(values["attempt"])
	if err != nil {
		return domain.Job{}, err
	}
	outboxID, err := redisStreamInt64(values["outbox_id"])
	if err != nil {
		return domain.Job{}, fmt.Errorf("redis stream job has invalid outbox_id: %w", err)
	}
	return domain.Job{SubmissionID: submissionID, OutboxID: outboxID, Attempts: attempts}, nil
}

func redisStreamInt64(raw any) (int64, error) {
	if raw == nil {
		return 0, nil
	}
	switch value := raw.(type) {
	case int:
		return int64(value), nil
	case int64:
		return value, nil
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			return 0, fmt.Errorf("invalid non-negative integer")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("invalid integer type %T", raw)
	}
}

func redisMessageJob(message redis.XMessage) (domain.Job, error) {
	job, err := redisStreamJob(message.Values)
	if err != nil {
		return domain.Job{}, err
	}
	job.Receipt = message.ID
	return job, nil
}

func redisStreamAttempts(raw any) (int, error) {
	if raw == nil {
		return 0, nil
	}
	switch value := raw.(type) {
	case int:
		return value, nil
	case int64:
		return int(value), nil
	case string:
		attempts, err := strconv.Atoi(value)
		if err != nil || attempts < 0 {
			return 0, fmt.Errorf("redis stream job has invalid attempt")
		}
		return attempts, nil
	default:
		return 0, fmt.Errorf("redis stream job has invalid attempt")
	}
}

func retryTarget(job domain.Job) (string, int) {
	attempts := job.Attempts + 1
	if attempts >= DefaultMaxAttempts {
		return DefaultDeadLetterStream, attempts
	}
	return DefaultJudgeStream, attempts
}

func (q *RedisStreamsQueue) record(action string, err error) {
	if q.metrics == nil {
		return
	}
	result := "success"
	if err != nil {
		result = "error"
	}
	q.metrics.ObserveQueueOperation(action, result)
}

func isRedisBusyGroup(err error) bool {
	return err != nil && len(err.Error()) >= len("BUSYGROUP") && err.Error()[:len("BUSYGROUP")] == "BUSYGROUP"
}
