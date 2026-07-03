package judgeworker_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/judgeworker"
)

type fakeWorkerMetrics struct {
	mu              sync.Mutex
	starts          int
	finishCalls     [][3]string // language, result, positive duration
	retries         int
	deadLetters     int
	leaseTakeovers  int
}

func (m *fakeWorkerMetrics) WorkerJobStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.starts++
}
func (m *fakeWorkerMetrics) WorkerJobFinished(language, result string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finishCalls = append(m.finishCalls, [3]string{language, result, fmtDuration(duration)})
}
func (m *fakeWorkerMetrics) WorkerRetry() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retries++
}
func (m *fakeWorkerMetrics) WorkerDeadLetter() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deadLetters++
}
func (m *fakeWorkerMetrics) WorkerLeaseTakeover() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leaseTakeovers++
}

func fmtDuration(d time.Duration) string {
	if d > 0 { return "positive" }
	return "zero"
}

func TestProcessorRecordsAcceptedMetric(t *testing.T) {
	calls := []string{}
	st := &fakeStore{claim: acquiredClaim(), problem: domain.Problem{ID: "sum"}, completeOK: true, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{result: domain.JudgeResult{Status: domain.StatusAccepted}, calls: &calls}
	m := &fakeWorkerMetrics{}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{
		WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour,
		Token: func() (string, error) { return "token", nil }, Metrics: m,
	})
	if err := p.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.starts != 1 {
		t.Errorf("starts = %d, want 1", m.starts)
	}
	if len(m.finishCalls) != 1 || m.finishCalls[0][0] != "go" || m.finishCalls[0][1] != "accepted" || m.finishCalls[0][2] != "positive" {
		t.Errorf("finish calls = %v", m.finishCalls)
	}
}

func TestProcessorRecordsRetryMetric(t *testing.T) {
	calls := []string{}
	st := &fakeStore{claim: acquiredClaim(), problem: domain.Problem{ID: "sum"}, releaseOK: true, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{err: errors.New("docker unavailable"), calls: &calls}
	m := &fakeWorkerMetrics{}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{
		WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour,
		MaxAttempts: 3, Token: func() (string, error) { return "token", nil }, Metrics: m,
	})
	_ = p.ProcessOne(context.Background())
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.retries != 1 {
		t.Errorf("retries = %d, want 1", m.retries)
	}
	if m.starts != 1 {
		t.Errorf("starts = %d, want 1", m.starts)
	}
	if len(m.finishCalls) != 1 || m.finishCalls[0][1] != "retry" {
		t.Errorf("finish calls = %v, want result=retry", m.finishCalls)
	}
}

func TestProcessorRecordsDeadLetterMetric(t *testing.T) {
	calls := []string{}
	claim := acquiredClaim()
	claim.Attempts = 3
	st := &fakeStore{claim: claim, problem: domain.Problem{ID: "sum"}, completeOK: true, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{err: errors.New("docker unavailable"), calls: &calls}
	m := &fakeWorkerMetrics{}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{
		WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour,
		MaxAttempts: 3, Token: func() (string, error) { return "token", nil }, Metrics: m,
	})
	_ = p.ProcessOne(context.Background())
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deadLetters != 1 {
		t.Errorf("deadLetters = %d, want 1", m.deadLetters)
	}
	if m.retries != 0 {
		t.Errorf("retries should be 0 when dead-lettering, got %d", m.retries)
	}
	if len(m.finishCalls) != 1 || m.finishCalls[0][1] != "dead_letter" {
		t.Errorf("finish calls = %v, want result=dead_letter", m.finishCalls)
	}
}

func TestProcessorRecordsWrongAnswerNotAccepted(t *testing.T) {
	// 验证 judge 返回 wrong_answer 时，metrics 记录 wrong_answer 而非 accepted。
	calls := []string{}
	st := &fakeStore{claim: acquiredClaim(), problem: domain.Problem{ID: "sum"}, completeOK: true, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{result: domain.JudgeResult{Status: domain.StatusWrongAnswer}, calls: &calls}
	m := &fakeWorkerMetrics{}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{
		WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour,
		Token: func() (string, error) { return "token", nil }, Metrics: m,
	})
	if err := p.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.finishCalls) != 1 || m.finishCalls[0][1] != "wrong_answer" {
		t.Errorf("finish calls = %v, want result=wrong_answer", m.finishCalls)
	}
}

func TestProcessorSkipsRetryCounterOnReleaseFailure(t *testing.T) {
	// 验证 ReleaseSubmission 失败时不增加 retry 计数器。
	calls := []string{}
	st := &fakeStore{claim: acquiredClaim(), problem: domain.Problem{ID: "sum"}, releaseOK: false, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{err: errors.New("docker unavailable"), calls: &calls}
	m := &fakeWorkerMetrics{}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{
		WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour,
		MaxAttempts: 3, Token: func() (string, error) { return "token", nil }, Metrics: m,
	})
	_ = p.ProcessOne(context.Background())
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.retries != 0 {
		t.Errorf("retries = %d, want 0 (release failed)", m.retries)
	}
}

