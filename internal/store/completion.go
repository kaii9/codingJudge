package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/kaii9/codingJudge/internal/domain"
	"time"
)

func (s *PostgresStore) completeWithEvent(ctx context.Context, id, token string, now time.Time, result domain.JudgeResult) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	result.Status = normalizeFinalStatus(result.Status)
	event := domain.CompletionEvent{SubmissionID: id, Status: result.Status, FinishedAt: now}
	err = tx.QueryRow(ctx, `
 UPDATE submissions SET status=$4,stdout=$5,stderr=$6,exit_code=$7,duration_ms=$8,
 judge_token=NULL,judge_worker_id=NULL,judge_receipt=NULL,lease_expires_at=NULL,last_error=NULL,updated_at=$3
 WHERE id=$1 AND judge_token=$2 AND status='running' AND lease_expires_at>$3
 RETURNING user_id,problem_id,judge_attempts`, id, token, now, result.Status, result.Stdout, result.Stderr, result.ExitCode, result.Duration).Scan(&event.UserID, &event.ProblemID, &event.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	event.ID = fmt.Sprintf("%s:%d", id, event.Version)
	data, err := json.Marshal(event)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO completion_outbox(event_id,submission_id,payload) VALUES($1,$2,$3)", event.ID, id, data); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

type CompletionOutbox struct{ Store *PostgresStore }

func (o CompletionOutbox) ClaimOutbox(ctx context.Context, owner string, now time.Time, duration time.Duration, limit int) ([]domain.OutboxEvent, error) {
	rows, err := o.Store.pool.Query(ctx, `
 WITH candidates AS(
 SELECT id FROM completion_outbox WHERE published_at IS NULL AND next_attempt_at<=$1
 AND (claimed_by IS NULL OR claim_expires_at<=$1) ORDER BY id FOR UPDATE SKIP LOCKED LIMIT $2)
 UPDATE completion_outbox o SET claimed_by=$3,claim_expires_at=$4,publish_attempts=publish_attempts+1
 FROM candidates c WHERE o.id=c.id RETURNING o.id,o.submission_id,o.claimed_by,o.publish_attempts,o.payload::text
 `, now, limit, owner, now.Add(duration))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.OutboxEvent{}
	for rows.Next() {
		var event domain.OutboxEvent
		if err := rows.Scan(&event.ID, &event.SubmissionID, &event.ClaimToken, &event.PublishAttempts, &event.Payload); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}
func (o CompletionOutbox) MarkOutboxPublished(ctx context.Context, id int64, owner string, now time.Time) (bool, error) {
	tag, err := o.Store.pool.Exec(ctx, "UPDATE completion_outbox SET published_at=$3,claimed_by=NULL,claim_expires_at=NULL,last_error=NULL WHERE id=$1 AND claimed_by=$2 AND published_at IS NULL", id, owner, now)
	return tag.RowsAffected() == 1, err
}
func (o CompletionOutbox) MarkOutboxFailed(ctx context.Context, id int64, owner string, next time.Time, cause string) (bool, error) {
	tag, err := o.Store.pool.Exec(ctx, "UPDATE completion_outbox SET next_attempt_at=$3,last_error=$4,claimed_by=NULL,claim_expires_at=NULL WHERE id=$1 AND claimed_by=$2 AND published_at IS NULL", id, owner, next, cause)
	return tag.RowsAffected() == 1, err
}
