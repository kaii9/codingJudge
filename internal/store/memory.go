package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kai/codingjudge/internal/domain"
)

type MemoryStore struct {
	mu           sync.RWMutex
	problems     map[string]domain.Problem
	submissions  map[string]domain.Submission
	users        map[string]memoryUser
	usersByName  map[string]string
	sessions     map[string]memorySession
	leases       map[string]memoryLease
	outbox       map[int64]memoryOutbox
	artifacts    map[string][]domain.SubmissionArtifact
	nextID       int
	nextUser     int
	nextOutbox   int64
	nextArtifact int64
}

var (
	ErrConflict = errors.New("conflict")
	ErrNotFound = errors.New("not found")
)

type memoryUser struct {
	user         domain.User
	normalized   string
	passwordHash string
}

type memorySession struct {
	tokenHash string
	userID    string
	expiresAt time.Time
	createdAt time.Time
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
		users:       make(map[string]memoryUser),
		usersByName: make(map[string]string),
		sessions:    make(map[string]memorySession),
		leases:      make(map[string]memoryLease),
		outbox:      make(map[int64]memoryOutbox),
		artifacts:   make(map[string][]domain.SubmissionArtifact),
	}
	for _, problem := range problems {
		st.problems[problem.ID] = cloneProblem(problem)
	}
	return st
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func (s *MemoryStore) CreateUser(ctx context.Context, username, passwordHash string) (domain.User, error) {
	if err := ctx.Err(); err != nil {
		return domain.User{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	normalized := normalizeUsername(username)
	if _, exists := s.usersByName[normalized]; exists {
		return domain.User{}, ErrConflict
	}
	s.nextUser++
	now := time.Now().UTC()
	user := domain.User{
		ID:        fmt.Sprintf("user-%d", s.nextUser),
		Username:  strings.TrimSpace(username),
		CreatedAt: now,
	}
	s.users[user.ID] = memoryUser{
		user:         user,
		normalized:   normalized,
		passwordHash: passwordHash,
	}
	s.usersByName[normalized] = user.ID
	return user, nil
}

func (s *MemoryStore) GetUserByUsername(ctx context.Context, username string) (domain.User, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.User{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.usersByName[normalizeUsername(username)]
	if !ok {
		return domain.User{}, false, nil
	}
	user := s.users[id].user
	return user, true, nil
}

func (s *MemoryStore) GetPasswordHashByUsername(ctx context.Context, username string) (string, domain.User, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", domain.User{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.usersByName[normalizeUsername(username)]
	if !ok {
		return "", domain.User{}, false, nil
	}
	row := s.users[id]
	return row.passwordHash, row.user, true, nil
}

func (s *MemoryStore) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (domain.Session, error) {
	if err := ctx.Err(); err != nil {
		return domain.Session{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[userID]; !ok {
		return domain.Session{}, ErrNotFound
	}
	now := time.Now().UTC()
	session := memorySession{
		tokenHash: tokenHash,
		userID:    userID,
		expiresAt: expiresAt,
		createdAt: now,
	}
	s.sessions[tokenHash] = session
	return domain.Session{
		TokenHash: tokenHash,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, nil
}

func (s *MemoryStore) GetUserBySessionTokenHash(ctx context.Context, tokenHash string, now time.Time) (domain.User, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.User{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[tokenHash]
	if !ok || !now.Before(session.expiresAt) {
		return domain.User{}, false, nil
	}
	user, ok := s.users[session.userID]
	if !ok {
		return domain.User{}, false, nil
	}
	return user.user, true, nil
}

func (s *MemoryStore) DeleteSession(ctx context.Context, tokenHash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, tokenHash)
	return nil
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
		if problems[i].Collection != problems[j].Collection {
			return problems[i].Collection == domain.CollectionHot20
		}
		if problems[i].SortOrder != problems[j].SortOrder {
			return problems[i].SortOrder < problems[j].SortOrder
		}
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

func (s *MemoryStore) GetSubmissionForUser(ctx context.Context, id, userID string) (domain.Submission, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.Submission{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sub, ok := s.submissions[id]
	if !ok || sub.UserID != userID {
		return domain.Submission{}, false, nil
	}
	return cloneSubmission(sub), true, nil
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
	sortSubmissionsNewestFirst(submissions)
	return submissions, nil
}

func (s *MemoryStore) ListSubmissionsByUser(ctx context.Context, userID string) ([]domain.Submission, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	submissions := make([]domain.Submission, 0, len(s.submissions))
	for _, sub := range s.submissions {
		if sub.UserID == userID {
			submissions = append(submissions, cloneSubmission(sub))
		}
	}
	sortSubmissionsNewestFirst(submissions)
	return submissions, nil
}

func (s *MemoryStore) ListLeaderboard(ctx context.Context, limit int) ([]domain.LeaderboardEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	type accumulator struct {
		userID              string
		username            string
		solved              map[string]struct{}
		acceptedSubmissions int
		lastAcceptedAt      time.Time
	}
	acc := make(map[string]*accumulator)
	for _, sub := range s.submissions {
		if sub.UserID == "" || sub.Status != domain.StatusAccepted {
			continue
		}
		user, ok := s.users[sub.UserID]
		if !ok {
			continue
		}
		row := acc[sub.UserID]
		if row == nil {
			row = &accumulator{
				userID:   sub.UserID,
				username: user.user.Username,
				solved:   make(map[string]struct{}),
			}
			acc[sub.UserID] = row
		}
		row.solved[sub.ProblemID] = struct{}{}
		row.acceptedSubmissions++
		if sub.UpdatedAt.After(row.lastAcceptedAt) {
			row.lastAcceptedAt = sub.UpdatedAt
		}
	}

	entries := make([]domain.LeaderboardEntry, 0, len(acc))
	for _, row := range acc {
		entries = append(entries, domain.LeaderboardEntry{
			UserID:              row.userID,
			Username:            row.username,
			Solved:              len(row.solved),
			AcceptedSubmissions: row.acceptedSubmissions,
			LastAcceptedAt:      row.lastAcceptedAt,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Solved != entries[j].Solved {
			return entries[i].Solved > entries[j].Solved
		}
		if !entries[i].LastAcceptedAt.Equal(entries[j].LastAcceptedAt) {
			return entries[i].LastAcceptedAt.Before(entries[j].LastAcceptedAt)
		}
		return entries[i].Username < entries[j].Username
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	for i := range entries {
		entries[i].Rank = i + 1
	}
	return entries, nil
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

func sortSubmissionsNewestFirst(submissions []domain.Submission) {
	sort.Slice(submissions, func(i, j int) bool {
		if submissions[i].UpdatedAt.Equal(submissions[j].UpdatedAt) {
			return submissions[i].ID > submissions[j].ID
		}
		return submissions[i].UpdatedAt.After(submissions[j].UpdatedAt)
	})
}
