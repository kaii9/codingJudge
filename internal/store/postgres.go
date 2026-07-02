package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kai/codingjudge/internal/domain"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) ListProblems(ctx context.Context) ([]domain.Problem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, description, language, time_limit_ms, memory_limit_mb
		FROM problems
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	problems := []domain.Problem{}
	for rows.Next() {
		var problem domain.Problem
		if err := rows.Scan(
			&problem.ID,
			&problem.Title,
			&problem.Description,
			&problem.Language,
			&problem.TimeLimitMS,
			&problem.MemoryLimitMB,
		); err != nil {
			return nil, err
		}
		problems = append(problems, problem)
	}
	return problems, rows.Err()
}

func (s *PostgresStore) GetProblem(ctx context.Context, id string) (domain.Problem, bool, error) {
	var problem domain.Problem
	err := s.pool.QueryRow(ctx, `
		SELECT id, title, description, language, time_limit_ms, memory_limit_mb
		FROM problems
		WHERE id = $1
	`, id).Scan(
		&problem.ID,
		&problem.Title,
		&problem.Description,
		&problem.Language,
		&problem.TimeLimitMS,
		&problem.MemoryLimitMB,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.Problem{}, false, nil
		}
		return domain.Problem{}, false, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT input, expected_output
		FROM problem_test_cases
		WHERE problem_id = $1
		ORDER BY id
	`, id)
	if err != nil {
		return domain.Problem{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var tc domain.TestCase
		if err := rows.Scan(&tc.Input, &tc.ExpectedOutput); err != nil {
			return domain.Problem{}, false, err
		}
		problem.TestCases = append(problem.TestCases, tc)
	}
	if err := rows.Err(); err != nil {
		return domain.Problem{}, false, err
	}
	return problem, true, nil
}

func (s *PostgresStore) CreateSubmission(ctx context.Context, sub domain.Submission) (domain.Submission, error) {
	now := time.Now().UTC()
	sub.ID = fmt.Sprintf("sub-%d", now.UnixNano())
	sub.Status = domain.StatusQueued
	sub.CreatedAt = now
	sub.UpdatedAt = now
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Submission{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `
		INSERT INTO submissions (id, problem_id, language, code, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, sub.ID, sub.ProblemID, sub.Language, sub.Code, sub.Status, sub.CreatedAt, sub.UpdatedAt); err != nil {
		return domain.Submission{}, err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO judge_outbox (submission_id)
		VALUES ($1)
	`, sub.ID); err != nil {
		return domain.Submission{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Submission{}, err
	}
	return sub, nil
}

func (s *PostgresStore) ListSubmissions(ctx context.Context) ([]domain.Submission, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, problem_id, language, code, status, stdout, stderr, exit_code, duration_ms, created_at, updated_at
		FROM submissions
		ORDER BY updated_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	submissions := []domain.Submission{}
	for rows.Next() {
		sub, err := scanSubmission(rows)
		if err != nil {
			return nil, err
		}
		submissions = append(submissions, sub)
	}
	return submissions, rows.Err()
}

func (s *PostgresStore) GetSubmission(ctx context.Context, id string) (domain.Submission, bool, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, problem_id, language, code, status, stdout, stderr, exit_code, duration_ms, created_at, updated_at
		FROM submissions
		WHERE id = $1
	`, id)
	sub, err := scanSubmission(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.Submission{}, false, nil
		}
		return domain.Submission{}, false, err
	}
	return sub, true, nil
}

type submissionScanner interface {
	Scan(dest ...any) error
}

func scanSubmission(row submissionScanner) (domain.Submission, error) {
	var sub domain.Submission
	var stdout *string
	var stderr *string
	var exitCode *int
	var duration *int64
	if err := row.Scan(
		&sub.ID,
		&sub.ProblemID,
		&sub.Language,
		&sub.Code,
		&sub.Status,
		&stdout,
		&stderr,
		&exitCode,
		&duration,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	); err != nil {
		return domain.Submission{}, err
	}
	if stdout != nil || stderr != nil || exitCode != nil || duration != nil {
		result := domain.JudgeResult{Status: sub.Status}
		if stdout != nil {
			result.Stdout = *stdout
		}
		if stderr != nil {
			result.Stderr = *stderr
		}
		if exitCode != nil {
			result.ExitCode = *exitCode
		}
		if duration != nil {
			result.Duration = *duration
		}
		sub.Result = &result
	}
	return sub, nil
}
