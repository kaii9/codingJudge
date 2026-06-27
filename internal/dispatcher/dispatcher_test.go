package dispatcher_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kai/codingjudge/internal/dispatcher"
	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/queue"
	"github.com/kai/codingjudge/internal/store"
)

type fakeJudgeClient struct {
	got   dispatcher.JudgeRequest
	calls int
	err   error
}

func (f *fakeJudgeClient) Judge(ctx context.Context, req dispatcher.JudgeRequest) (domain.JudgeResult, error) {
	f.got = req
	f.calls++
	if f.err != nil {
		return domain.JudgeResult{}, f.err
	}
	return domain.JudgeResult{Status: domain.StatusAccepted, Stdout: "3\n"}, nil
}

type reliableQueue struct {
	job     domain.Job
	acked   int
	retried int
	dead    bool
}

func (q *reliableQueue) Dequeue(context.Context) (domain.Job, error) {
	return q.job, nil
}

func (q *reliableQueue) Ack(context.Context, domain.Job) error {
	q.acked++
	return nil
}

func (q *reliableQueue) Retry(context.Context, domain.Job, error) (bool, error) {
	q.retried++
	return q.dead, nil
}

type failingResultStore struct {
	*store.MemoryStore
}

func (s *failingResultStore) UpdateSubmissionResult(context.Context, string, domain.JudgeResult) error {
	return errors.New("database unavailable")
}

func TestProcessOneSendsSubmissionToJudgeClientAndStoresResult(t *testing.T) {
	t.Parallel()

	problems := []domain.Problem{{
		ID:          "sum",
		Title:       "A+B",
		Language:    domain.LanguageGo,
		TimeLimitMS: 1000,
		TestCases: []domain.TestCase{
			{Input: "1 2\n", ExpectedOutput: "3\n"},
		},
	}}
	st := store.NewMemoryStore(problems)
	q := queue.NewMemoryQueue(1)
	sub, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "package main",
	})
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}
	if err := q.Enqueue(context.Background(), domain.Job{SubmissionID: sub.ID}); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}

	client := &fakeJudgeClient{}
	d := dispatcher.New(st, q, client)
	if err := d.ProcessOne(context.Background()); err != nil {
		t.Fatalf("ProcessOne returned error: %v", err)
	}

	if client.got.SubmissionID != sub.ID || client.got.Code != "package main" {
		t.Fatalf("judge request = %+v", client.got)
	}
	got, ok, err := st.GetSubmission(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("GetSubmission returned error: %v", err)
	}
	if !ok {
		t.Fatal("submission not found")
	}
	if got.Status != domain.StatusAccepted {
		t.Fatalf("status = %q, want %q", got.Status, domain.StatusAccepted)
	}
}

func TestProcessOneAcknowledgesOnlyAfterResultIsStored(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore([]domain.Problem{{
		ID: "sum",
		TestCases: []domain.TestCase{
			{Input: "1 2\n", ExpectedOutput: "3\n"},
		},
	}})
	sub, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "package main",
	})
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}
	q := &reliableQueue{job: domain.Job{SubmissionID: sub.ID}}

	if err := dispatcher.New(st, q, &fakeJudgeClient{}).ProcessOne(context.Background()); err != nil {
		t.Fatalf("ProcessOne returned error: %v", err)
	}
	if q.acked != 1 || q.retried != 0 {
		t.Fatalf("queue calls: acked=%d retried=%d, want 1 and 0", q.acked, q.retried)
	}
}

func TestProcessOneRetriesWhenResultPersistenceFails(t *testing.T) {
	t.Parallel()

	memoryStore := store.NewMemoryStore([]domain.Problem{{ID: "sum"}})
	sub, err := memoryStore.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "package main",
	})
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}
	q := &reliableQueue{job: domain.Job{SubmissionID: sub.ID}}
	st := &failingResultStore{MemoryStore: memoryStore}

	if err := dispatcher.New(st, q, &fakeJudgeClient{}).ProcessOne(context.Background()); err == nil {
		t.Fatal("ProcessOne should return the persistence error")
	}
	if q.acked != 0 || q.retried != 1 {
		t.Fatalf("queue calls: acked=%d retried=%d, want 0 and 1", q.acked, q.retried)
	}
}

func TestProcessOneAcknowledgesCompletedSubmissionWithoutJudgingAgain(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore([]domain.Problem{{ID: "sum"}})
	sub, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "package main",
	})
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}
	if err := st.UpdateSubmissionResult(context.Background(), sub.ID, domain.JudgeResult{Status: domain.StatusAccepted}); err != nil {
		t.Fatalf("UpdateSubmissionResult returned error: %v", err)
	}
	q := &reliableQueue{job: domain.Job{SubmissionID: sub.ID}}
	client := &fakeJudgeClient{}

	if err := dispatcher.New(st, q, client).ProcessOne(context.Background()); err != nil {
		t.Fatalf("ProcessOne returned error: %v", err)
	}
	if client.calls != 0 {
		t.Fatalf("judge client calls = %d, want 0", client.calls)
	}
	if q.acked != 1 || q.retried != 0 {
		t.Fatalf("queue calls: acked=%d retried=%d, want 1 and 0", q.acked, q.retried)
	}
}

func TestProcessOneRequeuesSubmissionWhenJudgeIsUnavailable(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore([]domain.Problem{{ID: "sum"}})
	sub, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "package main",
	})
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}
	q := &reliableQueue{job: domain.Job{SubmissionID: sub.ID}}
	client := &fakeJudgeClient{err: errors.New("worker unavailable")}

	if err := dispatcher.New(st, q, client).ProcessOne(context.Background()); err == nil {
		t.Fatal("ProcessOne should return the judge transport error")
	}
	got, ok, err := st.GetSubmission(context.Background(), sub.ID)
	if err != nil || !ok {
		t.Fatalf("GetSubmission = %+v, %v, %v", got, ok, err)
	}
	if got.Status != domain.StatusQueued || got.Result != nil {
		t.Fatalf("submission after retry = %+v, want queued without result", got)
	}
	if q.acked != 0 || q.retried != 1 {
		t.Fatalf("queue calls: acked=%d retried=%d, want 0 and 1", q.acked, q.retried)
	}
}

func TestProcessOneStoresInternalErrorWhenJobIsDeadLettered(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore([]domain.Problem{{ID: "sum"}})
	sub, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "package main",
	})
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}
	q := &reliableQueue{job: domain.Job{SubmissionID: sub.ID}, dead: true}
	client := &fakeJudgeClient{err: errors.New("worker unavailable")}

	if err := dispatcher.New(st, q, client).ProcessOne(context.Background()); err == nil {
		t.Fatal("ProcessOne should report the exhausted judge error")
	}
	got, ok, err := st.GetSubmission(context.Background(), sub.ID)
	if err != nil || !ok {
		t.Fatalf("GetSubmission = %+v, %v, %v", got, ok, err)
	}
	if got.Status != domain.StatusInternalError || got.Result == nil {
		t.Fatalf("submission after dead letter = %+v, want internal_error result", got)
	}
	if q.acked != 0 || q.retried != 1 {
		t.Fatalf("queue calls: acked=%d retried=%d, want 0 and 1", q.acked, q.retried)
	}
}
