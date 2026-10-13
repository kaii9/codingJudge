package executor

import (
	"context"
	legacy "github.com/kaii9/codingJudge/api/compat/executor"
	pb "github.com/kaii9/codingJudge/api/gen/executor/v1"
	"github.com/kaii9/codingJudge/internal/judge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"testing"
	"time"
)

func TestFrozenV1ClientWorksAgainstCurrentExecutor(t *testing.T) {
	_, _, addr := grpcFixture(t, runnerFunc(func(context.Context, judge.RunRequest, []string) ([]judge.RunResult, error) {
		return []judge.RunResult{{Stdout: "3", Stage: judge.StageRun}}, nil
	}))
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var response legacy.Response
	err = conn.Invoke(authorized(ctx), pb.ExecutorService_RunBatch_FullMethodName, &legacy.Request{Language: "python", Code: "print(3)", Inputs: []string{""}, TimeLimitMs: 1000, MemoryLimitMb: 64}, &response)
	if err != nil || len(response.Results) != 1 || response.Results[0].Stdout != "3" {
		t.Fatalf("frozen client: %+v %v", response.Results, err)
	}
}
