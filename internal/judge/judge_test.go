package judge_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/judge"
)

type fakeRunner struct {
	results []judge.RunResult
	err     error
}

func (f *fakeRunner) Run(ctx context.Context, req judge.RunRequest) (judge.RunResult, error) {
	if f.err != nil {
		return judge.RunResult{}, f.err
	}
	if len(f.results) == 0 {
		return judge.RunResult{}, nil
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result, nil
}

type fakeBatchRunner struct {
	results         []judge.RunResult
	batchCalls      int
	individualCalls int
	inputs          []string
}

func (f *fakeBatchRunner) Run(context.Context, judge.RunRequest) (judge.RunResult, error) {
	f.individualCalls++
	return judge.RunResult{}, nil
}

func (f *fakeBatchRunner) RunBatch(_ context.Context, _ judge.RunRequest, inputs []string) ([]judge.RunResult, error) {
	f.batchCalls++
	f.inputs = append([]string(nil), inputs...)
	return f.results, nil
}

func TestServiceAcceptsWhenAllCasesMatch(t *testing.T) {
	t.Parallel()

	service := judge.NewService(&fakeRunner{results: []judge.RunResult{
		{Stdout: "3\n", ExitCode: 0},
		{Stdout: "7\n", ExitCode: 0},
	}})

	result, err := service.Evaluate(context.Background(), domain.Problem{
		ID: "sum",
		TestCases: []domain.TestCase{
			{Input: "1 2\n", ExpectedOutput: "3\n"},
			{Input: "3 4\n", ExpectedOutput: "7\n"},
		},
	}, domain.LanguageGo, "code")
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != domain.StatusAccepted {
		t.Fatalf("status = %q, want %q", result.Status, domain.StatusAccepted)
	}
}

func TestServiceUsesBatchRunnerOnceForAllTestCases(t *testing.T) {
	t.Parallel()

	runner := &fakeBatchRunner{results: []judge.RunResult{
		{Stdout: "3\n"},
		{Stdout: "7\n"},
	}}
	service := judge.NewService(runner)
	result, err := service.Evaluate(context.Background(), domain.Problem{
		ID: "sum",
		TestCases: []domain.TestCase{
			{Input: "1 2\n", ExpectedOutput: "3\n"},
			{Input: "3 4\n", ExpectedOutput: "7\n"},
		},
	}, domain.LanguageGo, "code")
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != domain.StatusAccepted {
		t.Fatalf("status = %q, want %q", result.Status, domain.StatusAccepted)
	}
	if runner.batchCalls != 1 || runner.individualCalls != 0 {
		t.Fatalf("runner calls: batch=%d individual=%d, want 1 and 0", runner.batchCalls, runner.individualCalls)
	}
	if len(runner.inputs) != 2 || runner.inputs[0] != "1 2\n" || runner.inputs[1] != "3 4\n" {
		t.Fatalf("batch inputs = %#v", runner.inputs)
	}
}

func TestServiceReportsWrongAnswerOnFirstMismatch(t *testing.T) {
	t.Parallel()

	service := judge.NewService(&fakeRunner{results: []judge.RunResult{
		{Stdout: "4\n", ExitCode: 0},
	}})

	result, err := service.Evaluate(context.Background(), domain.Problem{
		ID: "sum",
		TestCases: []domain.TestCase{
			{Input: "1 2\n", ExpectedOutput: "3\n"},
		},
	}, domain.LanguageGo, "code")
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != domain.StatusWrongAnswer {
		t.Fatalf("status = %q, want %q", result.Status, domain.StatusWrongAnswer)
	}
}

func TestServiceReportsTimeLimitExceeded(t *testing.T) {
	t.Parallel()

	service := judge.NewService(&fakeRunner{results: []judge.RunResult{
		{TimedOut: true, Stderr: "timeout"},
	}})

	result, err := service.Evaluate(context.Background(), domain.Problem{
		ID: "loop",
		TestCases: []domain.TestCase{
			{Input: "", ExpectedOutput: ""},
		},
	}, domain.LanguageGo, "code")
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != domain.StatusTimeLimitExceeded {
		t.Fatalf("status = %q, want %q", result.Status, domain.StatusTimeLimitExceeded)
	}
}

func TestEvaluateReturnsRunnerInfrastructureError(t *testing.T) {
	t.Parallel()
	want := errors.New("docker daemon unavailable")
	service := judge.NewService(&fakeRunner{err: want})
	result, err := service.Evaluate(context.Background(), domain.Problem{
		ID: "sum", TestCases: []domain.TestCase{{Input: "1 2\n", ExpectedOutput: "3\n"}},
	}, domain.LanguageGo, "code")
	if !errors.Is(err, want) || result.Status != "" {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
}
