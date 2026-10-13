package store_test

import (
	"context"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/store"
	"testing"
	"time"
)

func TestDuplicateCompletionProjectsOneSolvedProblem(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore(nil)
	user, err := st.CreateUser(ctx, "alice", "hash")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	first := domain.CompletionEvent{ID: "event-1", SubmissionID: "sub-1", UserID: user.ID, ProblemID: "sum", Status: domain.StatusAccepted, Version: 1, FinishedAt: at}
	second := first
	second.ID = "event-2"
	second.SubmissionID = "sub-2"
	second.FinishedAt = at.Add(time.Second)
	for _, event := range []domain.CompletionEvent{first, first, second} {
		if err := st.ApplyCompletionEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	leaders, err := st.ListProjectedLeaderboard(ctx, 50)
	if err != nil || len(leaders) != 1 || leaders[0].Solved != 1 || leaders[0].AcceptedSubmissions != 2 {
		t.Fatalf("leaders=%+v err=%v", leaders, err)
	}
	old := second
	old.ID = "old-event"
	old.Version = 0
	old.Status = domain.StatusWrongAnswer
	if err := st.ApplyCompletionEvent(ctx, old); err == nil {
		t.Fatal("invalid event version accepted")
	}
}
func TestProjectionIgnoresLateOlderVersion(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore(nil)
	user, _ := st.CreateUser(ctx, "alice", "hash")
	current := domain.CompletionEvent{ID: "new", SubmissionID: "sub", UserID: user.ID, ProblemID: "sum", Status: domain.StatusAccepted, Version: 2, FinishedAt: time.Now()}
	old := current
	old.ID = "old"
	old.Version = 1
	old.Status = domain.StatusWrongAnswer
	if err := st.ApplyCompletionEvent(ctx, current); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyCompletionEvent(ctx, old); err != nil {
		t.Fatal(err)
	}
	leaders, err := st.ListProjectedLeaderboard(ctx, 50)
	if err != nil || len(leaders) != 1 || leaders[0].Solved != 1 {
		t.Fatalf("leaders=%+v err=%v", leaders, err)
	}
}
