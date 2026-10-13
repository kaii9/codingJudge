package store

import (
	"context"
	"fmt"
	"github.com/kaii9/codingJudge/internal/domain"
	"sort"
)

func validateCompletion(e domain.CompletionEvent) error {
	if e.ID == "" || e.SubmissionID == "" || e.UserID == "" || e.ProblemID == "" || e.Version < 1 || !domain.IsTerminalSubmissionStatus(e.Status) || e.FinishedAt.IsZero() {
		return fmt.Errorf("invalid completion event")
	}
	return nil
}
func (s *PostgresStore) ApplyCompletionEvent(ctx context.Context, event domain.CompletionEvent) error {
	if err := validateCompletion(event); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	inserted, err := tx.Exec(ctx, "INSERT INTO processed_completion_events(event_id) VALUES($1) ON CONFLICT DO NOTHING", event.ID)
	if err != nil {
		return err
	}
	if inserted.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, `INSERT INTO submission_projection(submission_id,user_id,problem_id,status,version,finished_at)
 VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(submission_id) DO UPDATE SET status=EXCLUDED.status,
 version=EXCLUDED.version,finished_at=EXCLUDED.finished_at
 WHERE submission_projection.version<EXCLUDED.version`, event.SubmissionID, event.UserID, event.ProblemID, event.Status, event.Version, event.FinishedAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *PostgresStore) ListProjectedLeaderboard(ctx context.Context, limit int) ([]domain.LeaderboardEntry, error) {
	if limit < 1 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT u.id,u.username,COUNT(DISTINCT p.problem_id),COUNT(*),MAX(p.finished_at)
 FROM submission_projection p JOIN users u ON u.id=p.user_id WHERE p.status='accepted'
 GROUP BY u.id,u.username ORDER BY COUNT(DISTINCT p.problem_id) DESC,MAX(p.finished_at) ASC,u.username ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.LeaderboardEntry{}
	for rows.Next() {
		var row domain.LeaderboardEntry
		if err := rows.Scan(&row.UserID, &row.Username, &row.Solved, &row.AcceptedSubmissions, &row.LastAcceptedAt); err != nil {
			return nil, err
		}
		row.Rank = len(out) + 1
		out = append(out, row)
	}
	return out, rows.Err()
}
func (s *MemoryStore) ApplyCompletionEvent(ctx context.Context, event domain.CompletionEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateCompletion(event); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processedEvents[event.ID] {
		return nil
	}
	if old, exists := s.projections[event.SubmissionID]; !exists || old.Version < event.Version {
		s.projections[event.SubmissionID] = event
	}
	s.processedEvents[event.ID] = true
	return nil
}
func (s *MemoryStore) ListProjectedLeaderboard(ctx context.Context, limit int) ([]domain.LeaderboardEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := map[string]*domain.LeaderboardEntry{}
	solved := map[string]map[string]bool{}
	for _, event := range s.projections {
		if event.Status != domain.StatusAccepted {
			continue
		}
		user, ok := s.users[event.UserID]
		if !ok {
			continue
		}
		if stats[event.UserID] == nil {
			stats[event.UserID] = &domain.LeaderboardEntry{UserID: event.UserID, Username: user.user.Username}
			solved[event.UserID] = map[string]bool{}
		}
		entry := stats[event.UserID]
		entry.AcceptedSubmissions++
		solved[event.UserID][event.ProblemID] = true
		if event.FinishedAt.After(entry.LastAcceptedAt) {
			entry.LastAcceptedAt = event.FinishedAt
		}
	}
	out := []domain.LeaderboardEntry{}
	for user, entry := range stats {
		entry.Solved = len(solved[user])
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Solved != b.Solved {
			return a.Solved > b.Solved
		}
		if !a.LastAcceptedAt.Equal(b.LastAcceptedAt) {
			return a.LastAcceptedAt.Before(b.LastAcceptedAt)
		}
		return a.Username < b.Username
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	return out, nil
}
