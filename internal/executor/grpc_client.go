package executor

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	pb "github.com/kaii9/codingJudge/api/gen/executor/v1"
	"github.com/kaii9/codingJudge/internal/judge"
	"github.com/kaii9/codingJudge/internal/telemetry"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	_ "google.golang.org/grpc/health"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
	"google.golang.org/grpc/status"
)

type GRPCClientPool struct {
	conn    *grpc.ClientConn
	client  pb.ExecutorServiceClient
	token   string
	timeout time.Duration
}

func NewGRPCClientPool(endpoints []string, token string, timeout time.Duration, options ...grpc.DialOption) (*GRPCClientPool, error) {
	if len(endpoints) == 0 || token == "" || timeout <= 0 {
		return nil, fmt.Errorf("executor endpoints, token and positive timeout required")
	}
	builder := manual.NewBuilderWithScheme("gojudge-executor")
	addresses := make([]resolver.Address, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if strings.Contains(endpoint, "://") {
			parsed, err := url.Parse(endpoint)
			if err != nil || parsed.Host == "" {
				return nil, fmt.Errorf("invalid executor endpoint")
			}
			endpoint = parsed.Host
		}
		addresses = append(addresses, resolver.Address{Addr: endpoint})
	}
	builder.InitialState(resolver.State{Addresses: addresses})
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithResolvers(builder), grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}],"healthCheckConfig":{"serviceName":""}}`), grpc.WithDisableRetry(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxResponseBytes), grpc.MaxCallSendMsgSize(maxRequestBytes))}
	opts = append(opts, grpc.WithChainUnaryInterceptor(telemetry.ClientUnary))
	opts = append(opts, options...)
	conn, err := grpc.NewClient("gojudge-executor:///pool", opts...)
	if err != nil {
		return nil, err
	}
	return &GRPCClientPool{conn: conn, client: pb.NewExecutorServiceClient(conn), token: token, timeout: timeout}, nil
}
func (c *GRPCClientPool) Close() error { return c.conn.Close() }
func (c *GRPCClientPool) Run(ctx context.Context, r judge.RunRequest) (judge.RunResult, error) {
	runs, err := c.RunBatch(ctx, r, []string{r.Input})
	if err != nil {
		return judge.RunResult{}, err
	}
	return runs[0], nil
}
func (c *GRPCClientPool) RunBatch(ctx context.Context, r judge.RunRequest, inputs []string) ([]judge.RunResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)
	response, err := c.client.RunBatch(ctx, &pb.RunBatchRequest{Language: string(r.Language), Code: r.Code, Inputs: inputs, TimeLimitMs: int32(r.TimeLimitMS), MemoryLimitMb: int32(r.MemoryLimitMB), ExecutionId: telemetry.ExecutionID(ctx)}, grpc.WaitForReady(true))
	if err != nil {
		return nil, classifyRPCError(err)
	}
	if len(response.Results) == 0 || len(response.Results) > len(inputs) {
		return nil, &CallError{Kind: ErrorProtocol, Cause: fmt.Errorf("invalid result count")}
	}
	runs := make([]judge.RunResult, len(response.Results))
	for i, res := range response.Results {
		if res.Stage != string(judge.StageRun) && res.Stage != string(judge.StageCompile) {
			return nil, &CallError{Kind: ErrorProtocol, Cause: fmt.Errorf("invalid result stage")}
		}
		runs[i] = judge.RunResult{Stdout: res.Stdout, Stderr: res.Stderr, ExitCode: int(res.ExitCode), Duration: res.DurationMs, TimedOut: res.TimedOut, Stage: judge.RunStage(res.Stage)}
	}
	return runs, nil
}
func classifyRPCError(err error) error {
	kind := ErrorExecutionUnknown
	switch status.Code(err) {
	case codes.InvalidArgument:
		kind = ErrorInvalid
	case codes.Unauthenticated, codes.PermissionDenied:
		kind = ErrorUnauthorized
	case codes.ResourceExhausted:
		kind = ErrorOverloaded
	case codes.DeadlineExceeded:
		kind = ErrorDeadline
	case codes.Canceled:
		kind = ErrorCanceled
	case codes.Unimplemented, codes.DataLoss:
		kind = ErrorProtocol
	}
	return &CallError{Kind: kind, Cause: err}
}

var _ judge.Runner = (*GRPCClientPool)(nil)
