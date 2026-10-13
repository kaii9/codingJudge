package executor

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errOverloaded = errors.New("executor capacity exhausted")
var errDraining = errors.New("executor is draining")

type admission struct {
	slots       chan struct{}
	waiters     chan struct{}
	waitTimeout time.Duration
	mu          sync.Mutex
	draining    bool
	done        chan struct{}
	workCtx     context.Context
	cancel      context.CancelFunc
	active      sync.WaitGroup
}

func newAdmission(slots, waiters int, timeout time.Duration) *admission {
	ctx, cancel := context.WithCancel(context.Background())
	return &admission{slots: make(chan struct{}, slots), waiters: make(chan struct{}, waiters), waitTimeout: timeout, done: make(chan struct{}), workCtx: ctx, cancel: cancel}
}
func WithAdmission(waiters int, timeout time.Duration) ServerOption {
	return func(s *Server) {
		if waiters < 0 {
			waiters = 0
		}
		if timeout <= 0 {
			timeout = time.Second
		}
		s.admission = newAdmission(cap(s.slots), waiters, timeout)
	}
}
func (a *admission) acquire(ctx context.Context) (context.Context, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	select {
	case <-a.done:
		return nil, nil, errDraining
	default:
	}
	select {
	case a.slots <- struct{}{}:
	default:
		select {
		case a.waiters <- struct{}{}:
		default:
			return nil, nil, errOverloaded
		}
		timer := time.NewTimer(a.waitTimeout)
		defer timer.Stop()
		select {
		case a.slots <- struct{}{}:
		case <-ctx.Done():
			<-a.waiters
			return nil, nil, ctx.Err()
		case <-a.done:
			<-a.waiters
			return nil, nil, errDraining
		case <-timer.C:
			<-a.waiters
			return nil, nil, errOverloaded
		}
		<-a.waiters
	}
	a.mu.Lock()
	if a.draining {
		a.mu.Unlock()
		<-a.slots
		return nil, nil, errDraining
	}
	a.active.Add(1)
	a.mu.Unlock()
	workCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(a.workCtx, cancel)
	return workCtx, func() { stop(); cancel(); <-a.slots; a.active.Done() }, nil
}
func (a *admission) drain(ctx context.Context) error {
	a.mu.Lock()
	if !a.draining {
		a.draining = true
		close(a.done)
	}
	a.mu.Unlock()
	done := make(chan struct{})
	go func() { a.active.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		a.cancel()
		return ctx.Err()
	}
}
