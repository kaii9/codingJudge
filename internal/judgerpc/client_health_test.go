package judgerpc

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func serveClientFixture(t *testing.T, server *grpc.Server) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	return listener.Addr().String()
}

func clientFixture(t *testing.T, addr, token string, timeout time.Duration) *Client {
	t.Helper()
	client, err := NewClient(addr, token, timeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func connectClientFixture(t *testing.T, client *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client.conn.Connect()
	for {
		state := client.conn.GetState()
		if state == connectivity.Ready {
			return
		}
		if !client.conn.WaitForStateChange(ctx, state) {
			t.Fatal("fixture gRPC connection did not become ready")
		}
	}
}

// Removing token propagation or invoking a business RPC instead of Health
// breaks this test: the production server has no backend to call.
func TestClientPingAuthenticatesWithoutBusinessSideEffects(t *testing.T) {
	addr := serveClientFixture(t, NewServer(nil, nil, "health-token"))
	for _, fixture := range []struct {
		name, token string
		want        codes.Code
	}{
		{"valid-token", "health-token", codes.OK},
		{"invalid-token", "wrong-token", codes.Unauthenticated},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			client := clientFixture(t, addr, fixture.token, time.Second)
			if err := client.Ping(context.Background()); status.Code(err) != fixture.want {
				t.Fatalf("Ping code = %v, want %v", status.Code(err), fixture.want)
			}
		})
	}
}

// Treating any successful Health response as ready would accept NOT_SERVING.
func TestClientPingRejectsNonServingStatus(t *testing.T) {
	for _, value := range []healthpb.HealthCheckResponse_ServingStatus{
		healthpb.HealthCheckResponse_NOT_SERVING, healthpb.HealthCheckResponse_UNKNOWN,
		healthpb.HealthCheckResponse_SERVICE_UNKNOWN,
	} {
		t.Run(value.String(), func(t *testing.T) {
			server := grpc.NewServer()
			checker := health.NewServer()
			checker.SetServingStatus("", value)
			healthpb.RegisterHealthServer(server, checker)
			client := clientFixture(t, serveClientFixture(t, server), "health-token", time.Second)
			if err := client.Ping(context.Background()); err == nil {
				t.Fatal("non-serving Judge accepted as ready")
			}
		})
	}
}

type blockingClientHealth struct {
	healthpb.UnimplementedHealthServer
	calls    atomic.Int32
	canceled chan struct{}
}

func (h *blockingClientHealth) Check(ctx context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	h.calls.Add(1)
	<-ctx.Done()
	h.canceled <- struct{}{}
	return nil, status.FromContextError(ctx.Err()).Err()
}

// Dropping either caller deadline or client cap hangs the call; retries cause
// more than one real server invocation. Cancellation must reach the handler.
func TestClientPingRespectsBudgetsAndCancelsWithoutRetry(t *testing.T) {
	for _, fixture := range []struct {
		name           string
		parent, client time.Duration
	}{
		{"parent-budget", 100 * time.Millisecond, 5 * time.Second},
		{"client-cap", 5 * time.Second, 100 * time.Millisecond},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			checker := &blockingClientHealth{canceled: make(chan struct{}, 4)}
			server := grpc.NewServer()
			healthpb.RegisterHealthServer(server, checker)
			client := clientFixture(t, serveClientFixture(t, server), "health-token", fixture.client)
			connectClientFixture(t, client)
			ctx, cancel := context.WithTimeout(context.Background(), fixture.parent)
			defer cancel()
			started := time.Now()
			if err := client.Ping(ctx); status.Code(err) != codes.DeadlineExceeded {
				t.Fatalf("Ping code = %v, want DeadlineExceeded", status.Code(err))
			}
			if time.Since(started) > time.Second {
				t.Fatal("shorter deadline was not enforced")
			}
			select {
			case <-checker.canceled:
			case <-time.After(time.Second):
				t.Fatal("health handler did not observe cancellation")
			}
			if calls := checker.calls.Load(); calls != 1 {
				t.Fatalf("health server received %d calls, want 1", calls)
			}
		})
	}
}

func TestClientPingRejectsUnavailableJudge(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	client := clientFixture(t, addr, "health-token", 200*time.Millisecond)
	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("unreachable Judge accepted as ready")
	}
}
