package judgeworker_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kai/codingjudge/internal/judgeworker"
)

type fakeSlot struct {
	started chan struct{}
	active  atomic.Int32
}

func (s *fakeSlot) Run(ctx context.Context) error {
	s.active.Add(1)
	select {
	case s.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	s.active.Add(-1)
	return ctx.Err()
}

func TestPoolStartsEverySlotAndStopsOnCancellation(t *testing.T) {
	started := make(chan struct{}, 2)
	one := &fakeSlot{started: started}
	two := &fakeSlot{started: started}
	pool := judgeworker.NewPool([]judgeworker.Slot{one, two}, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- pool.Run(ctx) }()
	<-started
	<-started
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if one.active.Load() != 0 || two.active.Load() != 0 {
		t.Fatal("pool left active slots")
	}
}
