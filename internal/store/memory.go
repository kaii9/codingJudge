package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/kai/codingjudge/internal/domain"
)

type MemoryStore struct {
	mu          sync.RWMutex
	problems    map[string]domain.Problem
	submissions map[string]domain.Submission
	leases      map[string]memoryLease
	outbox      map[int64]memoryOutbox
	nextID      int
	nextOutbox  int64
}

type memoryLease struct {
	token     string
	workerID  string
	receipt   string
	expiresAt time.Time
	attempts  int
	lastError string
}

type memoryOutbox struct {
	id              int64
	submissionID    string
	publishedAt     *time.Time
	claimedBy       string
	claimExpiresAt  time.Time
	publishAttempts int
	nextAttemptAt   time.Time
	lastError       string
}

func NewMemoryStore(problems []domain.Problem) *MemoryStore {
	st := &MemoryStore{
		problems:    make(map[string]domain.Problem, len(problems)),
		submissions: make(map[string]domain.Submission),
		leases:      make(map[string]memoryLease),
		outbox:      make(map[int64]memoryOutbox),
	}
	for _, problem := range problems {
		st.problems[problem.ID] = cloneProblem(problem)
	}
	return st
}

func (s *MemoryStore) ListProblems(ctx context.Context) ([]domain.Problem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	problems := make([]domain.Problem, 0, len(s.problems))
	for _, problem := range s.problems {
		problems = append(problems, cloneProblem(problem))
	}
	sort.Slice(problems, func(i, j int) bool {
		return problems[i].ID < problems[j].ID
	})
	return problems, nil
}

func (s *MemoryStore) GetProblem(ctx context.Context, id string) (domain.Problem, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.Problem{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	problem, ok := s.problems[id]
	return cloneProblem(problem), ok, nil
}

func (s *MemoryStore) CreateSubmission(ctx context.Context, sub domain.Submission) (domain.Submission, error) {
	if err := ctx.Err(); err != nil {
		return domain.Submission{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	now := time.Now().UTC()
	sub.ID = fmt.Sprintf("sub-%d", s.nextID)
	sub.Status = domain.StatusQueued
	sub.CreatedAt = now
	sub.UpdatedAt = now
	s.submissions[sub.ID] = sub
	s.nextOutbox++
	s.outbox[s.nextOutbox] = memoryOutbox{
		id:            s.nextOutbox,
		submissionID:  sub.ID,
		nextAttemptAt: now,
	}
	return sub, nil
}

func (s *MemoryStore) GetSubmission(ctx context.Context, id string) (domain.Submission, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.Submission{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sub, ok := s.submissions[id]
	return cloneSubmission(sub), ok, nil
}

func (s *MemoryStore) ListSubmissions(ctx context.Context) ([]domain.Submission, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	submissions := make([]domain.Submission, 0, len(s.submissions))
	for _, sub := range s.submissions {
		submissions = append(submissions, cloneSubmission(sub))
	}
	sort.Slice(submissions, func(i, j int) bool {
		if submissions[i].UpdatedAt.Equal(submissions[j].UpdatedAt) {
			return submissions[i].ID > submissions[j].ID
		}
		return submissions[i].UpdatedAt.After(submissions[j].UpdatedAt)
	})
	return submissions, nil
}

func (s *MemoryStore) UpdateSubmissionStatus(ctx context.Context, id string, status domain.SubmissionStatus) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sub, ok := s.submissions[id]
	if !ok {
		return fmt.Errorf("submission %q not found", id)
	}
	sub.Status = status
	sub.UpdatedAt = time.Now().UTC()
	s.submissions[id] = sub
	return nil
}

func (s *MemoryStore) UpdateSubmissionResult(ctx context.Context, id string, result domain.JudgeResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sub, ok := s.submissions[id]
	if !ok {
		return fmt.Errorf("submission %q not found", id)
	}
	result.Status = normalizeFinalStatus(result.Status)
	sub.Status = result.Status
	sub.Result = &result
	sub.UpdatedAt = time.Now().UTC()
	s.submissions[id] = sub
	return nil
}

func normalizeFinalStatus(status domain.SubmissionStatus) domain.SubmissionStatus {
	if status == "" {
		return domain.StatusInternalError
	}
	return status
}

func cloneProblem(problem domain.Problem) domain.Problem {
	if problem.TestCases != nil {
		problem.TestCases = append([]domain.TestCase(nil), problem.TestCases...)
	}
	return problem
}

func cloneSubmission(sub domain.Submission) domain.Submission {
	if sub.Result != nil {
		result := *sub.Result
		sub.Result = &result
	}
	return sub
}
