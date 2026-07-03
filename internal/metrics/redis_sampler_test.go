package metrics

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeFetcher struct {
	mu    sync.Mutex
	count int64
	err   error
}

func (f *fakeFetcher) XPending(_ context.Context, _, _ string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count, f.err
}

func TestSamplerSetsGauge(t *testing.T) {
	fetcher := &fakeFetcher{count: 42}
	var mu sync.Mutex
	var gaugeValue float64

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go SamplePending(ctx, fetcher, "stream", "group", func(v float64) {
		mu.Lock()
		defer mu.Unlock()
		gaugeValue = v
	}, 10*time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if gaugeValue != 42 {
		t.Errorf("gauge = %v, want 42", gaugeValue)
	}
}

func TestSamplerExitsOnCancel(t *testing.T) {
	fetcher := &fakeFetcher{count: 1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消。

	done := make(chan struct{})
	go func() {
		SamplePending(ctx, fetcher, "stream", "group", func(float64) {}, time.Hour)
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(time.Second):
		t.Fatal("sampler did not exit on cancel")
	}
}

func TestSamplerHandlesError(t *testing.T) {
	fetcher := &fakeFetcher{err: errors.New("redis down")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go SamplePending(ctx, fetcher, "stream", "group", func(float64) {
		t.Error("gauge should not be called on error")
	}, 10*time.Millisecond)

	time.Sleep(30 * time.Millisecond)
	cancel()
}
