package rpcsecurity_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/executor"
	"github.com/kaii9/codingJudge/internal/judge"
	"github.com/kaii9/codingJudge/internal/judgerpc"
	"github.com/kaii9/codingJudge/internal/rpcsecurity"
	"github.com/kaii9/codingJudge/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func serveFixture(t *testing.T, server *grpc.Server) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	return listener.Addr().String()
}

func clientTLS(t *testing.T, identity testIdentity, ca, name string) []grpc.DialOption {
	t.Helper()
	opts, err := rpcsecurity.ClientOptions(identity.env(ca, name))
	if err != nil || len(opts) == 0 {
		t.Fatalf("TLS client options: %v", err)
	}
	return opts
}

func serverTLS(t *testing.T, identity testIdentity, ca string) []grpc.ServerOption {
	t.Helper()
	opts, err := rpcsecurity.ServerOptions(identity.env(ca, ""))
	if err != nil || len(opts) == 0 {
		t.Fatalf("TLS server options: %v", err)
	}
	return opts
}

func tlsObserver(seen chan<- tls.ConnectionState) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Observe, never enforce TLS here: otherwise this test interceptor could
		// hide a plaintext fallback in the production transport configuration.
		var state tls.ConnectionState
		if p, ok := peer.FromContext(ctx); ok {
			if info, ok := p.AuthInfo.(credentials.TLSInfo); ok {
				state = info.State
			}
		}
		select {
		case seen <- state:
		default:
			return nil, status.Error(codes.Internal, "unexpected extra TLS request")
		}
		return handler(ctx, req)
	}
}

func assertVerifiedPeer(t *testing.T, seen <-chan tls.ConnectionState, name string) {
	t.Helper()
	select {
	case state := <-seen:
		if state.Version != tls.VersionTLS13 || len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 || state.PeerCertificates[0].Subject.CommonName != name {
			t.Fatalf("unverified TLS peer: version=%x verifiedChains=%d", state.Version, len(state.VerifiedChains))
		}
	default:
		t.Fatal("no actual TLS request observed")
	}
}

func healthCall(t *testing.T, addr string, opts []grpc.DialOption) error {
	t.Helper()
	opts = append(opts, grpc.WithDisableRetry())
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	response, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err == nil && response.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("unexpected health status: %v", response.Status)
	}
	return err
}

