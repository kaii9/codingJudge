package store

import (
	"context"
	"sort"
	"time"

	"github.com/kai/codingjudge/internal/domain"
)

type LeaseStore interface {
	GetProblem(context.Context, string) (domain.Problem, bool, error)
	ClaimSubmission(context.Context, string, string, string, string, time.Time, time.Duration) (domain.SubmissionClaim, error)
	RenewSubmissionLease(context.Context, string, string, time.Time, time.Duration) (bool, error)
	CompleteSubmission(context.Context, string, string, time.Time, domain.JudgeResult) (bool, error)
	ReleaseSubmission(context.Context, string, string, time.Time, string) (bool, error)
}

type OutboxStore interface {
	ClaimOutbox(context.Context, string, time.Time, time.Duration, int) ([]domain.OutboxEvent, error)
	MarkOutboxPublished(context.Context, int64, string, time.Time) (bool, error)
	MarkOutboxFailed(context.Context, int64, string, time.Time, string) (bool, error)
}

func (s *MemoryStore) ClaimSubmission(_ context.Context, id, workerID, token, receipt string, now time.Time, leaseDuration time.Duration) (domain.SubmissionClaim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub, ok := s.submissions[id]
	if !ok {
		return domain.SubmissionClaim{State: domain.ClaimMissing}, nil
	}
	if domain.IsTerminalSubmissionStatus(sub.Status) {
		return domain.SubmissionClaim{State: domain.ClaimTerminal, Submission: cloneSubmission(sub)}, nil
	}
	lease := s.leases[id]
	if sub.Status == domain.StatusRunning && now.Before(lease.expiresAt) {
		state := domain.ClaimActiveOtherReceipt
		if lease.receipt == receipt {
			state = domain.ClaimActiveSameReceipt
		}
		return domain.SubmissionClaim{
			State:          state,
			Submission:     cloneSubmission(sub),
			ActiveReceipt:  lease.receipt,
			LeaseExpiresAt: lease.expiresAt,
			Attempts:       lease.attempts,
		}, nil
	}

	// 判断是否为租约接管：前置状态为 running 且租约已过期。
	takeover := sub.Status == domain.StatusRunning

	lease.token = token
	lease.workerID = workerID
	lease.receipt = receipt
	lease.expiresAt = now.Add(leaseDuration)
	lease.attempts++
	lease.lastError = ""
	s.leases[id] = lease
	sub.Status = domain.StatusRunning
	sub.UpdatedAt = now
	s.submissions[id] = sub
	return domain.SubmissionClaim{
		State:          domain.ClaimAcquired,
		Submission:     cloneSubmission(sub),
		Token:          token,
		WorkerID:       workerID,
		Receipt:        receipt,
		LeaseExpiresAt: lease.expiresAt,
		Attempts:       lease.attempts,
		LeaseTakeover:  takeover,
	}, nil
}

func (s *MemoryStore) RenewSubmissionLease(_ context.Context, id, token string, now time.Time, leaseDuration time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[id]
	if !ok || lease.token != token || !now.Before(lease.expiresAt) || s.submissions[id].Status != domain.StatusRunning {
		return false, nil
	}
	lease.expiresAt = now.Add(leaseDuration)
	s.leases[id] = lease
	return true, nil
}

func (s *MemoryStore) CompleteSubmission(_ context.Context, id, token string, now time.Time, result domain.JudgeResult) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[id]
	sub, found := s.submissions[id]
	if !ok || !found || sub.Status != domain.StatusRunning || lease.token != token || !now.Before(lease.expiresAt) {
		return false, nil
	}
	result.Status = normalizeFinalStatus(result.Status)
	sub.Status = result.Status
	sub.Result = &result
	sub.UpdatedAt = now
	s.submissions[id] = sub
	delete(s.leases, id)
	return true, nil
}

func (s *MemoryStore) ReleaseSubmission(_ context.Context, id, token string, now time.Time, cause string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[id]
	sub, found := s.submissions[id]
	if !ok || !found || sub.Status != domain.StatusRunning || lease.token != token {
		return false, nil
	}
	lease.token = ""
	lease.workerID = ""
	lease.receipt = ""
	lease.expiresAt = time.Time{}
	lease.lastError = cause
	s.leases[id] = lease
	sub.Status = domain.StatusQueued
	sub.UpdatedAt = now
	s.submissions[id] = sub
	return true, nil
}

func (s *MemoryStore) ClaimOutbox(_ context.Context, owner string, now time.Time, claimDuration time.Duration, limit int) ([]domain.OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]int64, 0, len(s.outbox))
	for id := range s.outbox {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	events := make([]domain.OutboxEvent, 0, limit)
	for _, id := range ids {
		row := s.outbox[id]
		if row.publishedAt != nil || now.Before(row.nextAttemptAt) || (row.claimedBy != "" && now.Before(row.claimExpiresAt)) {
			continue
		}
		row.claimedBy = owner
		row.claimExpiresAt = now.Add(claimDuration)
		row.publishAttempts++
		s.outbox[id] = row
		events = append(events, domain.OutboxEvent{ID: row.id, SubmissionID: row.submissionID, ClaimToken: owner, PublishAttempts: row.publishAttempts})
		if len(events) == limit {
			break
		}
	}
	return events, nil
}

func (s *MemoryStore) MarkOutboxPublished(_ context.Context, id int64, owner string, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.outbox[id]
	if !ok || row.publishedAt != nil || row.claimedBy != owner {
		return false, nil
	}
	row.publishedAt = &now
	row.claimedBy = ""
	row.claimExpiresAt = time.Time{}
	s.outbox[id] = row
	return true, nil
}

func (s *MemoryStore) MarkOutboxFailed(_ context.Context, id int64, owner string, nextAttempt time.Time, cause string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.outbox[id]
	if !ok || row.publishedAt != nil || row.claimedBy != owner {
		return false, nil
	}
	row.claimedBy = ""
	row.claimExpiresAt = time.Time{}
	row.nextAttemptAt = nextAttempt
	row.lastError = cause
	s.outbox[id] = row
	return true, nil
}
