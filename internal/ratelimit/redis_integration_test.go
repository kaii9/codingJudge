//go:build integration

package ratelimit

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestRedisTokenBucketEnforcesBurst(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is not set")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	userID := "integration-rate-limit-user"
	key := redisKey(userID)
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })
	if err := client.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}

	limiter, err := NewRedisTokenBucket(client, 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	for remaining := 2; remaining >= 0; remaining-- {
		decision, err := limiter.Allow(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if !decision.Allowed || decision.Remaining != remaining {
			t.Fatalf("decision = %+v, want allowed with remaining=%d", decision, remaining)
		}
	}
	decision, err := limiter.Allow(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.RetryAfter <= 0 {
		t.Fatalf("decision = %+v, want rejected with retry delay", decision)
	}
}
