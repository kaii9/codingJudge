package judgerpc

import (
	"context"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/store"
	"net"
	"testing"
	"time"
)

func TestRPCSubmissionOwnershipAndSnapshotReplay(t *testing.T) {
	st := store.NewMemoryStore(nil)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(st, nil, "secret")
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	defer listener.Close()
	client, err := NewClient(listener.Addr().String(), "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	problem := domain.Problem{ID: "sum", TimeLimitMS: 1000, MemoryLimitMB: 64, TestCases: []domain.TestCase{{Input: "1 2", ExpectedOutput: "3"}}}
	ctx := context.Background()
	sub := domain.Submission{UserID: "alice", ProblemID: "sum", Language: domain.LanguagePython, Code: "print(3)"}
	created, replay, err := client.Create(ctx, sub, "key", Fingerprint(sub), problem)
	if err != nil || replay {
		t.Fatalf("create: %+v %v", created, err)
	}
	problem.TestCases[0].ExpectedOutput = "new"
	repeated, replay, err := client.Create(ctx, sub, "key", Fingerprint(sub), problem)
	if err != nil || !replay || repeated.ID != created.ID {
		t.Fatalf("replay: %+v %v", repeated, err)
	}
	snapshot, ok, err := st.GetSubmissionProblem(ctx, created.ID)
	if err != nil || !ok || snapshot.TestCases[0].ExpectedOutput != "3" {
		t.Fatalf("snapshot: %+v %v", snapshot, err)
	}
	if _, ok, err := client.GetSubmissionForUser(ctx, created.ID, "bob"); err != nil || ok {
		t.Fatalf("cross-user read: %v %v", ok, err)
	}
	own, ok, err := client.GetSubmissionForUser(ctx, created.ID, "alice")
	if err != nil || !ok || own.ID != created.ID {
		t.Fatalf("own: %+v %v", own, err)
	}
	bad, err := NewClient(listener.Addr().String(), "bad-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	if _, _, err := bad.GetSubmissionForUser(ctx, created.ID, "alice"); err == nil {
		t.Fatal("invalid service identity accepted")
	}
}
