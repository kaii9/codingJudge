package domain

import "time"

type Language string

type ProblemDifficulty string
type ProblemCollection string

const (
	DifficultyEasy    ProblemDifficulty = "easy"
	DifficultyMedium  ProblemDifficulty = "medium"
	DifficultyHard    ProblemDifficulty = "hard"
	CollectionStarter ProblemCollection = "starter"
	CollectionHot20   ProblemCollection = "hot20"
)

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
	StatusCompileError      SubmissionStatus = "compile_error"
	StatusRuntimeError      SubmissionStatus = "runtime_error"
	StatusTimeLimitExceeded SubmissionStatus = "time_limit_exceeded"
	StatusInternalError     SubmissionStatus = "internal_error"
)

func IsTerminalSubmissionStatus(status SubmissionStatus) bool {
	switch status {
	case StatusAccepted,
		StatusWrongAnswer,
		StatusCompileError,
		StatusRuntimeError,
		StatusTimeLimitExceeded,
		StatusInternalError:
		return true
	default:
		return false
	}
}

type ClaimState string

const (
	ClaimAcquired           ClaimState = "acquired"
	ClaimTerminal           ClaimState = "terminal"
	ClaimActiveSameReceipt  ClaimState = "active_same_receipt"
	ClaimActiveOtherReceipt ClaimState = "active_other_receipt"
	ClaimMissing            ClaimState = "missing"
)

type Problem struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Language      Language          `json:"language"`
	TimeLimitMS   int               `json:"timeLimitMs"`
	MemoryLimitMB int               `json:"memoryLimitMb"`
	Difficulty    ProblemDifficulty `json:"difficulty"`
	Collection    ProblemCollection `json:"collection"`
	SortOrder     int               `json:"sortOrder"`
	Tags          []string          `json:"tags"`
	TestCases     []TestCase        `json:"testCases,omitempty"`
}

type TestCase struct {
	ID                      int64
	Input                   string
	ExpectedOutput          string
	InputObjectKey          string
	ExpectedOutputObjectKey string
	InputSHA256             string
	ExpectedOutputSHA256    string
	InputSizeBytes          int64
	ExpectedOutputSizeBytes int64
	Hidden                  bool
}

type Submission struct {
	ID        string           `json:"id"`
	UserID    string           `json:"userId,omitempty"`
	ProblemID string           `json:"problemId"`
	Language  Language         `json:"language"`
	Code      string           `json:"code,omitempty"`
	Status    SubmissionStatus `json:"status"`
	Result    *JudgeResult     `json:"result,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
}

type Session struct {
	TokenHash string    `json:"-"`
	UserID    string    `json:"userId"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

type LeaderboardEntry struct {
	Rank                int       `json:"rank"`
	UserID              string    `json:"userId"`
	Username            string    `json:"username"`
	Solved              int       `json:"solved"`
	AcceptedSubmissions int       `json:"acceptedSubmissions"`
	LastAcceptedAt      time.Time `json:"lastAcceptedAt"`
}

type JudgeResult struct {
	Status   SubmissionStatus `json:"status"`
	Stdout   string           `json:"stdout,omitempty"`
	Stderr   string           `json:"stderr,omitempty"`
	ExitCode int              `json:"exitCode,omitempty"`
	Duration int64            `json:"durationMs,omitempty"`
}

type ArtifactKind string

const (
	ArtifactKindSource ArtifactKind = "source"
	ArtifactKindStdout ArtifactKind = "stdout"
	ArtifactKindStderr ArtifactKind = "stderr"
)

type SubmissionArtifact struct {
	ID           int64        `json:"id"`
	SubmissionID string       `json:"submissionId"`
	Attempt      int          `json:"attempt"`
	Token        string       `json:"-"`
	Kind         ArtifactKind `json:"kind"`
	ObjectKey    string       `json:"-"`
	SHA256       string       `json:"sha256"`
	SizeBytes    int64        `json:"sizeBytes"`
	CreatedAt    time.Time    `json:"createdAt"`
}

type Job struct {
	SubmissionID string `json:"submissionId"`
	OutboxID     int64  `json:"-"`
	Attempts     int    `json:"-"`
	Receipt      string `json:"-"`
}

type OutboxEvent struct {
	ID              int64
	SubmissionID    string
	ClaimToken      string
	PublishAttempts int
}

type SubmissionClaim struct {
	State          ClaimState
	Submission     Submission
	Token          string
	WorkerID       string
	Receipt        string
	ActiveReceipt  string
	LeaseExpiresAt time.Time
	Attempts       int
	LeaseTakeover  bool // 本次认领是在前一个租约过期后的接管
}
