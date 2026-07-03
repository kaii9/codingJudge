package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// PendingFetcher abstracts the Redis XPENDING command.
type PendingFetcher interface {
	XPending(ctx context.Context, stream, group string) (int64, error)
}

// RedisPendingFetcher adapts a redis.Client to PendingFetcher.
type RedisPendingFetcher struct {
	Client *redis.Client
}

func (f RedisPendingFetcher) XPending(ctx context.Context, stream, group string) (int64, error) {
	result, err := f.Client.XPending(ctx, stream, group).Result()
	if err != nil {
		return 0, err
	}
	return result.Count, nil
}

// SamplePending periodically fetches the pending count from a Redis stream
// and updates the gauge. Errors are logged and the gauge is not updated.
func SamplePending(ctx context.Context, fetcher PendingFetcher, stream, group string, gauge func(float64), interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := fetcher.XPending(ctx, stream, group)
			if err != nil {
				slog.Warn("pending sampler failed", "error", err)
				continue
			}
			gauge(float64(count))
		}
	}
}