func TestMTLSRejectsInvalidPeersBeforeHandler(t *testing.T) {
	ca, foreign := newTestAuthority(t), newTestAuthority(t)
	serverID := ca.issue(t, "executor.test", x509.ExtKeyUsageServerAuth, false)
	clientID := ca.issue(t, "worker.test", x509.ExtKeyUsageClientAuth, false)
	foreignID := foreign.issue(t, "untrusted.test", x509.ExtKeyUsageClientAuth, false)
	expiredID := ca.issue(t, "expired-client.test", x509.ExtKeyUsageClientAuth, true)
	wrongUsage := ca.issue(t, "server-only.test", x509.ExtKeyUsageServerAuth, false)
	seen := make(chan tls.ConnectionState, 16)
	opts := append(serverTLS(t, serverID, ca.file), grpc.ChainUnaryInterceptor(tlsObserver(seen)))
	server := grpc.NewServer(opts...)
	checker := health.NewServer()
	healthpb.RegisterHealthServer(server, checker)
	addr := serveFixture(t, server)
	validOptions := clientTLS(t, clientID, ca.file, "executor.test")
	if err := healthCall(t, addr, validOptions); err != nil {
		t.Fatalf("positive control: %v", err)
	}
	assertVerifiedPeer(t, seen, "worker.test")
	for _, fixture := range []struct {
		name string
		opts []grpc.DialOption
	}{
		{"no-client-certificate", []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: ca.roots, ServerName: "executor.test", MinVersion: tls.VersionTLS13}))}},
		{"untrusted-client-CA", clientTLS(t, foreignID, ca.file, "executor.test")},
		{"expired-client", clientTLS(t, expiredID, ca.file, "executor.test")},
		{"wrong-client-EKU", clientTLS(t, wrongUsage, ca.file, "executor.test")},
		{"untrusted-server-CA", clientTLS(t, clientID, foreign.file, "executor.test")},
		{"server-name-mismatch", clientTLS(t, clientID, ca.file, "different.test")},
		{"TLS-1.2-only", []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: ca.roots, ServerName: "executor.test", Certificates: []tls.Certificate{clientID.certificate(t)}, MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12}))}},
		{"plaintext", []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if err := healthCall(t, addr, fixture.opts); err == nil {
				t.Fatal("invalid peer accepted")
			}
			if len(seen) != 0 {
				t.Fatal("invalid peer reached application handler")
			}
		})
	}
	// A no-certificate client is otherwise usable against one-way TLS. Its
	// rejection above is not caused by an unusable client or closed listener.
	oneWay := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{serverID.certificate(t)}, MinVersion: tls.VersionTLS13})))
	healthpb.RegisterHealthServer(oneWay, health.NewServer())
	if err := healthCall(t, serveFixture(t, oneWay), []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: ca.roots, ServerName: "executor.test", MinVersion: tls.VersionTLS13}))}); err != nil {
		t.Fatalf("one-way TLS control: %v", err)
	}
	if err := healthCall(t, addr, validOptions); err != nil {
		t.Fatalf("positive control after rejection cases: %v", err)
	}
	assertVerifiedPeer(t, seen, "worker.test")
	// The observer must not implement the rejection itself. A deliberately
	// plaintext Health-only fixture remains usable and is recorded as non-TLS.
	plain := grpc.NewServer(grpc.ChainUnaryInterceptor(tlsObserver(seen)))
	healthpb.RegisterHealthServer(plain, health.NewServer())
	if err := healthCall(t, serveFixture(t, plain), []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}); err != nil {
		t.Fatalf("observer masked plaintext control: %v", err)
	}
	select {
	case state := <-seen:
		if state.Version != 0 || len(state.VerifiedChains) != 0 {
			t.Fatal("plaintext control recorded as TLS")
		}
	default:
		t.Fatal("observer failed to record plaintext handler entry")
	}
}

func TestMTLSRejectsExpiredServerCertificate(t *testing.T) {
	ca := newTestAuthority(t)
	serverID := ca.issue(t, "expired-server.test", x509.ExtKeyUsageServerAuth, true)
	clientID := ca.issue(t, "client.test", x509.ExtKeyUsageClientAuth, false)
	seen := make(chan tls.ConnectionState, 1)
	opts := append(serverTLS(t, serverID, ca.file), grpc.ChainUnaryInterceptor(tlsObserver(seen)))
	server := grpc.NewServer(opts...)
	healthpb.RegisterHealthServer(server, health.NewServer())
	if err := healthCall(t, serveFixture(t, server), clientTLS(t, clientID, ca.file, "expired-server.test")); err == nil || len(seen) != 0 {
		t.Fatalf("expired server accepted/reached handler: %v", err)
	}
	validID := ca.issue(t, "valid-server.test", x509.ExtKeyUsageServerAuth, false)
	validOpts := append(serverTLS(t, validID, ca.file), grpc.ChainUnaryInterceptor(tlsObserver(seen)))
	validServer := grpc.NewServer(validOpts...)
	healthpb.RegisterHealthServer(validServer, health.NewServer())
	if err := healthCall(t, serveFixture(t, validServer), clientTLS(t, clientID, ca.file, "valid-server.test")); err != nil {
		t.Fatalf("non-expired server control: %v", err)
	}
	assertVerifiedPeer(t, seen, "client.test")
}

type countedRunner struct{ calls atomic.Int64 }

func (r *countedRunner) RunBatch(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error) {
	r.calls.Add(1)
	return []judge.RunResult{{Stage: judge.StageRun, Stdout: "3"}}, nil
}

