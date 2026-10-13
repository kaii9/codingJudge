package judgeworker_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/judgeworker"
	"github.com/kaii9/codingJudge/internal/store"
)

// Unlike the older fakes, these methods reject expired contexts like PostgreSQL
// and Redis do. A task deadline must not also consume its recovery budget.
type budgetStore struct {
	*fakeStore
	stage string
}

func (s *budgetStore) GetProblem(ctx context.Context, id string) (domain.Problem, bool, error) {
	if s.stage == "problem" {
		<-ctx.Done()
		return domain.Problem{}, false, ctx.Err()
	}
	return s.fakeStore.GetProblem(ctx, id)
}
func (s *budgetStore) CompleteSubmission(ctx context.Context, id, token string, now time.Time, result domain.JudgeResult) (bool, error) {
	if s.stage == "completion" && result.Status == domain.StatusAccepted {
		<-ctx.Done()
		return false, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return false, errors.New("unbounded recovery context")
	}
	return s.fakeStore.CompleteSubmission(ctx, id, token, now, result)
}
func (s *budgetStore) ReleaseSubmission(ctx context.Context, id, token string, now time.Time, cause string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return false, errors.New("unbounded recovery context")
	}
	return s.fakeStore.ReleaseSubmission(ctx, id, token, now, cause)
}

func TestTaskBudgetStillAllowsFencedRecovery(t *testing.T) {
	for _, stage := range []string{"problem", "execution", "completion"} {
		for _, attempt := range []int{1, 3} {
			t.Run(stage+"/attempt-"+strconv.Itoa(attempt), func(t *testing.T) {
				calls := []string{}
				claim := acquiredClaim()
				claim.Attempts = attempt
				st := &budgetStore{fakeStore: &fakeStore{claim: claim, problem: domain.Problem{ID: "sum"}, completeOK: true, releaseOK: true, calls: &calls}, stage: stage}
				q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
				j := &fakeJudge{result: domain.JudgeResult{Status: domain.StatusAccepted}, block: stage == "execution", calls: &calls}
				p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{TaskTimeout: 15 * time.Millisecond, HeartbeatInterval: time.Hour, MaxAttempts: 3})
				err := p.ProcessOne(context.Background())
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("error=%v, want task deadline", err)
				}
				want := []string{"release", "retry"}
				if attempt == 3 {
					want = []string{"complete", "dead"}
				}
				if len(calls) < 2 || !reflect.DeepEqual(calls[len(calls)-2:], want) {
					t.Fatalf("calls=%v, want recovery suffix=%v", calls, want)
				}
			})
		}
	}
}

func TestExhaustedReclaimedAttemptNeverExecutesAgain(t *testing.T) {
	calls := []string{}
	claim := acquiredClaim()
	claim.Attempts = 4 // Three earlier processes died before recording recovery.
	claim.LeaseTakeover = true
	st := &budgetStore{fakeStore: &fakeStore{claim: claim, completeOK: true, calls: &calls}}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	p := judgeworker.NewProcessor(st, q, &fakeJudge{calls: &calls}, judgeworker.Config{MaxAttempts: 3, HeartbeatInterval: time.Hour})
	if err := p.ProcessOne(context.Background()); err == nil {
		t.Fatal("expected exhausted attempt error")
	}
	if want := []string{"dequeue", "claim", "complete", "dead"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v, want=%v", calls, want)
	}
}

func TestParentCancellationDoesNotDetachRecovery(t *testing.T) {
	calls := []string{}
	ctx, cancel := context.WithCancel(context.Background())
	st := &budgetStore{fakeStore: &fakeStore{claim: acquiredClaim(), problem: domain.Problem{ID: "sum"}, releaseOK: true, calls: &calls}}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := judgeFunc(func(context.Context, domain.Problem, domain.Language, string) (domain.JudgeResult, error) {
		cancel()
		return domain.JudgeResult{}, context.Canceled
	})
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{HeartbeatInterval: time.Hour})
	if err := p.ProcessOne(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want parent cancellation", err)
	}
	if want := []string{"dequeue", "claim", "problem"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("shutdown must leave pending recovery to another worker: %v", calls)
	}
}

type judgeFunc func(context.Context, domain.Problem, domain.Language, string) (domain.JudgeResult, error)

func (f judgeFunc) Evaluate(ctx context.Context, p domain.Problem, lang domain.Language, code string) (domain.JudgeResult, error) {
	return f(ctx, p, lang, code)
}

type deadlineMemoryStore struct{ *store.MemoryStore }

func (s deadlineMemoryStore) CompleteSubmission(ctx context.Context, id, token string, now time.Time, result domain.JudgeResult) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return s.MemoryStore.CompleteSubmission(ctx, id, token, now, result)
}
func (s deadlineMemoryStore) ReleaseSubmission(ctx context.Context, id, token string, now time.Time, cause string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return s.MemoryStore.ReleaseSubmission(ctx, id, token, now, cause)
}

func TestRepeatedTaskDeadlinesConvergeWithinAttemptBudget(t *testing.T) {
	ctx := context.Background()
	st := deadlineMemoryStore{store.NewMemoryStore([]domain.Problem{{ID: "sum"}})}
	sub, err := st.CreateSubmission(ctx, domain.Submission{ProblemID: "sum", Language: domain.LanguageGo, Code: "code"})
	if err != nil {
		t.Fatal(err)
	}
	job := domain.Job{SubmissionID: sub.ID, Receipt: "1-0"}
	calls := 0
	j := judgeFunc(func(ctx context.Context, _ domain.Problem, _ domain.Language, _ string) (domain.JudgeResult, error) {
		calls++
		<-ctx.Done()
		return domain.JudgeResult{}, ctx.Err()
	})
	p := judgeworker.NewProcessor(st, quietQueue{job: job}, j, judgeworker.Config{TaskTimeout: 15 * time.Millisecond, HeartbeatInterval: time.Hour, MaxAttempts: 3})
	for attempt := 1; attempt <= 3; attempt++ {
		if err := p.ProcessJob(ctx, job); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		current, _, _ := st.GetSubmission(ctx, sub.ID)
		want := domain.StatusQueued
		if attempt == 3 {
			want = domain.StatusInternalError
		}
		if current.Status != want {
			t.Fatalf("attempt %d status=%s, want=%s", attempt, current.Status, want)
		}
	}
	if err := p.ProcessJob(ctx, job); err != nil {
		t.Fatalf("redelivered terminal message: %v", err)
	}
	if calls != 3 {
		t.Fatalf("executions=%d, want exactly three bounded attempts", calls)
	}
}
