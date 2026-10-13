package store_test

import (
	"context"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/store"
	"testing"
)

func TestSnapshotReplayKeepsFirstProblem(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore(nil)
	old := domain.Problem{ID: "sum", TimeLimitMS: 1000, MemoryLimitMB: 64, TestCases: []domain.TestCase{{Input: "1 2", ExpectedOutput: "3"}}}
	sub := domain.Submission{UserID: "user-1", ProblemID: "sum", Language: domain.LanguagePython, Code: "print(3)"}
	first, replayed, err := st.CreateSubmissionWithSnapshot(ctx, sub, "key", "hash", old)
	if err != nil || replayed {
		t.Fatalf("first: %v %v", err, replayed)
	}
	newer := old
	newer.TestCases = []domain.TestCase{{ExpectedOutput: "changed"}}
	second, replayed, err := st.CreateSubmissionWithSnapshot(ctx, sub, "key", "hash", newer)
	if err != nil || !replayed || first.ID != second.ID {
		t.Fatalf("replay: %+v %v %v", second, replayed, err)
	}
	snapshot, ok, err := st.GetSubmissionProblem(ctx, first.ID)
	if err != nil || !ok || snapshot.TestCases[0].ExpectedOutput != "3" {
		t.Fatalf("snapshot: %+v %v", snapshot, err)
	}
	snapshot.TestCases[0].ExpectedOutput = "mutated"
	again, _, _ := st.GetSubmissionProblem(ctx, first.ID)
	if again.TestCases[0].ExpectedOutput != "3" {
		t.Fatal("snapshot mutated through returned value")
	}
}
