package judgerpc

import (
	"context"
	"crypto/subtle"
	"errors"
	pb "github.com/kaii9/codingJudge/api/gen/judge/v1"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/store"
	"github.com/kaii9/codingJudge/internal/telemetry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"regexp"
	"strings"
)

type Backend interface {
	CreateSubmissionWithSnapshot(context.Context, domain.Submission, string, string, domain.Problem) (domain.Submission, bool, error)
	FindSubmissionByIdempotencyKey(context.Context, string, string) (domain.Submission, string, bool, error)
	GetSubmissionForUser(context.Context, string, string) (domain.Submission, bool, error)
	ListSubmissionsByUser(context.Context, string) ([]domain.Submission, error)
	ListSubmissionArtifacts(context.Context, string) ([]domain.SubmissionArtifact, error)
}
type ObjectGetter interface {
	Get(context.Context, string) ([]byte, error)
}
type service struct {
	pb.UnimplementedJudgeServiceServer
	store   Backend
	objects ObjectGetter
}

func NewServer(st Backend, objects ObjectGetter, token string, options ...grpc.ServerOption) *grpc.Server {
	auth := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get("authorization")
		if token == "" || len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+token)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "unauthorized")
		}
		return handler(ctx, req)
	}
	options = append(options, grpc.ChainUnaryInterceptor(auth, telemetry.ServerUnary), grpc.MaxRecvMsgSize(16<<20), grpc.MaxSendMsgSize(32<<20))
	server := grpc.NewServer(options...)
	pb.RegisterJudgeServiceServer(server, &service{store: st, objects: objects})
	checker := health.NewServer()
	checker.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, checker)
	return server
}

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func (s *service) CreateSubmission(ctx context.Context, r *pb.CreateRequest) (*pb.CreateResponse, error) {
	sub := domain.Submission{UserID: r.UserId, ProblemID: r.ProblemId, Language: domain.Language(r.Language), Code: r.Code}
	if sub.UserID == "" || sub.ProblemID == "" || !domain.IsSupportedLanguage(sub.Language) || strings.TrimSpace(sub.Code) == "" || len(sub.Code) > 64<<10 {
		return nil, status.Error(codes.InvalidArgument, "invalid submission")
	}
	if r.IdempotencyKey != "" && (!keyPattern.MatchString(r.IdempotencyKey) || r.RequestHash != Fingerprint(sub)) {
		return nil, status.Error(codes.InvalidArgument, "invalid idempotency fingerprint")
	}
	problem := decodeProblem(r.Snapshot)
	if problem.ID != sub.ProblemID || problem.TimeLimitMS < 1 || problem.TimeLimitMS > 60000 || problem.MemoryLimitMB < 1 || problem.MemoryLimitMB > 2048 || len(problem.TestCases) < 1 || len(problem.TestCases) > 100 {
		return nil, status.Error(codes.InvalidArgument, "invalid problem snapshot")
	}
	created, replayed, err := s.store.CreateSubmissionWithSnapshot(ctx, sub, r.IdempotencyKey, r.RequestHash, problem)
	if err != nil {
		return nil, storeError(err)
	}
	return &pb.CreateResponse{Submission: encodeSub(created), Replayed: replayed}, nil
}
func (s *service) FindSubmission(ctx context.Context, r *pb.FindRequest) (*pb.FindResponse, error) {
	if r.UserId == "" || r.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "user and key required")
	}
	sub, hash, found, err := s.store.FindSubmissionByIdempotencyKey(ctx, r.UserId, r.IdempotencyKey)
	if err != nil {
		return nil, storeError(err)
	}
	out := &pb.FindResponse{Found: found, RequestHash: hash}
	if found {
		out.Submission = encodeSub(sub)
	}
	return out, nil
}
func (s *service) owned(ctx context.Context, user, id string) (domain.Submission, error) {
	if user == "" || id == "" {
		return domain.Submission{}, status.Error(codes.InvalidArgument, "user and submission required")
	}
	sub, ok, err := s.store.GetSubmissionForUser(ctx, id, user)
	if err != nil {
		return domain.Submission{}, storeError(err)
	}
	if !ok {
		return domain.Submission{}, status.Error(codes.NotFound, "submission not found")
	}
	return sub, nil
}
func (s *service) GetSubmission(ctx context.Context, r *pb.SubmissionQuery) (*pb.SubmissionRecord, error) {
	sub, err := s.owned(ctx, r.UserId, r.SubmissionId)
	if err != nil {
		return nil, err
	}
	return encodeSub(sub), nil
}
func (s *service) ListSubmissions(ctx context.Context, r *pb.UserQuery) (*pb.SubmissionList, error) {
	if r.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user required")
	}
	subs, err := s.store.ListSubmissionsByUser(ctx, r.UserId)
	if err != nil {
		return nil, storeError(err)
	}
	out := &pb.SubmissionList{}
	for _, sub := range subs {
		out.Submissions = append(out.Submissions, encodeSub(sub))
	}
	return out, nil
}
func (s *service) ListArtifacts(ctx context.Context, r *pb.SubmissionQuery) (*pb.ArtifactList, error) {
	if _, err := s.owned(ctx, r.UserId, r.SubmissionId); err != nil {
		return nil, err
	}
	artifacts, err := s.store.ListSubmissionArtifacts(ctx, r.SubmissionId)
	if err != nil {
		return nil, storeError(err)
	}
	out := &pb.ArtifactList{}
	for _, a := range artifacts {
		out.Artifacts = append(out.Artifacts, &pb.Artifact{Id: a.ID, SubmissionId: a.SubmissionID, Attempt: int32(a.Attempt), Kind: string(a.Kind), Sha256: a.SHA256, SizeBytes: a.SizeBytes, CreatedAtUnixNano: a.CreatedAt.UnixNano()})
	}
	return out, nil
}
func (s *service) GetArtifact(ctx context.Context, r *pb.ArtifactQuery) (*pb.ArtifactData, error) {
	if _, err := s.owned(ctx, r.UserId, r.SubmissionId); err != nil {
		return nil, err
	}
	if s.objects == nil {
		return nil, status.Error(codes.Unavailable, "object store unavailable")
	}
	artifacts, err := s.store.ListSubmissionArtifacts(ctx, r.SubmissionId)
	if err != nil {
		return nil, storeError(err)
	}
	for _, a := range artifacts {
		if a.ID == r.ArtifactId {
			data, err := s.objects.Get(ctx, a.ObjectKey)
			if err != nil {
				return nil, storeError(err)
			}
			return &pb.ArtifactData{Content: data}, nil
		}
	}
	return nil, status.Error(codes.NotFound, "artifact not found")
}
func storeError(err error) error {
	if errors.Is(err, store.ErrIdempotencyConflict) {
		return status.Error(codes.AlreadyExists, "idempotency conflict")
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "request deadline")
	}
	return status.Error(codes.Unavailable, "judge storage unavailable")
}
