package judgerpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/kaii9/codingJudge/api/gen/judge/v1"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/telemetry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func diagnosticsClient(t *testing.T, addr, token string, timeout time.Duration) (*Client, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	client, err := NewClient(addr, token, timeout)
	slog.SetDefault(previous)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, &output
}

func assertFailureDiagnostic(t *testing.T, output *bytes.Buffer, method, code, traceID string) {
	t.Helper()
	var entry map[string]any
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	if err := decoder.Decode(&entry); err != nil {
		t.Fatalf("RPC failure has no JSON diagnostic: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatal("one RPC produced more than one diagnostic")
	}
	if entry["grpc_method"] != method || entry["grpc_code"] != code || entry["level"] != "WARN" {
		t.Fatalf("wrong failure classification: %v", entry)
	}
	state, _ := entry["connection_state"].(string)
	if state != "READY" && state != "TRANSIENT_FAILURE" && state != "CONNECTING" && state != "IDLE" && state != "SHUTDOWN" {
		t.Fatalf("missing connection state: %v", entry)
	}
	if traceID != "" && entry["trace_id"] != traceID {
		t.Fatalf("failure not correlated with parent trace: %v", entry)
	}
	if duration, ok := entry["duration_ms"].(float64); !ok || duration < 0 {
		t.Fatalf("missing/invalid RPC duration: %v", entry)
	}
	for key := range entry {
		switch key {
		case "time", "level", "msg", "grpc_method", "grpc_code", "connection_state", "trace_id", "duration_ms":
		default:
			t.Fatalf("non-allowlisted diagnostic field: %s", key)
		}
	}
}

// Removing the common interceptor loses diagnostics for one or more actual
// business RPCs. Logging raw errors, payloads or metadata leaks these markers.
func TestClientFailureDiagnosticsAreCorrelatedAndRedacted(t *testing.T) {
	const sensitive = "password=private-password source=private-source token=private-token"
	const traceID = "62fe0fde361024663aee24e477eb7330"
	server := grpc.NewServer(grpc.UnaryInterceptor(func(context.Context, any, *grpc.UnaryServerInfo, grpc.UnaryHandler) (any, error) {
		return nil, status.Error(codes.Internal, sensitive)
	}))
	pb.RegisterJudgeServiceServer(server, &pb.UnimplementedJudgeServiceServer{})
	addr := serveClientFixture(t, server)
	ctx := telemetry.Extract(context.Background(), "00-"+traceID+"-765ec6fb011a2461-01")
	for _, fixture := range []struct {
		method string
		call   func(*Client) error
	}{
		{"/gojudge.judge.v1.JudgeService/CreateSubmission", func(client *Client) error {
			_, _, err := client.Create(ctx, domain.Submission{UserID: "private-user", Code: "private-source"}, "private-key", "private-hash", domain.Problem{})
			return err
		}},
		{"/gojudge.judge.v1.JudgeService/FindSubmission", func(client *Client) error {
			_, _, _, err := client.FindSubmissionByIdempotencyKey(ctx, "private-user", "private-key")
			return err
		}},
		{"/gojudge.judge.v1.JudgeService/GetSubmission", func(client *Client) error {
			_, _, err := client.GetSubmissionForUser(ctx, "private-submission", "private-user")
			return err
		}},
		{"/gojudge.judge.v1.JudgeService/ListSubmissions", func(client *Client) error {
			_, err := client.ListSubmissionsByUser(ctx, "private-user")
			return err
		}},
		{"/gojudge.judge.v1.JudgeService/ListArtifacts", func(client *Client) error {
			_, err := client.ListSubmissionArtifactsForUser(ctx, "private-submission", "private-user")
			return err
		}},
		{"/gojudge.judge.v1.JudgeService/GetArtifact", func(client *Client) error {
			data, err := json.Marshal(&pb.ArtifactQuery{UserId: "private-user", SubmissionId: "private-submission", ArtifactId: 17})
			if err != nil {
				return err
			}
			key := base64.RawURLEncoding.EncodeToString(data)
			_, err = client.Get(ctx, key)
			return err
		}},
	} {
		t.Run(fixture.method, func(t *testing.T) {
			client, output := diagnosticsClient(t, addr, "private-token", time.Second)
			err := fixture.call(client)
			if status.Code(err) != codes.Internal || status.Convert(err).Message() != sensitive {
				t.Fatal("diagnostics altered the caller-visible RPC error")
			}
			assertFailureDiagnostic(t, output, fixture.method, "Internal", traceID)
			for _, marker := range []string{"password=", "source=", "token=", "private-"} {
				if strings.Contains(output.String(), marker) {
					t.Fatalf("sensitive marker %q leaked to diagnostics", marker)
				}
			}
		})
	}
}

func TestClientHealthAuthenticationFailureIsDiagnosed(t *testing.T) {
	addr := serveClientFixture(t, NewServer(nil, nil, "private-correct-token"))
	client, output := diagnosticsClient(t, addr, "private-bad-token", time.Second)
	if err := client.Ping(context.Background()); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("Ping code = %v", status.Code(err))
	}
	assertFailureDiagnostic(t, output, "/grpc.health.v1.Health/Check", "Unauthenticated", "")
	if strings.Contains(output.String(), "private-") {
		t.Fatal("authentication token leaked to diagnostics")
	}
}

func TestClientDeadlineFailureIsDiagnosedWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, _ any, _ *grpc.UnaryServerInfo, _ grpc.UnaryHandler) (any, error) {
		calls.Add(1)
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}))
	pb.RegisterJudgeServiceServer(server, &pb.UnimplementedJudgeServiceServer{})
	client, output := diagnosticsClient(t, serveClientFixture(t, server), "private-token", 100*time.Millisecond)
	connectClientFixture(t, client)
	_, err := client.ListSubmissionsByUser(context.Background(), "private-user")
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("RPC code = %v", status.Code(err))
	}
	assertFailureDiagnostic(t, output, "/gojudge.judge.v1.JudgeService/ListSubmissions", "DeadlineExceeded", "")
	if count := calls.Load(); count != 1 {
		t.Fatalf("diagnostics retried RPC: calls=%d", count)
	}
}

func TestClientExpectedBusinessErrorsDoNotWarn(t *testing.T) {
	for _, code := range []codes.Code{codes.NotFound, codes.AlreadyExists, codes.InvalidArgument, codes.Canceled} {
		t.Run(code.String(), func(t *testing.T) {
			server := grpc.NewServer(grpc.UnaryInterceptor(func(context.Context, any, *grpc.UnaryServerInfo, grpc.UnaryHandler) (any, error) {
				return nil, status.Error(code, "expected business outcome")
			}))
			pb.RegisterJudgeServiceServer(server, &pb.UnimplementedJudgeServiceServer{})
			client, output := diagnosticsClient(t, serveClientFixture(t, server), "private-token", time.Second)
			_, err := client.ListSubmissionsByUser(context.Background(), "private-user")
			if status.Code(err) != code {
				t.Fatalf("RPC code = %v, want %v", status.Code(err), code)
			}
			if output.Len() != 0 {
				t.Fatal("expected business error generated warning noise")
			}
		})
	}
}
