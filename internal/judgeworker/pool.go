package judgeworker

import (
	"context"
	"sync"
	"time"
)

type Slot interface {
	Run(context.Context) error
}

type Pool struct {
	slots         []Slot
	shutdownGrace time.Duration
}

func NewPool(slots []Slot, shutdownGrace time.Duration) *Pool {
	if shutdownGrace <= 0 {
		shutdownGrace = 30 * time.Second
	}
	return &Pool{slots: slots, shutdownGrace: shutdownGrace}
}

func (p *Pool) Run(ctx context.Context) error {
	workCtx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for _, slot := range p.slots {
		wg.Add(1)
		go func(slot Slot) {
			defer wg.Done()
			_ = slot.Run(workCtx)
		}(slot)
	}
	<-ctx.Done()
	cancel()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(p.shutdownGrace):
	}
	return nil
}
