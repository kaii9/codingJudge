package platform

import (
	"context"
	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/judgerpc"
	"github.com/kaii9/codingJudge/internal/store"
	"net"
	"testing"
	"time"
)

func TestAdapterUsesJudgeDomainInsteadOfLocalSubmissionTables(t *testing.T) {
	ctx := context.Background()
	platform := store.NewMemoryStore([]domain.Problem{{ID: "sum", TimeLimitMS: 1000, MemoryLimitMB: 64, TestCases: []domain.TestCase{{ExpectedOutput: "3"}}}})
	judge := store.NewMemoryStore(nil)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := judgerpc.NewServer(judge, nil, "secret")
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	defer listener.Close()
	client, err := judgerpc.NewClient(listener.Addr().String(), "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	adapter := NewStore(platform, client)
	sub := domain.Submission{UserID: "alice", ProblemID: "sum", Language: domain.LanguagePython, Code: "print(3)"}
	created, _, err := adapter.CreateSubmissionIdempotent(ctx, sub, "key", judgerpc.Fingerprint(sub))
	if err != nil {
		t.Fatal(err)
	}
	local, err := platform.ListSubmissions(ctx)
	if err != nil || len(local) != 0 {
		t.Fatalf("platform wrote submission: %+v %v", local, err)
	}
	own, ok, err := adapter.GetSubmissionForUser(ctx, created.ID, "alice")
	if err != nil || !ok || own.ID != created.ID {
		t.Fatalf("own submission: %+v %v", own, err)
	}
	if _, ok, _ := adapter.GetSubmissionForUser(ctx, created.ID, "bob"); ok {
		t.Fatal("cross user submission exposed")
	}
}
