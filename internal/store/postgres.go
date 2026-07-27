package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (s *PostgresStore) CreateUser(ctx context.Context, username, passwordHash string) (domain.User, error) {
	now := time.Now().UTC()
	user := domain.User{
		ID:        fmt.Sprintf("user-%d", now.UnixNano()),
		Username:  username,
		CreatedAt: now,
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, username, username_normalized, password_hash, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, user.ID, user.Username, normalizeUsername(username), passwordHash, user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.User{}, ErrConflict
		}
		return domain.User{}, err
	}
	return user, nil
}

func (s *PostgresStore) GetUserByUsername(ctx context.Context, username string) (domain.User, bool, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, created_at
		FROM users
		WHERE username_normalized = $1
	`, normalizeUsername(username)).Scan(&user.ID, &user.Username, &user.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.User{}, false, nil
		}
		return domain.User{}, false, err
	}
	return user, true, nil
}

func (s *PostgresStore) GetPasswordHashByUsername(ctx context.Context, username string) (string, domain.User, bool, error) {
	var user domain.User
	var passwordHash string
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, created_at
		FROM users
		WHERE username_normalized = $1
	`, normalizeUsername(username)).Scan(&user.ID, &user.Username, &passwordHash, &user.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", domain.User{}, false, nil
		}
		return "", domain.User{}, false, err
	}
	return passwordHash, user, true, nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (domain.Session, error) {
	now := time.Now().UTC()
	session := domain.Session{
		TokenHash: tokenHash,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_sessions (token_hash, user_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4)
	`, session.TokenHash, session.UserID, session.ExpiresAt, session.CreatedAt)
	if err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (s *PostgresStore) GetUserBySessionTokenHash(ctx context.Context, tokenHash string, now time.Time) (domain.User, bool, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.username, u.created_at
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > $2
	`, tokenHash, now).Scan(&user.ID, &user.Username, &user.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.User{}, false, nil
		}
		return domain.User{}, false, err
	}
	return user, true, nil
}

func (s *PostgresStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM user_sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *PostgresStore) ListProblems(ctx context.Context) ([]domain.Problem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.title, p.description, p.language, p.time_limit_ms, p.memory_limit_mb,
		       p.difficulty, p.collection, p.sort_order,
		       COALESCE((SELECT array_agg(tag ORDER BY tag) FROM problem_tags WHERE problem_id=p.id), '{}')
		FROM problems p
		ORDER BY CASE WHEN p.collection='hot20' THEN 0 ELSE 1 END, p.sort_order, p.id
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
			&problem.Difficulty, &problem.Collection, &problem.SortOrder, &problem.Tags,
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
		SELECT p.id, p.title, p.description, p.language, p.time_limit_ms, p.memory_limit_mb,
		       p.difficulty, p.collection, p.sort_order,
		       COALESCE((SELECT array_agg(tag ORDER BY tag) FROM problem_tags WHERE problem_id=p.id), '{}')
		FROM problems p
		WHERE p.id = $1
	`, id).Scan(
		&problem.ID,
		&problem.Title,
		&problem.Description,
		&problem.Language,
		&problem.TimeLimitMS,
		&problem.MemoryLimitMB,
		&problem.Difficulty, &problem.Collection, &problem.SortOrder, &problem.Tags,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.Problem{}, false, nil
		}
		return domain.Problem{}, false, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, COALESCE(input, ''), COALESCE(expected_output, ''),
		       COALESCE(input_object_key, ''), COALESCE(expected_output_object_key, ''),
		       COALESCE(input_sha256, ''), COALESCE(expected_output_sha256, ''),
		       COALESCE(input_size_bytes, 0), COALESCE(expected_output_size_bytes, 0),
		       hidden
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
		if err := rows.Scan(
			&tc.ID,
			&tc.Input,
			&tc.ExpectedOutput,
			&tc.InputObjectKey,
			&tc.ExpectedOutputObjectKey,
			&tc.InputSHA256,
			&tc.ExpectedOutputSHA256,
			&tc.InputSizeBytes,
			&tc.ExpectedOutputSizeBytes,
			&tc.Hidden,
		); err != nil {
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
		INSERT INTO submissions (id, user_id, problem_id, language, code, status, created_at, updated_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8)
	`, sub.ID, sub.UserID, sub.ProblemID, sub.Language, sub.Code, sub.Status, sub.CreatedAt, sub.UpdatedAt); err != nil {
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

func (s *PostgresStore) ReplaceProblemTestCases(ctx context.Context, problemID string, cases []domain.TestCase) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM problem_test_cases WHERE problem_id = $1`, problemID); err != nil {
		return err
	}
	for _, tc := range cases {
		if _, err := tx.Exec(ctx, `
			INSERT INTO problem_test_cases
			    (problem_id, input, expected_output, input_object_key, expected_output_object_key,
			     input_sha256, expected_output_sha256, input_size_bytes, expected_output_size_bytes, hidden)
			VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''),
			        NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, 0), NULLIF($9, 0), $10)
		`, problemID, tc.Input, tc.ExpectedOutput, tc.InputObjectKey, tc.ExpectedOutputObjectKey,
			tc.InputSHA256, tc.ExpectedOutputSHA256, tc.InputSizeBytes, tc.ExpectedOutputSizeBytes, tc.Hidden); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ListSubmissions(ctx context.Context) ([]domain.Submission, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, COALESCE(user_id, ''), problem_id, language, code, status, stdout, stderr, exit_code, duration_ms, created_at, updated_at
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

func (s *PostgresStore) ListSubmissionsByUser(ctx context.Context, userID string) ([]domain.Submission, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, COALESCE(user_id, ''), problem_id, language, code, status, stdout, stderr, exit_code, duration_ms, created_at, updated_at
		FROM submissions
		WHERE user_id = $1
		ORDER BY updated_at DESC, id DESC
	`, userID)
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
		SELECT id, COALESCE(user_id, ''), problem_id, language, code, status, stdout, stderr, exit_code, duration_ms, created_at, updated_at
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

func (s *PostgresStore) GetSubmissionForUser(ctx context.Context, id, userID string) (domain.Submission, bool, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, COALESCE(user_id, ''), problem_id, language, code, status, stdout, stderr, exit_code, duration_ms, created_at, updated_at
		FROM submissions
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	sub, err := scanSubmission(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.Submission{}, false, nil
		}
		return domain.Submission{}, false, err
	}
	return sub, true, nil
}

func (s *PostgresStore) ListLeaderboard(ctx context.Context, limit int) ([]domain.LeaderboardEntry, error) {
	if limit < 1 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.username, COUNT(DISTINCT sub.problem_id) AS solved,
		       COUNT(*) AS accepted_submissions, MAX(sub.updated_at) AS last_accepted_at
		FROM submissions sub
		JOIN users u ON u.id = sub.user_id
		WHERE sub.status = $1
		GROUP BY u.id, u.username
		ORDER BY solved DESC, last_accepted_at ASC, u.username ASC
		LIMIT $2
	`, domain.StatusAccepted, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []domain.LeaderboardEntry{}
	for rows.Next() {
		var entry domain.LeaderboardEntry
		if err := rows.Scan(&entry.UserID, &entry.Username, &entry.Solved, &entry.AcceptedSubmissions, &entry.LastAcceptedAt); err != nil {
			return nil, err
		}
		entry.Rank = len(entries) + 1
		entries = append(entries, entry)
	}
	return entries, rows.Err()
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
		&sub.UserID,
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
