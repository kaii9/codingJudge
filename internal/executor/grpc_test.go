package executor

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/kaii9/codingJudge/api/gen/executor/v1"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/judge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func grpcFixture(t *testing.T, runner judge.BatchRunner, options ...ServerOption) (*Server, *grpc.Server, string) {
	t.Helper()
	core, err := NewServer(runner, "secret", 1, nil, nil, options...)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := NewGRPCServer(core)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); listener.Close() })
	return core, server, listener.Addr().String()
}
func batchFixture() *pb.RunBatchRequest {
	return &pb.RunBatchRequest{Language: "python", Code: "print(3)", Inputs: []string{"1 2"}, TimeLimitMs: 1000, MemoryLimitMb: 64}
}
func directClient(t *testing.T, addr string) pb.ExecutorServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return pb.NewExecutorServiceClient(conn)
}
func authorized(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer secret")
}

func TestGRPCRejectsUnauthenticatedAndInvalidBatch(t *testing.T) {
	var calls atomic.Int64
	_, _, addr := grpcFixture(t, runnerFunc(func(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error) {
		calls.Add(1)
		return []judge.RunResult{{Stdout: "3", Stage: judge.StageRun}}, nil
	}))
	client := directClient(t, addr)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.RunBatch(ctx, batchFixture()); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthenticated: %v", err)
	}
	req := batchFixture()
	req.Inputs = nil
	if _, err := client.RunBatch(authorized(ctx), req); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("runner called %d times", calls.Load())
	}
}
func TestGRPCDeadlineCancelsActualRunner(t *testing.T) {
	canceled := make(chan struct{})
	_, _, addr := grpcFixture(t, runnerFunc(func(ctx context.Context, _ judge.RunRequest, _ []string) ([]judge.RunResult, error) {
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	}))
	client := directClient(t, addr)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, err := client.RunBatch(authorized(ctx), batchFixture()); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("deadline: %v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("runner was not canceled")
	}
}
func TestGRPCBoundedAdmissionAndDrain(t *testing.T) {
	started := make(chan struct{}, 1)
	_, _, addr := grpcFixture(t, runnerFunc(func(ctx context.Context, _ judge.RunRequest, _ []string) ([]judge.RunResult, error) {
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}), WithAdmission(0, time.Second))
	client := directClient(t, addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.RunBatch(authorized(ctx), batchFixture()); done <- err }()
	<-started
	callCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if _, err := client.RunBatch(authorized(callCtx), batchFixture()); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected overload, got %v", err)
	}
	cancel()
	<-done
}
func TestGRPCPoolSkipsUnhealthyAndRecovers(t *testing.T) {
	first, _, a := grpcFixture(t, runnerFunc(func(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error) {
		return []judge.RunResult{{Stdout: "A", Stage: judge.StageRun}}, nil
	}))
	_, _, b := grpcFixture(t, runnerFunc(func(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error) {
		return []judge.RunResult{{Stdout: "B", Stage: judge.StageRun}}, nil
	}))
	first.SetServing(false)
	pool, err := NewGRPCClientPool([]string{a, b}, "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	request := judge.RunRequest{Language: domain.LanguagePython, Code: "print(3)", TimeLimitMS: 1000, MemoryLimitMB: 64}
	for range 6 {
		runs, err := pool.RunBatch(context.Background(), request, []string{"1 2"})
		if err != nil || len(runs) != 1 || runs[0].Stdout != "B" {
			t.Fatalf("unhealthy selection: %+v %v", runs, err)
		}
	}
	first.SetServing(true)
	seenA := false
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		runs, err := pool.RunBatch(context.Background(), request, []string{"1 2"})
		if err == nil && runs[0].Stdout == "A" {
			seenA = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !seenA {
		t.Fatal("healthy endpoint did not recover")
	}
}
func TestGRPCExecutionErrorNeverAutomaticallyReplays(t *testing.T) {
	var calls atomic.Int64
	_, _, addr := grpcFixture(t, runnerFunc(func(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error) {
		calls.Add(1)
		return nil, errors.New("execution response unavailable")
	}))
	pool, err := NewGRPCClientPool([]string{addr}, "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.RunBatch(context.Background(), judge.RunRequest{Language: domain.LanguagePython, Code: "print(3)", TimeLimitMS: 1000, MemoryLimitMB: 64}, []string{"1 2"})
	var callErr *CallError
	if !errors.As(err, &callErr) || calls.Load() != 1 {
		t.Fatalf("calls=%d error=%v", calls.Load(), err)
	}
}

func TestGRPCLostResponseDoesNotReplaySuccessfulExecution(t *testing.T) {
	var calls atomic.Int64
	core, err := NewServer(runnerFunc(func(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error) {
		calls.Add(1)
		return []judge.RunResult{{Stdout: "3", Stage: judge.StageRun}}, nil
	}), "secret", 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := NewGRPCServer(core, grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		response, err := handler(ctx, req)
		if err == nil && info.FullMethod == pb.ExecutorService_RunBatch_FullMethodName {
			return nil, status.Error(codes.Unavailable, "response lost after execution")
		}
		return response, err
	}))
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	defer listener.Close()
	pool, err := NewGRPCClientPool([]string{listener.Addr().String()}, "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.RunBatch(context.Background(), judge.RunRequest{Language: domain.LanguagePython, Code: "print(3)", TimeLimitMS: 1000, MemoryLimitMB: 64}, []string{""})
	var failure *CallError
	if !errors.As(err, &failure) || failure.Kind != ErrorExecutionUnknown || calls.Load() != 1 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}

type runnerFunc func(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error)

func (f runnerFunc) RunBatch(ctx context.Context, r judge.RunRequest, in []string) ([]judge.RunResult, error) {
	return f(ctx, r, in)
}
