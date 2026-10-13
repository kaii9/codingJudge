package executor

import (
	"context"
	"github.com/kaii9/codingJudge/internal/judge"
	"net/http/httptest"
	"testing"
	"time"
)

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
