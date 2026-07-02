package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/store"
)

func TestMemoryStoreCreatesQueuedSubmission(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore([]domain.Problem{
		{ID: "sum", Title: "A+B"},
	})

	sub, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "package main\nfunc main(){}",
	})
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}

	if sub.ID == "" {
		t.Fatal("CreateSubmission should assign an ID")
	}
	if sub.Status != domain.StatusQueued {
		t.Fatalf("status = %q, want %q", sub.Status, domain.StatusQueued)
	}

	got, ok, err := st.GetSubmission(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("GetSubmission returned error: %v", err)
	}
	if !ok {
		t.Fatal("created submission was not found")
	}
	if got.ProblemID != "sum" || got.Language != domain.LanguageGo {
		t.Fatalf("stored submission = %+v", got)
	}
}

func TestMemoryStoreUpdatesSubmissionResult(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore([]domain.Problem{{ID: "echo", Title: "Echo"}})
	sub, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "echo",
		Language:  domain.LanguageGo,
		Code:      "code",
	})
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}

	result := domain.JudgeResult{
		Status:   domain.StatusAccepted,
		Stdout:   "ok\n",
		Duration: 12,
	}
	now := time.Now().UTC()
	claim, err := st.ClaimSubmission(context.Background(), sub.ID, "worker", "token", "1-0", now, time.Minute)
	if err != nil || claim.State != domain.ClaimAcquired {
		t.Fatalf("ClaimSubmission = %+v, %v", claim, err)
	}
	if ok, err := st.CompleteSubmission(context.Background(), sub.ID, "token", now.Add(time.Second), result); err != nil || !ok {
		t.Fatalf("CompleteSubmission = %v, %v", ok, err)
	}

	got, ok, err := st.GetSubmission(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("GetSubmission returned error: %v", err)
	}
	if !ok {
		t.Fatal("updated submission was not found")
	}
	if got.Status != domain.StatusAccepted {
		t.Fatalf("status = %q, want %q", got.Status, domain.StatusAccepted)
	}
	if got.Result == nil || got.Result.Stdout != "ok\n" {
		t.Fatalf("result = %+v, want stdout ok", got.Result)
	}
}

func TestMemoryStoreListsSubmissionsNewestFirst(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore([]domain.Problem{{ID: "sum", Title: "A+B"}})
	first, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "first",
	})
	if err != nil {
		t.Fatalf("CreateSubmission first returned error: %v", err)
	}
	second, err := st.CreateSubmission(context.Background(), domain.Submission{
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "second",
	})
	if err != nil {
		t.Fatalf("CreateSubmission second returned error: %v", err)
	}

	got, err := st.ListSubmissions(context.Background())
	if err != nil {
		t.Fatalf("ListSubmissions returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("submission count = %d, want 2", len(got))
	}
	if got[0].ID != second.ID || got[1].ID != first.ID {
		t.Fatalf("submission order = [%s, %s], want [%s, %s]", got[0].ID, got[1].ID, second.ID, first.ID)
	}
}
