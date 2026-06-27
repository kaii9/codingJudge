package judge

import (
	"context"
	"errors"
	"strings"

	"github.com/kai/codingjudge/internal/domain"
)

type RunRequest struct {
	Language      domain.Language
	Code          string
	Input         string
	TimeLimitMS   int
	MemoryLimitMB int
}

type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration int64
	TimedOut bool
}

type Runner interface {
	Run(context.Context, RunRequest) (RunResult, error)
}

type BatchRunner interface {
	RunBatch(context.Context, RunRequest, []string) ([]RunResult, error)
}

type Service struct {
	runner Runner
}

func NewService(runner Runner) *Service {
	return &Service{runner: runner}
}

func (s *Service) Evaluate(ctx context.Context, problem domain.Problem, language domain.Language, code string) domain.JudgeResult {
	if len(problem.TestCases) == 0 {
		return domain.JudgeResult{Status: domain.StatusAccepted}
	}
	baseRequest := RunRequest{
		Language:      language,
		Code:          code,
		TimeLimitMS:   problem.TimeLimitMS,
		MemoryLimitMB: problem.MemoryLimitMB,
	}
	if runner, ok := s.runner.(BatchRunner); ok {
		inputs := make([]string, len(problem.TestCases))
		for i, tc := range problem.TestCases {
			inputs[i] = tc.Input
		}
		runs, err := runner.RunBatch(ctx, baseRequest, inputs)
		if err != nil {
			return judgeErrorResult(err)
		}
		for i, run := range runs {
			if i >= len(problem.TestCases) {
				break
			}
			if result, finished := judgeRun(problem.TestCases[i], run); finished {
				return result
			}
		}
		if len(runs) != len(problem.TestCases) {
			return domain.JudgeResult{Status: domain.StatusInternalError, Stderr: "runner returned incomplete results"}
		}
		return domain.JudgeResult{Status: domain.StatusAccepted}
	}

	for _, tc := range problem.TestCases {
		request := baseRequest
		request.Input = tc.Input
		run, err := s.runner.Run(ctx, request)
		if err != nil {
			return judgeErrorResult(err)
		}
		if result, finished := judgeRun(tc, run); finished {
			return result
		}
	}
	return domain.JudgeResult{Status: domain.StatusAccepted}
}

func judgeErrorResult(err error) domain.JudgeResult {
	status := domain.StatusRuntimeError
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		status = domain.StatusTimeLimitExceeded
	}
	return domain.JudgeResult{Status: status, Stderr: err.Error()}
}

func judgeRun(tc domain.TestCase, run RunResult) (domain.JudgeResult, bool) {
	result := domain.JudgeResult{
		Stdout:   run.Stdout,
		Stderr:   run.Stderr,
		ExitCode: run.ExitCode,
		Duration: run.Duration,
	}
	if run.TimedOut {
		result.Status = domain.StatusTimeLimitExceeded
		return result, true
	}
	if run.ExitCode != 0 {
		result.Status = domain.StatusRuntimeError
		return result, true
	}
	if normalizeOutput(run.Stdout) != normalizeOutput(tc.ExpectedOutput) {
		result.Status = domain.StatusWrongAnswer
		return result, true
	}
	return domain.JudgeResult{}, false
}

func normalizeOutput(output string) string {
	return strings.TrimRight(output, " \n\r\t")
}
