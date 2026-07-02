//go:build integration

package store

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kai/codingjudge/internal/domain"
)

func integrationStore(t *testing.T) *PostgresStore {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE IF EXISTS judge_outbox, submissions, problem_test_cases, problems CASCADE`,
		mustReadMigration(t, "../../migrations/001_init.sql"),
		mustReadMigration(t, "../../migrations/002_seed.sql"),
		mustReadMigration(t, "../../migrations/003_reliable_workers.sql"),
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			pool.Close()
			t.Fatal(err)
		}
	}
	t.Cleanup(pool.Close)
	return &PostgresStore{pool: pool}
}

func mustReadMigration(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPostgresSubmissionAndOutboxAreAtomic(t *testing.T) {
	st := integrationStore(t)
	ctx := context.Background()
	sub, err := st.CreateSubmission(ctx, domain.Submission{ProblemID: "sum", Language: domain.LanguageGo, Code: "code"})
	if err != nil {
		t.Fatal(err)
	}
	events, err := st.ClaimOutbox(ctx, "api-1", time.Now().UTC(), 30*time.Second, 10)
	if err != nil || len(events) != 1 || events[0].SubmissionID != sub.ID {
		t.Fatalf("events = %+v, %v", events, err)
	}
}

func TestPostgresConcurrentClaimsHaveOneWinner(t *testing.T) {
	st := integrationStore(t)
	ctx := context.Background()
	sub, err := st.CreateSubmission(ctx, domain.Submission{ProblemID: "sum", Language: domain.LanguageGo, Code: "code"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	results := make(chan domain.SubmissionClaim, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, values := range [][3]string{{"worker-a", "token-a", "1-0"}, {"worker-b", "token-b", "2-0"}} {
		wg.Add(1)
		go func(i int, values [3]string) {
			defer wg.Done()
			claim, err := st.ClaimSubmission(ctx, sub.ID, values[0], values[1], values[2], now, 30*time.Second)
			results <- claim
			errs <- err
		}(i, values)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	acquired := 0
	active := 0
	for result := range results {
		switch result.State {
		case domain.ClaimAcquired:
			acquired++
		case domain.ClaimActiveOtherReceipt:
			active++
		}
	}
	if acquired != 1 || active != 1 {
		t.Fatalf("acquired=%d active=%d, want 1 and 1", acquired, active)
	}
}

func TestPostgresExpiredLeaseRejectsStaleCompletion(t *testing.T) {
	st := integrationStore(t)
	ctx := context.Background()
	sub, _ := st.CreateSubmission(ctx, domain.Submission{ProblemID: "sum", Language: domain.LanguageGo, Code: "code"})
	now := time.Now().UTC()
	_, _ = st.ClaimSubmission(ctx, sub.ID, "worker-a", "token-a", "1-0", now, time.Second)
	replacement, err := st.ClaimSubmission(ctx, sub.ID, "worker-b", "token-b", "1-0", now.Add(2*time.Second), 30*time.Second)
	if err != nil || replacement.State != domain.ClaimAcquired {
		t.Fatalf("replacement = %+v, %v", replacement, err)
	}
	if ok, err := st.CompleteSubmission(ctx, sub.ID, "token-a", now.Add(3*time.Second), domain.JudgeResult{Status: domain.StatusAccepted}); err != nil || ok {
		t.Fatalf("stale completion = %v, %v", ok, err)
	}
	if ok, err := st.CompleteSubmission(ctx, sub.ID, "token-b", now.Add(3*time.Second), domain.JudgeResult{Status: domain.StatusAccepted}); err != nil || !ok {
		t.Fatalf("active completion = %v, %v", ok, err)
	}
}
