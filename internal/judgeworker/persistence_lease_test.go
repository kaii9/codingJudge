package judgeworker_test

import (
	"context"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/judgeworker"
	"github.com/kaii9/codingJudge/internal/store"
	"testing"
	"time"
)

type slowArtifacts struct{}

func (slowArtifacts) Get(context.Context, string) ([]byte, error) { return nil, nil }
func (slowArtifacts) Put(ctx context.Context, _ string, _ []byte, _ string) error {
	select {
	case <-time.After(120 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type quietQueue struct{ job domain.Job }

func (q quietQueue) Dequeue(context.Context) (domain.Job, error)              { return q.job, nil }
func (q quietQueue) Touch(context.Context, domain.Job) error                  { return nil }
func (q quietQueue) Ack(context.Context, domain.Job) error                    { return nil }
func (q quietQueue) RetryJob(context.Context, domain.Job, int, error) error   { return nil }
func (q quietQueue) DeadLetter(context.Context, domain.Job, int, error) error { return nil }

type acceptedJudge struct{}

func (acceptedJudge) Evaluate(context.Context, domain.Problem, domain.Language, string) (domain.JudgeResult, error) {
	return domain.JudgeResult{Status: domain.StatusAccepted}, nil
}
func TestLeaseHeartbeatContinuesDuringArtifactPersistence(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore([]domain.Problem{{ID: "sum"}})
	sub, err := st.CreateSubmission(ctx, domain.Submission{ProblemID: "sum", Language: domain.LanguageGo, Code: "code"})
	if err != nil {
		t.Fatal(err)
	}
	p := judgeworker.NewProcessor(st, quietQueue{job: domain.Job{SubmissionID: sub.ID, Receipt: "1-0"}}, acceptedJudge{}, judgeworker.Config{
		WorkerID: "worker", LeaseDuration: 60 * time.Millisecond, HeartbeatInterval: 10 * time.Millisecond, ArtifactStore: slowArtifacts{},
	})
	if err := p.ProcessOne(ctx); err != nil {
		t.Fatalf("slow persistence lost lease: %v", err)
	}
	final, ok, err := st.GetSubmission(ctx, sub.ID)
	if err != nil || !ok || final.Status != domain.StatusAccepted {
		t.Fatalf("final=%+v err=%v", final, err)
	}
}
