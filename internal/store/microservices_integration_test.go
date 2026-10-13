//go:build integration

package store

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kaii9/codingJudge/internal/domain"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func domainIntegrationStore(t *testing.T, kind string) *PostgresStore {
	t.Helper()
	dsn := os.Getenv("TEST_" + strings.ToUpper(kind) + "_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated domain database not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	st, err := NewPostgresStore(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	st.judgeDomain = kind == "judge"
	t.Cleanup(st.Close)
	paths, err := filepath.Glob("../../migrations/" + kind + "/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	return st
}
func TestMicroserviceSnapshotAndResultOutboxAtomic(t *testing.T) {
	st := domainIntegrationStore(t, "judge")
	ctx := context.Background()
	problem := domain.Problem{ID: "sum", TimeLimitMS: 1000, MemoryLimitMB: 64, TestCases: []domain.TestCase{{Input: "1 2", ExpectedOutput: "3"}}}
	sub := domain.Submission{UserID: "external-user", ProblemID: "sum", Language: domain.LanguagePython, Code: "print(3)"}
	created, _, err := st.CreateSubmissionWithSnapshot(ctx, sub, "key", strings.Repeat("a", 64), problem)
	if err != nil {
		t.Fatal(err)
	}
	problem.TestCases[0].ExpectedOutput = "new"
	_, replay, err := st.CreateSubmissionWithSnapshot(ctx, sub, "key", strings.Repeat("a", 64), problem)
	if err != nil || !replay {
		t.Fatalf("replay=%v err=%v", replay, err)
	}
	snapshot, ok, err := st.GetSubmissionProblem(ctx, created.ID)
	if err != nil || !ok || snapshot.TestCases[0].ExpectedOutput != "3" {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	now := time.Now().UTC()
	claim, err := st.ClaimSubmission(ctx, created.ID, "worker", "token", "1-0", now, time.Minute)
	if err != nil || claim.State != domain.ClaimAcquired {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	_, err = st.pool.Exec(ctx, `CREATE FUNCTION reject_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected event failure'; END $$;
 CREATE TRIGGER reject_event BEFORE INSERT ON completion_outbox FOR EACH ROW EXECUTE FUNCTION reject_event();`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CompleteSubmission(ctx, created.ID, "token", now.Add(time.Second), domain.JudgeResult{Status: domain.StatusAccepted}); err == nil {
		t.Fatal("injected failure ignored")
	}
	after, _, err := st.GetSubmission(ctx, created.ID)
	if err != nil || after.Status != domain.StatusRunning {
		t.Fatalf("partial result commit: %+v %v", after, err)
	}
	if _, err := st.pool.Exec(ctx, "DROP TRIGGER reject_event ON completion_outbox"); err != nil {
		t.Fatal(err)
	}
	done, err := st.CompleteSubmission(ctx, created.ID, "token", now.Add(2*time.Second), domain.JudgeResult{Status: domain.StatusAccepted})
	if err != nil || !done {
		t.Fatalf("complete=%v err=%v", done, err)
	}
	var count int
	if err := st.pool.QueryRow(ctx, "SELECT count(*) FROM completion_outbox WHERE submission_id=$1", created.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("events=%d err=%v", count, err)
	}
	if done, err := st.CompleteSubmission(ctx, created.ID, "old-token", now.Add(3*time.Second), domain.JudgeResult{Status: domain.StatusWrongAnswer}); err != nil || done {
		t.Fatalf("stale completion accepted: %v %v", done, err)
	}
}
func TestMicroserviceProjectionDuplicateAndLateEvent(t *testing.T) {
	st := domainIntegrationStore(t, "platform")
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "projection_user", "hash")
	if err != nil {
		t.Fatal(err)
	}
	first := domain.CompletionEvent{ID: "first", SubmissionID: "sub-1", UserID: user.ID, ProblemID: "sum", Status: domain.StatusAccepted, Version: 2, FinishedAt: time.Now().UTC()}
	second := first
	second.ID = "second"
	second.SubmissionID = "sub-2"
	old := first
	old.ID = "old"
	old.Version = 1
	old.Status = domain.StatusWrongAnswer
	for _, event := range []domain.CompletionEvent{first, second, first, old} {
		if err := st.ApplyCompletionEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := st.ListProjectedLeaderboard(ctx, 50)
	if err != nil || len(rows) != 1 || rows[0].Solved != 1 || rows[0].AcceptedSubmissions != 2 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}
func TestMicroserviceDatabaseCredentialsCannotCrossDomains(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, pair := range [][2]string{{"TEST_PLATFORM_DATABASE_URL", "cj_judge"}, {"TEST_JUDGE_DATABASE_URL", "cj_platform"}} {
		dsn := os.Getenv(pair[0])
		if dsn == "" {
			t.Skip("dedicated credentials not configured")
		}
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		parsed.Path = "/" + pair[1]
		pool, err := pgxpool.New(ctx, parsed.String())
		if err != nil {
			t.Fatal(err)
		}
		err = pool.Ping(ctx)
		pool.Close()
		if err == nil {
			t.Fatalf("%s can connect to foreign domain %s", pair[0], pair[1])
		}
	}
}