func TestProcessorSkipsDeadLetterCounterOnCompleteFailure(t *testing.T) {
	// 验证 CompleteSubmission 失败时不增加 dead-letter 计数器。
	calls := []string{}
	claim := acquiredClaim()
	claim.Attempts = 3
	st := &fakeStore{claim: claim, problem: domain.Problem{ID: "sum"}, completeOK: false, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{err: errors.New("docker unavailable"), calls: &calls}
	m := &fakeWorkerMetrics{}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{
		WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour,
		MaxAttempts: 3, Token: func() (string, error) { return "token", nil }, Metrics: m,
	})
	_ = p.ProcessOne(context.Background())
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deadLetters != 0 {
		t.Errorf("deadLetters = %d, want 0 (complete failed)", m.deadLetters)
	}
}

func TestProcessorJobFinishedResultLabelForRetry(t *testing.T) {
	// 验证 infrastructure retry 路径记录 result="retry"。
	calls := []string{}
	st := &fakeStore{claim: acquiredClaim(), problem: domain.Problem{ID: "sum"}, releaseOK: true, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{err: errors.New("docker unavailable"), calls: &calls}
	m := &fakeWorkerMetrics{}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{
		WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour,
		MaxAttempts: 3, Token: func() (string, error) { return "token", nil }, Metrics: m,
	})
	_ = p.ProcessOne(context.Background())
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.finishCalls) != 1 || m.finishCalls[0][1] != "retry" {
		t.Errorf("finish calls = %v, want result=retry", m.finishCalls)
	}
}

func TestProcessorJobFinishedResultLabelForDeadLetter(t *testing.T) {
	// 验证 dead-letter 路径记录 result="dead_letter"。
	calls := []string{}
	claim := acquiredClaim()
	claim.Attempts = 3
	st := &fakeStore{claim: claim, problem: domain.Problem{ID: "sum"}, completeOK: true, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{err: errors.New("docker unavailable"), calls: &calls}
	m := &fakeWorkerMetrics{}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{
		WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour,
		MaxAttempts: 3, Token: func() (string, error) { return "token", nil }, Metrics: m,
	})
	_ = p.ProcessOne(context.Background())
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.finishCalls) != 1 || m.finishCalls[0][1] != "dead_letter" {
		t.Errorf("finish calls = %v, want result=dead_letter", m.finishCalls)
	}
}

func TestProcessorRecordsTakeoverMetric(t *testing.T) {
	calls := []string{}
	claim := acquiredClaim()
	claim.LeaseTakeover = true
	st := &fakeStore{claim: claim, problem: domain.Problem{ID: "sum"}, completeOK: true, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{result: domain.JudgeResult{Status: domain.StatusAccepted}, calls: &calls}
	m := &fakeWorkerMetrics{}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{
		WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour,
		Token: func() (string, error) { return "token", nil }, Metrics: m,
	})
	if err := p.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.leaseTakeovers != 1 {
		t.Errorf("leaseTakeovers = %d, want 1", m.leaseTakeovers)
	}
}

type fakeStore struct {
	claim       domain.SubmissionClaim
	problem     domain.Problem
	completeOK  bool
	releaseOK   bool
	renewOK     bool
	completeErr error
	releaseErr  error
	calls       *[]string
}

func (s *fakeStore) GetProblem(context.Context, string) (domain.Problem, bool, error) {
	*s.calls = append(*s.calls, "problem")
	return s.problem, s.problem.ID != "", nil
}
func (s *fakeStore) ClaimSubmission(context.Context, string, string, string, string, time.Time, time.Duration) (domain.SubmissionClaim, error) {
	*s.calls = append(*s.calls, "claim")
	return s.claim, nil
}
func (s *fakeStore) RenewSubmissionLease(context.Context, string, string, time.Time, time.Duration) (bool, error) {
	*s.calls = append(*s.calls, "renew")
	return s.renewOK, nil
}
func (s *fakeStore) CompleteSubmission(context.Context, string, string, time.Time, domain.JudgeResult) (bool, error) {
	*s.calls = append(*s.calls, "complete")
	return s.completeOK, s.completeErr
}
func (s *fakeStore) ReleaseSubmission(context.Context, string, string, time.Time, string) (bool, error) {
	*s.calls = append(*s.calls, "release")
	return s.releaseOK, s.releaseErr
}

type fakeQueue struct {
	job       domain.Job
	calls     *[]string
	retryErr  error
	deadErr   error
}

