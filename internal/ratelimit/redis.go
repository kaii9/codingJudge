package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Decision describes the state of a user's submission token bucket.
type Decision struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

// Limiter controls how frequently a user may create new submissions.
type Limiter interface {
	Allow(context.Context, string) (Decision, error)
}

// RedisTokenBucket implements an atomic, per-user token bucket in Redis.
type RedisTokenBucket struct {
	client        redis.Scripter
	ratePerMinute int
	burst         int
}

var tokenBucketScript = redis.NewScript(`
local clock = redis.call('TIME')
local now_ms = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local rate_per_minute = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local refill_per_ms = rate_per_minute / 60000

local values = redis.call('HMGET', KEYS[1], 'tokens', 'updated_at_ms')
local tokens = tonumber(values[1])
local updated_at_ms = tonumber(values[2])
if tokens == nil or updated_at_ms == nil then
  tokens = capacity
  updated_at_ms = now_ms
end
if now_ms < updated_at_ms then
  now_ms = updated_at_ms
end

tokens = math.min(capacity, tokens + (now_ms - updated_at_ms) * refill_per_ms)
local allowed = 0
local retry_after_ms = 0
if tokens >= 1 then
  allowed = 1
  tokens = tokens - 1
else
  retry_after_ms = math.ceil((1 - tokens) / refill_per_ms)
end

redis.call('HSET', KEYS[1], 'tokens', tostring(tokens), 'updated_at_ms', tostring(now_ms))
local ttl_ms = math.max(60000, math.ceil((capacity / refill_per_ms) * 2))
redis.call('PEXPIRE', KEYS[1], ttl_ms)
return {allowed, math.floor(tokens), retry_after_ms}
`)

func NewRedisTokenBucket(client redis.Scripter, ratePerMinute, burst int) (*RedisTokenBucket, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if ratePerMinute < 1 || burst < 1 {
		return nil, fmt.Errorf("rate and burst must be positive")
	}
	return &RedisTokenBucket{client: client, ratePerMinute: ratePerMinute, burst: burst}, nil
}

func (l *RedisTokenBucket) Allow(ctx context.Context, userID string) (Decision, error) {
	values, err := tokenBucketScript.Run(
		ctx,
		l.client,
		[]string{redisKey(userID)},
		l.ratePerMinute,
		l.burst,
	).Int64Slice()
	if err != nil {
		return Decision{}, err
	}
	if len(values) != 3 {
		return Decision{}, fmt.Errorf("unexpected token bucket response length %d", len(values))
	}
	return Decision{
		Allowed:    values[0] == 1,
		Limit:      l.ratePerMinute,
		Remaining:  int(values[1]),
		RetryAfter: time.Duration(values[2]) * time.Millisecond,
	}, nil
}

func redisKey(userID string) string {
	sum := sha256.Sum256([]byte(userID))
	return "codingjudge:submission-rate:" + hex.EncodeToString(sum[:])
}
