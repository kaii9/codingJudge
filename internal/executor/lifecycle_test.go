package executor

import (
	"context"
	"errors"
	"github.com/kaii9/codingJudge/internal/judge"
	"net/http/httptest"
	"testing"
	"time"
)

func TestForcedDrainReturnsRetryableFailureAndUsesHealthyPeer(t *testing.T) {
	started := make(chan struct{})
	core, _, a := grpcFixture(t, runnerFunc(func(ctx context.Context, _ judge.RunRequest, _ []string) ([]judge.RunResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	peer, _, b := grpcFixture(t, runnerFunc(func(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error) {
		return []judge.RunResult{{Stdout: "3", Stage: judge.StageRun}}, nil
	}))
	peer.SetServing(false)
	pool, err := NewGRPCClientPool([]string{a, b}, "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	request := judge.RunRequest{Language: "python", Code: "print(3)", TimeLimitMS: 1000, MemoryLimitMB: 64}
	call := make(chan error, 1)
	go func() { _, err := pool.RunBatch(context.Background(), request, []string{""}); call <- err }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first executor was not selected")
	}
	peer.SetServing(true)
	drainCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := core.Drain(drainCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("drain=%v", err)
	}
	var failure *CallError
	if err := <-call; !errors.As(err, &failure) || !failure.Retryable() {
		t.Fatalf("server drain must not be permanent caller cancellation: %v", err)
	}
	// This is an explicit new attempt, not transparent replay of RunBatch.
	runs, err := pool.RunBatch(context.Background(), request, []string{""})
	if err != nil || len(runs) != 1 || runs[0].Stdout != "3" {
		t.Fatalf("healthy peer not used: %+v %v", runs, err)
	}
}

func TestCanceledWaiterReleasesCapacityBeforeRunnerAdmission(t *testing.T) {
	a := newAdmission(1, 1, time.Second)
	_, release, err := a.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	a.mu.Lock() // Hold the admission commit until the waiter has acquired a slot.
	go func() {
		_, done, err := a.acquire(ctx)
		if done != nil {
			done()
		}
		result <- err
	}()
	until := time.Now().Add(time.Second)
	for len(a.waiters) != 1 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if len(a.waiters) != 1 {
		a.mu.Unlock()
		release()
		t.Fatal("waiter did not enqueue")
	}
	release()
	for len(a.waiters) != 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	a.mu.Unlock()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter admitted: %v", err)
	}
	if len(a.slots) != 0 || len(a.waiters) != 0 {
		t.Fatal("canceled waiter leaked capacity")
	}
}

func TestDrainWaitsForInFlightAndRejectsNewWork(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	core, _, addr := grpcFixture(t, runnerFunc(func(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error) {
		close(started)
		<-finish
		return []judge.RunResult{{Stage: judge.StageRun}}, nil
	}))
	client := directClient(t, addr)
	call := make(chan error, 1)
	go func() { _, err := client.RunBatch(authorized(context.Background()), batchFixture()); call <- err }()
	<-started
	drained := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { drained <- core.Drain(ctx) }()
	for core.serving.Load() {
		time.Sleep(time.Millisecond)
	}
	rec := httptest.NewRecorder()
	core.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 503 {
		t.Fatalf("readiness=%d", rec.Code)
	}
	select {
	case <-drained:
		t.Fatal("drain returned before in-flight request finished")
	default:
	}
	close(finish)
	if err := <-call; err != nil {
		t.Fatal(err)
	}
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
}

func TestDrainDeadlineCancelsInFlight(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	core, _, addr := grpcFixture(t, runnerFunc(func(ctx context.Context, _ judge.RunRequest, _ []string) ([]judge.RunResult, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	}))
	client := directClient(t, addr)
	go func() { _, _ = client.RunBatch(authorized(context.Background()), batchFixture()) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := core.Drain(ctx); err == nil {
		t.Fatal("expected grace expiry")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("in-flight work not canceled")
	}
}

func TestDrainCannotBeReactivatedByHealthProbe(t *testing.T) {
	core, _, _ := grpcFixture(t, runnerFunc(func(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error) { return nil, nil }))
	if err := core.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	core.SetServing(true)
	if core.serving.Load() {
		t.Fatal("health probe revived draining executor")
	}
}