func (q *fakeQueue) Dequeue(context.Context) (domain.Job, error) {
	*q.calls = append(*q.calls, "dequeue")
	return q.job, nil
}
func (q *fakeQueue) Touch(context.Context, domain.Job) error {
	*q.calls = append(*q.calls, "touch")
	return nil
}
func (q *fakeQueue) Ack(context.Context, domain.Job) error {
	*q.calls = append(*q.calls, "ack")
	return nil
}
func (q *fakeQueue) RetryJob(context.Context, domain.Job, int, error) error {
	*q.calls = append(*q.calls, "retry")
	return q.retryErr
}
func (q *fakeQueue) DeadLetter(context.Context, domain.Job, int, error) error {
	*q.calls = append(*q.calls, "dead")
	return q.deadErr
}

type fakeJudge struct {
	result domain.JudgeResult
	err    error
	calls  *[]string
	block  bool
}

func (j *fakeJudge) Evaluate(ctx context.Context, _ domain.Problem, _ domain.Language, _ string) (domain.JudgeResult, error) {
	*j.calls = append(*j.calls, "judge")
	if j.block {
		<-ctx.Done()
		return domain.JudgeResult{}, ctx.Err()
	}
	return j.result, j.err
}

func TestProcessorCancelsJudgeWhenLeaseIsLost(t *testing.T) {
	calls := []string{}
	st := &fakeStore{claim: acquiredClaim(), problem: domain.Problem{ID: "sum"}, renewOK: false, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{block: true, calls: &calls}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{
		WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: 5 * time.Millisecond,
		Token: func() (string, error) { return "token", nil },
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := p.ProcessOne(ctx)
	if !errors.Is(err, judgeworker.ErrLeaseLost) {
		t.Fatalf("error = %v, want lease lost", err)
	}
	if elapsed := time.Since(started); elapsed >= 50*time.Millisecond {
		t.Fatalf("judge cancellation took %v", elapsed)
	}
}

func acquiredClaim() domain.SubmissionClaim {
	return domain.SubmissionClaim{
		State: domain.ClaimAcquired, Token: "token", Attempts: 1,
		Submission: domain.Submission{ID: "sub-1", ProblemID: "sum", Language: domain.LanguageGo, Code: "code", Status: domain.StatusRunning},
	}
}

func TestProcessorPersistsResultBeforeAck(t *testing.T) {
	calls := []string{}
	st := &fakeStore{claim: acquiredClaim(), problem: domain.Problem{ID: "sum"}, completeOK: true, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{result: domain.JudgeResult{Status: domain.StatusAccepted}, calls: &calls}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour, Token: func() (string, error) { return "token", nil }})
	if err := p.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"dequeue", "claim", "problem", "judge", "complete", "ack"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestProcessorHandlesTerminalAndDuplicateClaims(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state domain.ClaimState
		ack   bool
	}{
		{name: "terminal", state: domain.ClaimTerminal, ack: true},
		{name: "other receipt", state: domain.ClaimActiveOtherReceipt, ack: true},
		{name: "same receipt", state: domain.ClaimActiveSameReceipt, ack: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			st := &fakeStore{claim: domain.SubmissionClaim{State: tc.state}, calls: &calls}
			q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
			j := &fakeJudge{calls: &calls}
			p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour, Token: func() (string, error) { return "token", nil }})
			if err := p.ProcessOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			want := []string{"dequeue", "claim"}
			if tc.ack {
				want = append(want, "ack")
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls = %v, want %v", calls, want)
			}
		})
	}
}

func TestProcessorRetriesInfrastructureFailure(t *testing.T) {
	calls := []string{}
	st := &fakeStore{claim: acquiredClaim(), problem: domain.Problem{ID: "sum"}, releaseOK: true, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{err: errors.New("docker unavailable"), calls: &calls}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour, MaxAttempts: 3, Token: func() (string, error) { return "token", nil }})
	if err := p.ProcessOne(context.Background()); err == nil {
		t.Fatal("ProcessOne should report infrastructure failure")
	}
	want := []string{"dequeue", "claim", "problem", "judge", "release", "retry"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestProcessorDeadLettersThirdInfrastructureFailure(t *testing.T) {
	calls := []string{}
	claim := acquiredClaim()
	claim.Attempts = 3
	st := &fakeStore{claim: claim, problem: domain.Problem{ID: "sum"}, completeOK: true, calls: &calls}
	q := &fakeQueue{job: domain.Job{SubmissionID: "sub-1", Receipt: "1-0"}, calls: &calls}
	j := &fakeJudge{err: errors.New("docker unavailable"), calls: &calls}
	p := judgeworker.NewProcessor(st, q, j, judgeworker.Config{WorkerID: "worker-a", LeaseDuration: time.Minute, HeartbeatInterval: time.Hour, MaxAttempts: 3, Token: func() (string, error) { return "token", nil }})
	if err := p.ProcessOne(context.Background()); err == nil {
		t.Fatal("ProcessOne should report infrastructure failure")
	}
	want := []string{"dequeue", "claim", "problem", "judge", "complete", "dead"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}
