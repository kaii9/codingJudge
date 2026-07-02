//go:build integration

package queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/redis/go-redis/v9"
)

func integrationQueue(t *testing.T, consumer string) (*RedisStreamsQueue, *redis.Client) {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is not set")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	stream := fmt.Sprintf("judge:test:%d", time.Now().UnixNano())
	q := NewRedisStreamsQueue(client, stream, "workers", consumer)
	q.claimMinIdle = 20 * time.Millisecond
	if err := q.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Del(context.Background(), stream, stream+":dead")
		client.Close()
	})
	return q, client
}

func TestRedisTouchRetryAndPoisonHandling(t *testing.T) {
	ctx := context.Background()
	q, client := integrationQueue(t, "worker-a")
	if err := q.Enqueue(ctx, domain.Job{SubmissionID: "sub-1", OutboxID: 9}); err != nil {
		t.Fatal(err)
	}
	job, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Touch(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := q.RetryJob(ctx, job, 2, errors.New("docker unavailable")); err != nil {
		t.Fatal(err)
	}
	retried, err := q.Dequeue(ctx)
	if err != nil || retried.Attempts != 2 || retried.Receipt == job.Receipt {
		t.Fatalf("retried = %+v, %v", retried, err)
	}

	if _, err := client.XAdd(ctx, &redis.XAddArgs{Stream: q.stream, Values: map[string]any{"attempt": 0}}).Result(); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(ctx, domain.Job{SubmissionID: "sub-2"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Ack(ctx, retried); err != nil {
		t.Fatal(err)
	}
	valid, err := q.Dequeue(ctx)
	if err != nil || valid.SubmissionID != "sub-2" {
		t.Fatalf("valid after poison = %+v, %v", valid, err)
	}
	dead, err := client.XLen(ctx, q.stream+":dead").Result()
	if err != nil || dead != 1 {
		t.Fatalf("dead length = %d, %v", dead, err)
	}
}

func TestRedisTouchDelaysAutoClaim(t *testing.T) {
	ctx := context.Background()
	q1, client := integrationQueue(t, "worker-a")
	q2 := NewRedisStreamsQueue(client, q1.stream, q1.group, "worker-b")
	q2.claimMinIdle = 20 * time.Millisecond
	if err := q1.Enqueue(ctx, domain.Job{SubmissionID: "sub-1"}); err != nil {
		t.Fatal(err)
	}
	job, err := q1.Dequeue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Millisecond)
	if err := q1.Touch(ctx, job); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := q2.claimPending(ctx); err != nil || ok {
		t.Fatalf("early claim = %v, %v", ok, err)
	}
	time.Sleep(25 * time.Millisecond)
	reclaimed, ok, err := q2.claimPending(ctx)
	if err != nil || !ok || reclaimed.Receipt != job.Receipt {
		t.Fatalf("reclaimed = %+v, %v, %v", reclaimed, ok, err)
	}
}
