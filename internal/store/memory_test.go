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

func TestMemoryStoreUsersAreCaseInsensitiveUnique(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore(nil)
	ctx := context.Background()

	user, err := st.CreateUser(ctx, "Kai", "hash-1")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	if user.ID == "" || user.Username != "Kai" {
		t.Fatalf("user = %+v", user)
	}

	if _, err := st.CreateUser(ctx, "kai", "hash-2"); err == nil {
		t.Fatal("CreateUser should reject duplicate usernames case-insensitively")
	}

	got, ok, err := st.GetUserByUsername(ctx, "KAI")
	if err != nil {
		t.Fatalf("GetUserByUsername returned error: %v", err)
	}
	if !ok || got.ID != user.ID {
		t.Fatalf("GetUserByUsername = %+v, %v; want user %s", got, ok, user.ID)
	}
}

func TestMemoryStoreSessionLookupHonorsExpiry(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore(nil)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "kai", "hash")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	if _, err := st.CreateSession(ctx, user.ID, "token-hash", expiresAt); err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	got, ok, err := st.GetUserBySessionTokenHash(ctx, "token-hash", expiresAt.Add(-time.Second))
	if err != nil {
		t.Fatalf("GetUserBySessionTokenHash returned error: %v", err)
	}
	if !ok || got.ID != user.ID {
		t.Fatalf("active session lookup = %+v, %v; want user %s", got, ok, user.ID)
	}

	got, ok, err = st.GetUserBySessionTokenHash(ctx, "token-hash", expiresAt.Add(time.Second))
	if err != nil {
		t.Fatalf("expired GetUserBySessionTokenHash returned error: %v", err)
	}
	if ok || got.ID != "" {
		t.Fatalf("expired session lookup = %+v, %v; want miss", got, ok)
	}
}

func TestMemoryStoreListsSubmissionsByUser(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore([]domain.Problem{{ID: "sum", Title: "A+B"}})
	ctx := context.Background()
	kai, err := st.CreateUser(ctx, "kai", "hash")
	if err != nil {
		t.Fatalf("CreateUser kai returned error: %v", err)
	}
	lin, err := st.CreateUser(ctx, "lin", "hash")
	if err != nil {
		t.Fatalf("CreateUser lin returned error: %v", err)
	}
	first, err := st.CreateSubmission(ctx, domain.Submission{
		UserID:    kai.ID,
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "first",
	})
	if err != nil {
		t.Fatalf("CreateSubmission first returned error: %v", err)
	}
	if _, err := st.CreateSubmission(ctx, domain.Submission{
		UserID:    lin.ID,
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "other user",
	}); err != nil {
		t.Fatalf("CreateSubmission other returned error: %v", err)
	}
	second, err := st.CreateSubmission(ctx, domain.Submission{
		UserID:    kai.ID,
		ProblemID: "sum",
		Language:  domain.LanguageGo,
		Code:      "second",
	})
	if err != nil {
		t.Fatalf("CreateSubmission second returned error: %v", err)
	}

	got, err := st.ListSubmissionsByUser(ctx, kai.ID)
	if err != nil {
		t.Fatalf("ListSubmissionsByUser returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("submission count = %d, want 2", len(got))
	}
	if got[0].ID != second.ID || got[1].ID != first.ID {
		t.Fatalf("submission order = [%s, %s], want [%s, %s]", got[0].ID, got[1].ID, second.ID, first.ID)
	}

	if _, ok, err := st.GetSubmissionForUser(ctx, first.ID, lin.ID); err != nil || ok {
		t.Fatalf("GetSubmissionForUser for other user = ok %v err %v, want miss", ok, err)
	}
}

func TestMemoryStoreLeaderboardCountsAcceptedUniqueProblems(t *testing.T) {
	t.Parallel()

	st := store.NewMemoryStore([]domain.Problem{
		{ID: "sum", Title: "A+B"},
		{ID: "echo", Title: "Echo"},
	})
	ctx := context.Background()
	kai, err := st.CreateUser(ctx, "kai", "hash")
	if err != nil {
		t.Fatalf("CreateUser kai returned error: %v", err)
	}
	lin, err := st.CreateUser(ctx, "lin", "hash")
	if err != nil {
		t.Fatalf("CreateUser lin returned error: %v", err)
	}

	completeMemorySubmission(t, st, domain.Submission{UserID: kai.ID, ProblemID: "sum", Language: domain.LanguageGo, Code: "ok"}, domain.StatusAccepted)
	completeMemorySubmission(t, st, domain.Submission{UserID: kai.ID, ProblemID: "sum", Language: domain.LanguageGo, Code: "duplicate ok"}, domain.StatusAccepted)
	completeMemorySubmission(t, st, domain.Submission{UserID: kai.ID, ProblemID: "echo", Language: domain.LanguageGo, Code: "wa"}, domain.StatusWrongAnswer)
	completeMemorySubmission(t, st, domain.Submission{UserID: lin.ID, ProblemID: "sum", Language: domain.LanguageGo, Code: "ok"}, domain.StatusAccepted)
	completeMemorySubmission(t, st, domain.Submission{UserID: lin.ID, ProblemID: "echo", Language: domain.LanguageGo, Code: "ok"}, domain.StatusAccepted)

	entries, err := st.ListLeaderboard(ctx, 10)
	if err != nil {
		t.Fatalf("ListLeaderboard returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if entries[0].Username != "lin" || entries[0].Solved != 2 || entries[0].AcceptedSubmissions != 2 || entries[0].Rank != 1 {
		t.Fatalf("first entry = %+v, want lin solved=2 accepted=2 rank=1", entries[0])
	}
	if entries[1].Username != "kai" || entries[1].Solved != 1 || entries[1].AcceptedSubmissions != 2 || entries[1].Rank != 2 {
		t.Fatalf("second entry = %+v, want kai solved=1 accepted=2 rank=2", entries[1])
	}
}

func completeMemorySubmission(t *testing.T, st *store.MemoryStore, sub domain.Submission, status domain.SubmissionStatus) {
	t.Helper()

	created, err := st.CreateSubmission(context.Background(), sub)
	if err != nil {
		t.Fatalf("CreateSubmission returned error: %v", err)
	}
	now := time.Now().UTC()
	claim, err := st.ClaimSubmission(context.Background(), created.ID, "worker", "token-"+created.ID, "1-0", now, time.Minute)
	if err != nil || claim.State != domain.ClaimAcquired {
		t.Fatalf("ClaimSubmission = %+v, %v", claim, err)
	}
	if ok, err := st.CompleteSubmission(context.Background(), created.ID, "token-"+created.ID, now.Add(time.Second), domain.JudgeResult{Status: status}); err != nil || !ok {
		t.Fatalf("CompleteSubmission = %v, %v", ok, err)
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
