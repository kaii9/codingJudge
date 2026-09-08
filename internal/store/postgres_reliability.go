package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kaii9/codingJudge/internal/domain"
)

func (s *PostgresStore) ClaimOutbox(ctx context.Context, owner string, now time.Time, claimDuration time.Duration, limit int) ([]domain.OutboxEvent, error) {
	rows, err := s.pool.Query(ctx, `
		WITH candidates AS (
			SELECT id
			FROM judge_outbox
			WHERE published_at IS NULL
			  AND next_attempt_at <= $1
			  AND (claimed_by IS NULL OR claim_expires_at <= $1)
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		UPDATE judge_outbox AS outbox
		SET claimed_by = $3,
		    claim_expires_at = $4,
		    publish_attempts = publish_attempts + 1
		FROM candidates
		WHERE outbox.id = candidates.id
		RETURNING outbox.id, outbox.submission_id, outbox.claimed_by, outbox.publish_attempts
	`, now, limit, owner, now.Add(claimDuration))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]domain.OutboxEvent, 0, limit)
	for rows.Next() {
		var event domain.OutboxEvent
		if err := rows.Scan(&event.ID, &event.SubmissionID, &event.ClaimToken, &event.PublishAttempts); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *PostgresStore) MarkOutboxPublished(ctx context.Context, id int64, owner string, now time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE judge_outbox
		SET published_at = $3, claimed_by = NULL, claim_expires_at = NULL, last_error = NULL
		WHERE id = $1 AND claimed_by = $2 AND published_at IS NULL
	`, id, owner, now)
	return tag.RowsAffected() == 1, err
}

func (s *PostgresStore) MarkOutboxFailed(ctx context.Context, id int64, owner string, nextAttempt time.Time, cause string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE judge_outbox
		SET claimed_by = NULL, claim_expires_at = NULL, next_attempt_at = $3, last_error = $4
		WHERE id = $1 AND claimed_by = $2 AND published_at IS NULL
	`, id, owner, nextAttempt, cause)
	return tag.RowsAffected() == 1, err
}

func (s *PostgresStore) ClaimSubmission(ctx context.Context, id, workerID, token, receipt string, now time.Time, leaseDuration time.Duration) (domain.SubmissionClaim, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.SubmissionClaim{}, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT id, COALESCE(user_id, ''), problem_id, language, code, status, stdout, stderr, exit_code,
		       duration_ms, created_at, updated_at, judge_receipt, lease_expires_at, judge_attempts
		FROM submissions
		WHERE id = $1
		FOR UPDATE
	`, id)
	sub, activeReceipt, expiresAt, attempts, err := scanClaimSubmission(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SubmissionClaim{State: domain.ClaimMissing}, tx.Commit(ctx)
	}
	if err != nil {
		return domain.SubmissionClaim{}, err
	}
	if domain.IsTerminalSubmissionStatus(sub.Status) {
		return domain.SubmissionClaim{State: domain.ClaimTerminal, Submission: sub}, tx.Commit(ctx)
	}
	if sub.Status == domain.StatusRunning && expiresAt != nil && now.Before(*expiresAt) {
		state := domain.ClaimActiveOtherReceipt
		if activeReceipt == receipt {
			state = domain.ClaimActiveSameReceipt
		}
		return domain.SubmissionClaim{
			State:          state,
			Submission:     sub,
			ActiveReceipt:  activeReceipt,
			LeaseExpiresAt: *expiresAt,
			Attempts:       attempts,
		}, tx.Commit(ctx)
	}

	// 判断是否为租约接管：前置状态为 running 且租约已过期。
	takeover := sub.Status == domain.StatusRunning && expiresAt != nil && !now.Before(*expiresAt)

	expires := now.Add(leaseDuration)
	attempts++
	if _, err := tx.Exec(ctx, `
		UPDATE submissions
		SET status = $2, judge_token = $3, judge_worker_id = $4, judge_receipt = $5,
		    lease_expires_at = $6, judge_attempts = $7, last_error = NULL, updated_at = $8
		WHERE id = $1
	`, id, domain.StatusRunning, token, workerID, receipt, expires, attempts, now); err != nil {
		return domain.SubmissionClaim{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.SubmissionClaim{}, err
	}
	sub.Status = domain.StatusRunning
	sub.UpdatedAt = now
	return domain.SubmissionClaim{
		State:          domain.ClaimAcquired,
		Submission:     sub,
		Token:          token,
		WorkerID:       workerID,
		Receipt:        receipt,
		LeaseExpiresAt: expires,
		Attempts:       attempts,
		LeaseTakeover:  takeover,
	}, nil
}

func (s *PostgresStore) RenewSubmissionLease(ctx context.Context, id, token string, now time.Time, leaseDuration time.Duration) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE submissions
		SET lease_expires_at = $4, updated_at = $3
		WHERE id = $1 AND judge_token = $2 AND status = $5 AND lease_expires_at > $3
	`, id, token, now, now.Add(leaseDuration), domain.StatusRunning)
	return tag.RowsAffected() == 1, err
}

