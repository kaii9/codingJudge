package domain

import "time"

type Language string

const (
	LanguageGo     Language = "go"
	LanguageCPP    Language = "cpp"
	LanguagePython Language = "python"
)

func IsSupportedLanguage(language Language) bool {
	switch language {
	case LanguageGo, LanguageCPP, LanguagePython:
		return true
	default:
		return false
	}
}

type SubmissionStatus string

const (
	StatusQueued            SubmissionStatus = "queued"
	StatusRunning           SubmissionStatus = "running"
	StatusAccepted          SubmissionStatus = "accepted"
	StatusWrongAnswer       SubmissionStatus = "wrong_answer"
	StatusRuntimeError      SubmissionStatus = "runtime_error"
	StatusTimeLimitExceeded SubmissionStatus = "time_limit_exceeded"
	StatusInternalError     SubmissionStatus = "internal_error"
)

type Problem struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Language      Language   `json:"language"`
	TimeLimitMS   int        `json:"timeLimitMs"`
	MemoryLimitMB int        `json:"memoryLimitMb"`
	TestCases     []TestCase `json:"testCases,omitempty"`
}

type TestCase struct {
	Input          string
	ExpectedOutput string
}

type Submission struct {
	ID        string           `json:"id"`
	ProblemID string           `json:"problemId"`
	Language  Language         `json:"language"`
	Code      string           `json:"code,omitempty"`
	Status    SubmissionStatus `json:"status"`
	Result    *JudgeResult     `json:"result,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

type JudgeResult struct {
	Status   SubmissionStatus `json:"status"`
	Stdout   string           `json:"stdout,omitempty"`
	Stderr   string           `json:"stderr,omitempty"`
	ExitCode int              `json:"exitCode,omitempty"`
	Duration int64            `json:"durationMs,omitempty"`
}

type Job struct {
	SubmissionID string `json:"submissionId"`
	Attempts     int    `json:"-"`
	Receipt      string `json:"-"`
}
