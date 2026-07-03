package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/store"
)

func TestMemoryClaimLeaseAndFenceStaleWorker(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 2, 6, 0, 0, 0, time.UTC)
	st := store.NewMemoryStore([]domain.Problem{{ID: "sum"}})
	sub, err := st.CreateSubmission(ctx, domain.Submission{ProblemID: "sum", Language: domain.LanguageGo, Code: "code"})
	if err != nil {
		t.Fatal(err)
	}

	first, err := st.ClaimSubmission(ctx, sub.ID, "worker-a", "token-a", "1-0", now, 30*time.Second)
	if err != nil || first.State != domain.ClaimAcquired || first.Attempts != 1 {
		t.Fatalf("first claim = %+v, %v", first, err)
	}
	other, err := st.ClaimSubmission(ctx, sub.ID, "worker-b", "token-b", "2-0", now.Add(time.Second), 30*time.Second)
	if err != nil || other.State != domain.ClaimActiveOtherReceipt {
		t.Fatalf("other claim = %+v, %v", other, err)
	}
	same, err := st.ClaimSubmission(ctx, sub.ID, "worker-b", "token-b", "1-0", now.Add(time.Second), 30*time.Second)
	if err != nil || same.State != domain.ClaimActiveSameReceipt {
		t.Fatalf("same claim = %+v, %v", same, err)
	}
	replacement, err := st.ClaimSubmission(ctx, sub.ID, "worker-b", "token-b", "1-0", now.Add(31*time.Second), 30*time.Second)
	if err != nil || replacement.State != domain.ClaimAcquired || replacement.Attempts != 2 {
		t.Fatalf("replacement = %+v, %v", replacement, err)
	}
	if ok, err := st.CompleteSubmission(ctx, sub.ID, "token-a", now.Add(32*time.Second), domain.JudgeResult{Status: domain.StatusAccepted}); err != nil || ok {
		t.Fatalf("stale completion = %v, %v", ok, err)
	}
	if ok, err := st.CompleteSubmission(ctx, sub.ID, "token-b", now.Add(32*time.Second), domain.JudgeResult{Status: domain.StatusAccepted}); err != nil || !ok {
		t.Fatalf("active completion = %v, %v", ok, err)
	}
	terminal, err := st.ClaimSubmission(ctx, sub.ID, "worker-c", "token-c", "3-0", now.Add(33*time.Second), 30*time.Second)
	if err != nil || terminal.State != domain.ClaimTerminal {
		t.Fatalf("terminal claim = %+v, %v", terminal, err)
	}
}

func TestMemoryLeaseRenewReleaseAndExpiry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 2, 7, 0, 0, 0, time.UTC)
	st := store.NewMemoryStore([]domain.Problem{{ID: "sum"}})
	sub, _ := st.CreateSubmission(ctx, domain.Submission{ProblemID: "sum", Language: domain.LanguageGo, Code: "code"})
	_, _ = st.ClaimSubmission(ctx, sub.ID, "worker-a", "token-a", "1-0", now, 30*time.Second)

	if ok, err := st.RenewSubmissionLease(ctx, sub.ID, "stale", now.Add(10*time.Second), 30*time.Second); err != nil || ok {
		t.Fatalf("stale renew = %v, %v", ok, err)
	}
	if ok, err := st.RenewSubmissionLease(ctx, sub.ID, "token-a", now.Add(10*time.Second), 30*time.Second); err != nil || !ok {
		t.Fatalf("renew = %v, %v", ok, err)
	}
	if ok, err := st.CompleteSubmission(ctx, sub.ID, "token-a", now.Add(41*time.Second), domain.JudgeResult{Status: domain.StatusAccepted}); err != nil || ok {
		t.Fatalf("expired completion = %v, %v", ok, err)
	}
	if ok, err := st.ReleaseSubmission(ctx, sub.ID, "token-a", now.Add(20*time.Second), "docker unavailable"); err != nil || !ok {
		t.Fatalf("release = %v, %v", ok, err)
	}
	got, ok, err := st.GetSubmission(ctx, sub.ID)
	if err != nil || !ok || got.Status != domain.StatusQueued {
		t.Fatalf("submission = %+v, %v, %v", got, ok, err)
	}
}

func TestMemoryTakeoverClassification(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	st := store.NewMemoryStore([]domain.Problem{{ID: "sum"}})
	sub, _ := st.CreateSubmission(ctx, domain.Submission{ProblemID: "sum", Language: domain.LanguageGo, Code: "code"})

	// 首次认领不是接管。
	first, err := st.ClaimSubmission(ctx, sub.ID, "w1", "t1", "r1", now, 30*time.Second)
	if err != nil || first.LeaseTakeover {
		t.Fatalf("first claim should not be takeover: %+v, %v", first, err)
	}

	// 显式释放后（状态变回 queued），重新认领也不是接管。
	if _, err := st.ReleaseSubmission(ctx, sub.ID, "t1", now.Add(10*time.Second), "err"); err != nil {
		t.Fatal(err)
	}
	retry, err := st.ClaimSubmission(ctx, sub.ID, "w2", "t2", "r2", now.Add(20*time.Second), 30*time.Second)
	if err != nil || retry.LeaseTakeover || retry.Attempts != 2 {
		t.Fatalf("retry claim should not be takeover: %+v, %v", retry, err)
	}

	// 租约过期后的认领是接管。
	_, _ = st.ClaimSubmission(ctx, sub.ID, "w3", "t3", "r3", now.Add(20*time.Second), 30*time.Second)
	// 续租可保持 active
	if _, err := st.RenewSubmissionLease(ctx, sub.ID, "t3", now.Add(40*time.Second), 30*time.Second); err != nil {
		t.Fatal(err)
	}
	// 时间推进到超过租约过期后，新 worker 认领应为接管
	takeover, err := st.ClaimSubmission(ctx, sub.ID, "w4", "t4", "r4", now.Add(100*time.Second), 30*time.Second)
	if err != nil || !takeover.LeaseTakeover || takeover.Attempts != 3 {
		t.Fatalf("expired lease takeover: %+v, %v", takeover, err)
	}
}

func TestMemorySubmissionCreatesAndPublishesOutbox(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := store.NewMemoryStore([]domain.Problem{{ID: "sum"}})
	sub, err := st.CreateSubmission(ctx, domain.Submission{ProblemID: "sum", Language: domain.LanguageGo, Code: "code"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Second)
	events, err := st.ClaimOutbox(ctx, "api-1", now, 30*time.Second, 10)
	if err != nil || len(events) != 1 || events[0].SubmissionID != sub.ID || events[0].PublishAttempts != 1 {
		t.Fatalf("events = %+v, %v", events, err)
	}
	if ok, err := st.MarkOutboxPublished(ctx, events[0].ID, "other", now); err != nil || ok {
		t.Fatalf("foreign publish = %v, %v", ok, err)
	}
	if ok, err := st.MarkOutboxPublished(ctx, events[0].ID, "api-1", now); err != nil || !ok {
		t.Fatalf("publish = %v, %v", ok, err)
	}
	events, err = st.ClaimOutbox(ctx, "api-2", now.Add(time.Minute), 30*time.Second, 10)
	if err != nil || len(events) != 0 {
		t.Fatalf("published events = %+v, %v", events, err)
	}
}
