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

type RedisStreamsQueue struct {
	client   *redis.Client
	stream   string
	group    string
	consumer string
}

func NewRedisStreamsQueue(client *redis.Client, stream, group, consumer string) *RedisStreamsQueue {
	if stream == "" {
		stream = DefaultJudgeStream
	}
	if group == "" {
		group = DefaultJudgeGroup
	}
	if consumer == "" {
		consumer = "api-dispatcher"
	}
	return &RedisStreamsQueue{
		client:   client,
		stream:   stream,
		group:    group,
		consumer: consumer,
	}
}

func (q *RedisStreamsQueue) Init(ctx context.Context) error {
	err := q.client.XGroupCreateMkStream(ctx, q.stream, q.group, "0").Err()
	if err != nil && !isRedisBusyGroup(err) {
		return err
	}
	return nil
}

func (q *RedisStreamsQueue) Enqueue(ctx context.Context, job domain.Job) error {
	return q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: q.stream,
		Values: redisStreamValues(job),
	}).Err()
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
			return domain.Job{}, err
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				job, err := redisMessageJob(message)
				if err != nil {
					return domain.Job{}, err
				}
				return job, nil
			}
		}
	}
}

func (q *RedisStreamsQueue) Ack(ctx context.Context, job domain.Job) error {
	if job.Receipt == "" {
		return fmt.Errorf("redis stream job missing receipt")
	}
	return q.client.XAck(ctx, q.stream, q.group, job.Receipt).Err()
}

func (q *RedisStreamsQueue) Retry(ctx context.Context, job domain.Job, cause error) (bool, error) {
	if job.Receipt == "" {
		return false, fmt.Errorf("redis stream job missing receipt")
	}
	receipt := job.Receipt
	_, attempts := retryTarget(job)
	deadLettered := attempts >= DefaultMaxAttempts
	target := q.stream
	if deadLettered {
		target = q.stream + ":dead"
	}
	job.Attempts = attempts
	job.Receipt = ""

	pipe := q.client.TxPipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{Stream: target, Values: retryStreamValues(job, cause)})
	pipe.XAck(ctx, q.stream, q.group, receipt)
	_, err := pipe.Exec(ctx)
	return deadLettered, err
}

func (q *RedisStreamsQueue) claimPending(ctx context.Context) (domain.Job, bool, error) {
	messages, _, err := q.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   q.stream,
		Group:    q.group,
		Consumer: q.consumer,
		MinIdle:  30 * time.Second,
		Start:    "0-0",
		Count:    1,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return domain.Job{}, false, err
	}
	if len(messages) == 0 {
		return domain.Job{}, false, nil
	}
	job, err := redisMessageJob(messages[0])
	return job, err == nil, err
}

func redisStreamValues(job domain.Job) map[string]any {
	return map[string]any{
		"submission_id": job.SubmissionID,
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
	return domain.Job{SubmissionID: submissionID, Attempts: attempts}, nil
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

func isRedisBusyGroup(err error) bool {
	return err != nil && len(err.Error()) >= len("BUSYGROUP") && err.Error()[:len("BUSYGROUP")] == "BUSYGROUP"
}
