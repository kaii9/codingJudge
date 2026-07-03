package judge

import (
	"context"
	"strings"
	"time"

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

// Metrics records test case evaluation outcomes.
type Metrics interface {
	ObserveJudgeCase(language, result string, duration time.Duration)
}

// Option configures a judge Service.
type Option func(*Service)

// WithMetrics sets the judge case metrics recorder.
func WithMetrics(m Metrics) Option {
	return func(s *Service) {
		s.metrics = m
	}
}

type Service struct {
	runner  Runner
	metrics Metrics
}

func NewService(runner Runner, options ...Option) *Service {
	s := &Service{runner: runner}
	for _, opt := range options {
		opt(s)
	}
	return s
}

func (s *Service) Evaluate(ctx context.Context, problem domain.Problem, language domain.Language, code string) (domain.JudgeResult, error) {
	if len(problem.TestCases) == 0 {
		return domain.JudgeResult{Status: domain.StatusAccepted}, nil
	}
	langStr := string(language)
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
			return domain.JudgeResult{}, err
		}
		for i, run := range runs {
			if i >= len(problem.TestCases) {
				break
			}
			if result, finished := judgeRun(problem.TestCases[i], run); finished {
				s.recordCase(langStr, result.Status, run)
				return result, nil
			}
			s.recordCase(langStr, domain.StatusAccepted, run)
		}
		if len(runs) != len(problem.TestCases) {
			return domain.JudgeResult{Status: domain.StatusInternalError, Stderr: "runner returned incomplete results"}, nil
		}
		return domain.JudgeResult{Status: domain.StatusAccepted}, nil
	}

	for _, tc := range problem.TestCases {
		request := baseRequest
		request.Input = tc.Input
		run, err := s.runner.Run(ctx, request)
		if err != nil {
			// 基础设施错误不记录用例指标
			return domain.JudgeResult{}, err
		}
		if result, finished := judgeRun(tc, run); finished {
			s.recordCase(langStr, result.Status, run)
			return result, nil
		}
		s.recordCase(langStr, domain.StatusAccepted, run)
	}
	return domain.JudgeResult{Status: domain.StatusAccepted}, nil
}

func (s *Service) recordCase(language string, status domain.SubmissionStatus, run RunResult) {
	if s.metrics == nil {
		return
	}
	duration := time.Duration(run.Duration) * time.Millisecond
	s.metrics.ObserveJudgeCase(language, string(status), duration)
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
