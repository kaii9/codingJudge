package domain

import "time"

type CompletionEvent struct {
	ID           string           `json:"eventId"`
	SubmissionID string           `json:"submissionId"`
	UserID       string           `json:"userId"`
	ProblemID    string           `json:"problemId"`
	Status       SubmissionStatus `json:"status"`
	Version      int              `json:"version"`
	FinishedAt   time.Time        `json:"finishedAt"`
}
