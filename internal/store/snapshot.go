package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/kaii9/codingJudge/internal/domain"
)

func NewJudgePostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	st, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		return nil, err
	}
	st.judgeDomain = true
	return st, nil
}
func (s *PostgresStore) CreateSubmissionWithSnapshot(ctx context.Context, sub domain.Submission, key, hash string, problem domain.Problem) (domain.Submission, bool, error) {
	return s.createSubmission(ctx, sub, key, hash, &problem)
}
func (s *PostgresStore) GetSubmissionProblem(ctx context.Context, id string) (domain.Problem, bool, error) {
	if !s.judgeDomain {
		sub, ok, err := s.GetSubmission(ctx, id)
		if err != nil || !ok {
			return domain.Problem{}, false, err
		}
		return s.GetProblem(ctx, sub.ProblemID)
	}
	var data []byte
	err := s.pool.QueryRow(ctx, "SELECT snapshot FROM submission_problem_snapshots WHERE submission_id=$1", id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Problem{}, false, nil
	}
	if err != nil {
		return domain.Problem{}, false, err
	}
	var problem domain.Problem
	if err := json.Unmarshal(data, &problem); err != nil {
		return domain.Problem{}, false, err
	}
	return problem, true, nil
}
func (s *MemoryStore) CreateSubmissionWithSnapshot(ctx context.Context, sub domain.Submission, key, hash string, problem domain.Problem) (domain.Submission, bool, error) {
	return s.createSubmission(ctx, sub, key, hash, &problem)
}
func (s *MemoryStore) GetSubmissionProblem(ctx context.Context, id string) (domain.Problem, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.Problem{}, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.snapshots[id]; ok {
		return cloneProblem(p), true, nil
	}
	sub, ok := s.submissions[id]
	if !ok {
		return domain.Problem{}, false, nil
	}
	p, ok := s.problems[sub.ProblemID]
	return cloneProblem(p), ok, nil
}
