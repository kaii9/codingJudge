package executor

import (
	"context"
	"errors"
	"time"

	pb "github.com/kaii9/codingJudge/api/gen/executor/v1"
	"github.com/kaii9/codingJudge/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type grpcExecutor struct {
	pb.UnimplementedExecutorServiceServer
	core *Server
}

func NewGRPCServer(core *Server, options ...grpc.ServerOption) *grpc.Server {
	options = append(options, grpc.MaxRecvMsgSize(maxRequestBytes), grpc.MaxSendMsgSize(maxResponseBytes))
	server := grpc.NewServer(options...)
	pb.RegisterExecutorServiceServer(server, &grpcExecutor{core: core})
	core.health = health.NewServer()
	healthpb.RegisterHealthServer(server, core.health)
	core.SetServing(core.serving.Load())
	return server
}
func (s *Server) SetServing(serving bool) {
	s.serving.Store(serving)
	if s.health != nil {
		state := healthpb.HealthCheckResponse_NOT_SERVING
		if serving {
			state = healthpb.HealthCheckResponse_SERVING
		}
		s.health.SetServingStatus("", state)
		s.health.SetServingStatus(pb.ExecutorService_ServiceDesc.ServiceName, state)
	}
}
func (s *Server) Drain(ctx context.Context) error { s.SetServing(false); return s.admission.drain(ctx) }
func (s *grpcExecutor) RunBatch(ctx context.Context, req *pb.RunBatchRequest) (*pb.RunBatchResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	auth := md.Get("authorization")
	if len(auth) != 1 || !validBearer(auth[0], s.core.token) {
		return nil, status.Error(codes.Unauthenticated, "unauthorized")
	}
	if !s.core.serving.Load() {
		return nil, status.Error(codes.Unavailable, "executor not serving")
	}
	request := batchRequest{Language: domain.Language(req.Language), Code: req.Code, Inputs: req.Inputs, TimeLimitMS: int(req.TimeLimitMs), MemoryLimitMB: int(req.MemoryLimitMb)}
	if err := request.validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	started := time.Now()
	workCtx, release, err := s.core.admission.acquire(ctx)
	if err != nil {
		return nil, rpcError(err)
	}
	defer release()
	resultMetric := "error"
	if s.core.executionMetrics != nil {
		s.core.executionMetrics.executionStarted(time.Since(started))
		defer func() { s.core.executionMetrics.executionFinished(resultMetric) }()
	}
	runs, err := s.core.runner.RunBatch(workCtx, request.judgeRequest(), request.Inputs)
	if err != nil {
		return nil, rpcError(err)
	}
	resultMetric = "success"
	out := &pb.RunBatchResponse{}
	for _, run := range runs {
		out.Results = append(out.Results, &pb.RunResult{Stdout: run.Stdout, Stderr: run.Stderr, ExitCode: int32(run.ExitCode), DurationMs: run.Duration, TimedOut: run.TimedOut, Stage: string(run.Stage)})
	}
	return out, nil
}
func rpcError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "execution canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "execution deadline exceeded")
	case errors.Is(err, errOverloaded):
		return status.Error(codes.ResourceExhausted, "executor capacity exhausted")
	case errors.Is(err, errDraining):
		return status.Error(codes.Unavailable, "executor draining")
	default:
		return status.Error(codes.Internal, "execution outcome unknown")
	}
}