func TestMTLSBusinessClientsStillRequireServiceToken(t *testing.T) {
	ca := newTestAuthority(t)
	serverID := ca.issue(t, "service.test", x509.ExtKeyUsageServerAuth, false)
	clientID := ca.issue(t, "caller.test", x509.ExtKeyUsageClientAuth, false)
	clientOpts := clientTLS(t, clientID, ca.file, "service.test")
	for _, kind := range []string{"executor", "judge"} {
		t.Run(kind, func(t *testing.T) {
			seen := make(chan tls.ConnectionState, 8)
			serverOpts := append(serverTLS(t, serverID, ca.file), grpc.ChainUnaryInterceptor(tlsObserver(seen)))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if kind == "executor" {
				runner := &countedRunner{}
				core, err := executor.NewServer(runner, "mtls-test-token", 1, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				addr := serveFixture(t, executor.NewGRPCServer(core, serverOpts...))
				good, err := executor.NewGRPCClientPool([]string{addr}, "mtls-test-token", 2*time.Second, clientOpts...)
				if err != nil {
					t.Fatal(err)
				}
				defer good.Close()
				request := judge.RunRequest{Language: domain.LanguagePython, Code: "print(3)", TimeLimitMS: 1000, MemoryLimitMB: 64}
				runs, err := good.RunBatch(ctx, request, []string{"1 2"})
				if err != nil || len(runs) != 1 || runs[0].Stdout != "3" {
					t.Fatalf("secure execution: %+v %v", runs, err)
				}
				assertVerifiedPeer(t, seen, "caller.test")
				bad, err := executor.NewGRPCClientPool([]string{addr}, "wrong-token", 2*time.Second, clientOpts...)
				if err != nil {
					t.Fatal(err)
				}
				defer bad.Close()
				_, err = bad.RunBatch(ctx, request, []string{"1 2"})
				var failure *executor.CallError
				if status.Code(err) != codes.Unauthenticated || !errors.As(err, &failure) || failure.Kind != executor.ErrorUnauthorized || failure.Retryable() || runner.calls.Load() != 1 {
					t.Fatalf("certificate bypassed token: calls=%d error=%v", runner.calls.Load(), err)
				}
				assertVerifiedPeer(t, seen, "caller.test")
			} else {
				backend := store.NewMemoryStore(nil)
				addr := serveFixture(t, judgerpc.NewServer(backend, nil, "mtls-test-token", serverOpts...))
				good, err := judgerpc.NewClient(addr, "mtls-test-token", 2*time.Second, clientOpts...)
				if err != nil {
					t.Fatal(err)
				}
				defer good.Close()
				sub := domain.Submission{UserID: "alice", ProblemID: "sum", Language: domain.LanguagePython, Code: "print(3)"}
				problem := domain.Problem{ID: "sum", TimeLimitMS: 1000, MemoryLimitMB: 64, TestCases: []domain.TestCase{{Input: "1 2", ExpectedOutput: "3"}}}
				created, replay, err := good.Create(ctx, sub, "good", judgerpc.Fingerprint(sub), problem)
				if err != nil || replay || created.ID == "" {
					t.Fatalf("secure submission: %+v %v", created, err)
				}
				assertVerifiedPeer(t, seen, "caller.test")
				bad, err := judgerpc.NewClient(addr, "wrong-token", 2*time.Second, clientOpts...)
				if err != nil {
					t.Fatal(err)
				}
				defer bad.Close()
				_, _, err = bad.Create(ctx, sub, "bad", judgerpc.Fingerprint(sub), problem)
				if status.Code(err) != codes.Unauthenticated {
					t.Fatalf("certificate bypassed Judge token: %v", err)
				}
				assertVerifiedPeer(t, seen, "caller.test")
				subs, err := backend.ListSubmissionsByUser(ctx, "alice")
				if err != nil || len(subs) != 1 {
					t.Fatalf("unauthorized submission written: %+v %v", subs, err)
				}
			}
		})
	}
}
