package judgerpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	pb "github.com/kaii9/codingJudge/api/gen/judge/v1"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/store"
	"github.com/kaii9/codingJudge/internal/telemetry"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Client struct {
	conn    *grpc.ClientConn
	rpc     pb.JudgeServiceClient
	token   string
	timeout time.Duration
	logger  *slog.Logger
}

func NewClient(endpoint, token string, timeout time.Duration, options ...grpc.DialOption) (*Client, error) {
	if endpoint == "" || token == "" || timeout <= 0 {
		return nil, fmt.Errorf("judge endpoint, token and timeout required")
	}
	client := &Client{token: token, timeout: timeout, logger: slog.Default()}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(32<<20), grpc.MaxCallSendMsgSize(16<<20))}
	// Run diagnostics inside telemetry so failures retain the RPC trace context.
	opts = append(opts, grpc.WithChainUnaryInterceptor(telemetry.ClientUnary, client.diagnoseFailure))
	opts = append(opts, options...)
	conn, err := grpc.NewClient(endpoint, opts...)
	if err != nil {
		return nil, err
	}
	client.conn, client.rpc = conn, pb.NewJudgeServiceClient(conn)
	return client, nil
}

func (c *Client) diagnoseFailure(ctx context.Context, method string, request, reply any, conn *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
	started := time.Now()
	err := invoke(ctx, method, request, reply, conn, options...)
	code := status.Code(err)
	switch code {
	case codes.OK, codes.NotFound, codes.AlreadyExists, codes.InvalidArgument, codes.Canceled:
		return err
	}
	// Deliberately allowlist metadata: errors and payloads may contain code,
	// passwords, tokens or identifiers. Observe only; never retry or wrap errors.
	fields := []any{
		"grpc_method", method,
		"grpc_code", code.String(),
		"connection_state", conn.GetState().String(),
		"duration_ms", time.Since(started).Milliseconds(),
	}
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		fields = append(fields, "trace_id", span.TraceID().String())
	}
	c.logger.WarnContext(ctx, "judge RPC failed", fields...)
	return err
}
func (c *Client) Close() error { return c.conn.Close() }

// Ping verifies the same transport and service identity as business RPCs.
// The caller's remaining budget wins over the client's timeout cap.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := c.context(ctx)
	defer cancel()
	response, err := healthpb.NewHealthClient(c.conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if response.Status != healthpb.HealthCheckResponse_SERVING {
		return status.Error(codes.Unavailable, "judge health is not serving")
	}
	return nil
}
func (c *Client) context(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token), cancel
}
func (c *Client) Create(ctx context.Context, sub domain.Submission, key, hash string, problem domain.Problem) (domain.Submission, bool, error) {
	ctx, cancel := c.context(ctx)
	defer cancel()
	response, err := c.rpc.CreateSubmission(ctx, &pb.CreateRequest{UserId: sub.UserID, ProblemId: sub.ProblemID, Language: string(sub.Language), Code: sub.Code, IdempotencyKey: key, RequestHash: hash, Snapshot: encodeProblem(problem)})
	if status.Code(err) == codes.AlreadyExists {
		return domain.Submission{}, false, store.ErrIdempotencyConflict
	}
	if err != nil {
		return domain.Submission{}, false, err
	}
	return decodeSub(response.Submission), response.Replayed, nil
}
func (c *Client) FindSubmissionByIdempotencyKey(ctx context.Context, user, key string) (domain.Submission, string, bool, error) {
	ctx, cancel := c.context(ctx)
	defer cancel()
	r, err := c.rpc.FindSubmission(ctx, &pb.FindRequest{UserId: user, IdempotencyKey: key})
	if err != nil {
		return domain.Submission{}, "", false, err
	}
	return decodeSub(r.Submission), r.RequestHash, r.Found, nil
}
func (c *Client) GetSubmissionForUser(ctx context.Context, id, user string) (domain.Submission, bool, error) {
	ctx, cancel := c.context(ctx)
	defer cancel()
	r, err := c.rpc.GetSubmission(ctx, &pb.SubmissionQuery{UserId: user, SubmissionId: id})
	if status.Code(err) == codes.NotFound {
		return domain.Submission{}, false, nil
	}
	if err != nil {
		return domain.Submission{}, false, err
	}
	return decodeSub(r), true, nil
}
func (c *Client) ListSubmissionsByUser(ctx context.Context, user string) ([]domain.Submission, error) {
	ctx, cancel := c.context(ctx)
	defer cancel()
	r, err := c.rpc.ListSubmissions(ctx, &pb.UserQuery{UserId: user})
	if err != nil {
		return nil, err
	}
	out := []domain.Submission{}
	for _, sub := range r.Submissions {
		out = append(out, decodeSub(sub))
	}
	return out, nil
}
func (c *Client) ListSubmissionArtifactsForUser(ctx context.Context, id, user string) ([]domain.SubmissionArtifact, error) {
	ctx, cancel := c.context(ctx)
	defer cancel()
	r, err := c.rpc.ListArtifacts(ctx, &pb.SubmissionQuery{UserId: user, SubmissionId: id})
	if err != nil {
		return nil, err
	}
	out := []domain.SubmissionArtifact{}
	for _, a := range r.Artifacts {
		key, _ := json.Marshal(&pb.ArtifactQuery{UserId: user, SubmissionId: id, ArtifactId: a.Id})
		out = append(out, domain.SubmissionArtifact{ID: a.Id, SubmissionID: a.SubmissionId, Attempt: int(a.Attempt), Kind: domain.ArtifactKind(a.Kind), SHA256: a.Sha256, SizeBytes: a.SizeBytes, CreatedAt: time.Unix(0, a.CreatedAtUnixNano).UTC(), ObjectKey: base64.RawURLEncoding.EncodeToString(key)})
	}
	return out, nil
}
func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil {
		return nil, err
	}
	var req pb.ArtifactQuery
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	ctx, cancel := c.context(ctx)
	defer cancel()
	r, err := c.rpc.GetArtifact(ctx, &req)
	if err != nil {
		return nil, err
	}
	return r.Content, nil
}