func (s *PostgresStore) CompleteSubmission(ctx context.Context, id, token string, now time.Time, result domain.JudgeResult) (bool, error) {
	result.Status = normalizeFinalStatus(result.Status)
	tag, err := s.pool.Exec(ctx, `
		UPDATE submissions
		SET status = $4, stdout = $5, stderr = $6, exit_code = $7, duration_ms = $8,
		    judge_token = NULL, judge_worker_id = NULL, judge_receipt = NULL,
		    lease_expires_at = NULL, last_error = NULL, updated_at = $3
		WHERE id = $1 AND judge_token = $2 AND status = $9 AND lease_expires_at > $3
	`, id, token, now, result.Status, result.Stdout, result.Stderr, result.ExitCode, result.Duration, domain.StatusRunning)
	return tag.RowsAffected() == 1, err
}

func (s *PostgresStore) SaveSubmissionArtifacts(ctx context.Context, id, token string, now time.Time, artifacts []domain.SubmissionArtifact) (bool, error) {
	if len(artifacts) == 0 {
		return true, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var active bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM submissions
			WHERE id = $1 AND judge_token = $2 AND status = $4 AND lease_expires_at > $3
		)
	`, id, token, now, domain.StatusRunning).Scan(&active); err != nil {
		return false, err
	}
	if !active {
		return false, tx.Commit(ctx)
	}
	for _, artifact := range artifacts {
		_, err := tx.Exec(ctx, `
			INSERT INTO submission_artifacts
			    (submission_id, attempt, fencing_token, kind, object_key, sha256, size_bytes, created_at)
			VALUES ($1, $3, $2, $4, $5, $6, $7, $8)
			ON CONFLICT (submission_id, attempt, fencing_token, kind) DO NOTHING
		`, id, token, artifact.Attempt, artifact.Kind, artifact.ObjectKey, artifact.SHA256, artifact.SizeBytes, now)
		if err != nil {
			return false, err
		}
	}
	return true, tx.Commit(ctx)
}

func (s *PostgresStore) ListSubmissionArtifacts(ctx context.Context, id string) ([]domain.SubmissionArtifact, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, submission_id, attempt, fencing_token, kind, object_key, sha256, size_bytes, created_at
		FROM submission_artifacts
		WHERE submission_id = $1
		ORDER BY id
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	artifacts := []domain.SubmissionArtifact{}
	for rows.Next() {
		var artifact domain.SubmissionArtifact
		if err := rows.Scan(
			&artifact.ID,
			&artifact.SubmissionID,
			&artifact.Attempt,
			&artifact.Token,
			&artifact.Kind,
			&artifact.ObjectKey,
			&artifact.SHA256,
			&artifact.SizeBytes,
			&artifact.CreatedAt,
		); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

func (s *PostgresStore) ReleaseSubmission(ctx context.Context, id, token string, now time.Time, cause string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE submissions
		SET status = $4, judge_token = NULL, judge_worker_id = NULL, judge_receipt = NULL,
		    lease_expires_at = NULL, last_error = $5, updated_at = $3
		WHERE id = $1 AND judge_token = $2 AND status = $6
	`, id, token, now, domain.StatusQueued, cause, domain.StatusRunning)
	return tag.RowsAffected() == 1, err
}

func scanClaimSubmission(row submissionScanner) (domain.Submission, string, *time.Time, int, error) {
	var sub domain.Submission
	var stdout, stderr, receipt *string
	var exitCode *int
	var duration *int64
	var expiresAt *time.Time
	var attempts int
	if err := row.Scan(
		&sub.ID, &sub.UserID, &sub.ProblemID, &sub.Language, &sub.Code, &sub.Status,
		&stdout, &stderr, &exitCode, &duration, &sub.CreatedAt, &sub.UpdatedAt,
		&receipt, &expiresAt, &attempts,
	); err != nil {
		return domain.Submission{}, "", nil, 0, err
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
	activeReceipt := ""
	if receipt != nil {
		activeReceipt = *receipt
	}
	return sub, activeReceipt, expiresAt, attempts, nil
}
