package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaii9/codingJudge/internal/httpapi"
	"github.com/kaii9/codingJudge/internal/judgerpc"
	"github.com/kaii9/codingJudge/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func serveJudge(t *testing.T, server *grpc.Server) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	return listener.Addr().String()
}

func newJudgeClient(t *testing.T, addr, token string) *judgerpc.Client {
	t.Helper()
	client, err := judgerpc.NewClient(addr, token, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func checkHealthRoute(t *testing.T, handler http.Handler, path string, want int, component string) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != want {
		t.Fatalf("%s status = %d, want %d: %s", path, response.Code, want, response.Body.String())
	}
	if component != "" {
		var body map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["status"] != "not_ready" || body["component"] != component {
			t.Fatalf("unexpected readiness response: %v", body)
		}
	}
}

// Omitting the remote Judge readiness option leaves /readyz incorrectly 200
// for rejected credentials or a stopped service. /healthz must stay local.
func TestRemoteJudgeOptionsDriveReadinessNotLiveness(t *testing.T) {
	server := judgerpc.NewServer(nil, nil, "judge-token")
	addr := serveJudge(t, server)
	for _, fixture := range []struct {
		name, token string
		want        int
		component   string
	}{
		{"authenticated", "judge-token", http.StatusOK, ""},
		{"rejected-identity", "bad-token", http.StatusServiceUnavailable, "judge_rpc"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			client := newJudgeClient(t, addr, fixture.token)
			handler := httpapi.NewServer(store.NewMemoryStore(nil), remoteJudgeOptions(client)...)
			checkHealthRoute(t, handler, "/readyz", fixture.want, fixture.component)
			checkHealthRoute(t, handler, "/healthz", http.StatusOK, "")
		})
	}
	client := newJudgeClient(t, addr, "judge-token")
	handler := httpapi.NewServer(store.NewMemoryStore(nil), remoteJudgeOptions(client)...)
	checkHealthRoute(t, handler, "/readyz", http.StatusOK, "")
	server.Stop()
	checkHealthRoute(t, handler, "/readyz", http.StatusServiceUnavailable, "judge_rpc")
	checkHealthRoute(t, handler, "/healthz", http.StatusOK, "")
}

func TestLegacyAPIHasNoRemoteJudgeReadinessDependency(t *testing.T) {
	handler := httpapi.NewServer(store.NewMemoryStore(nil), remoteJudgeOptions(nil)...)
	checkHealthRoute(t, handler, "/readyz", http.StatusOK, "")
	checkHealthRoute(t, handler, "/healthz", http.StatusOK, "")
}

// Giving Judge its own fresh timeout instead of the overall readiness context
// would let a slow dependency extend the probe beyond the shared 2s budget.
func TestRemoteJudgeReadinessSharesOverallBudget(t *testing.T) {
	remaining := make(chan time.Duration, 1)
	canceled := make(chan struct{}, 1)
	server := judgerpc.NewServer(nil, nil, "judge-token", grpc.ChainUnaryInterceptor(
		func(ctx context.Context, _ any, _ *grpc.UnaryServerInfo, _ grpc.UnaryHandler) (any, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				remaining <- 0
			} else {
				remaining <- time.Until(deadline)
			}
			<-ctx.Done()
			canceled <- struct{}{}
			return nil, status.FromContextError(ctx.Err()).Err()
		}))
	client := newJudgeClient(t, serveJudge(t, server), "judge-token")
	options := []httpapi.Option{httpapi.WithReadinessCheck("store", func(ctx context.Context) error {
		select {
		case <-time.After(600 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})}
	options = append(options, remoteJudgeOptions(client)...)
	handler := httpapi.NewServer(store.NewMemoryStore(nil), options...)
	started := time.Now()
	checkHealthRoute(t, handler, "/readyz", http.StatusServiceUnavailable, "judge_rpc")
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("readiness exceeded overall budget: %s", elapsed)
	}
	select {
	case budget := <-remaining:
		if budget <= 0 || budget > 1600*time.Millisecond {
			t.Fatalf("Judge received a fresh/missing deadline: %s", budget)
		}
	default:
		t.Fatal("Judge health probe was not invoked")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("readiness cancellation did not reach Judge")
	}
}
