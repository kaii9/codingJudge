package platform

import (
	"context"
	"fmt"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/httpapi"
	"github.com/kaii9/codingJudge/internal/judgerpc"
)

type Store struct {
	httpapi.ProblemStore
	judge *judgerpc.Client
}

func NewStore(local httpapi.ProblemStore, remote *judgerpc.Client) *Store {
	return &Store{ProblemStore: local, judge: remote}
}
func (s *Store) CreateSubmission(ctx context.Context, sub domain.Submission) (domain.Submission, error) {
	out, _, err := s.CreateSubmissionIdempotent(ctx, sub, "", "")
	return out, err
}
func (s *Store) CreateSubmissionIdempotent(ctx context.Context, sub domain.Submission, key, hash string) (domain.Submission, bool, error) {
	problem, ok, err := s.ProblemStore.GetProblem(ctx, sub.ProblemID)
	if err != nil {
		return domain.Submission{}, false, err
	}
	if !ok {
		return domain.Submission{}, false, fmt.Errorf("problem not found")
	}
	return s.judge.Create(ctx, sub, key, hash, problem)
}
func (s *Store) FindSubmissionByIdempotencyKey(ctx context.Context, user, key string) (domain.Submission, string, bool, error) {
	return s.judge.FindSubmissionByIdempotencyKey(ctx, user, key)
}
func (s *Store) GetSubmissionForUser(ctx context.Context, id, user string) (domain.Submission, bool, error) {
	return s.judge.GetSubmissionForUser(ctx, id, user)
}
func (s *Store) ListSubmissionsByUser(ctx context.Context, user string) ([]domain.Submission, error) {
	return s.judge.ListSubmissionsByUser(ctx, user)
}
func (s *Store) ListSubmissionArtifactsForUser(ctx context.Context, id, user string) ([]domain.SubmissionArtifact, error) {
	return s.judge.ListSubmissionArtifactsForUser(ctx, id, user)
}
func (s *Store) ListSubmissions(context.Context) ([]domain.Submission, error) {
	return nil, fmt.Errorf("unscoped submission access forbidden")
}
func (s *Store) GetSubmission(context.Context, string) (domain.Submission, bool, error) {
	return domain.Submission{}, false, fmt.Errorf("unscoped submission access forbidden")
}
func (s *Store) ListSubmissionArtifacts(context.Context, string) ([]domain.SubmissionArtifact, error) {
	return nil, fmt.Errorf("unscoped artifact access forbidden")
}

func (s *Store) ListLeaderboard(ctx context.Context, limit int) ([]domain.LeaderboardEntry, error) {
	if reader, ok := s.ProblemStore.(interface {
		ListProjectedLeaderboard(context.Context, int) ([]domain.LeaderboardEntry, error)
	}); ok {
		return reader.ListProjectedLeaderboard(ctx, limit)
	}
	return nil, fmt.Errorf("leaderboard projection unavailable")
}
