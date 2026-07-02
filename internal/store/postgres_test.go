package store

import (
	"testing"
	"time"

	"github.com/kai/codingjudge/internal/domain"
)

func TestScanSubmissionIncludesCodeForWorker(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	row := fakeSubmissionRow{values: []any{
		"sub-1",
		"sum",
		domain.LanguageGo,
		"package main",
		domain.StatusQueued,
		(*string)(nil),
		(*string)(nil),
		(*int)(nil),
		(*int64)(nil),
		now,
		now,
	}}

	sub, err := scanSubmission(row)
	if err != nil {
		t.Fatalf("scanSubmission returned error: %v", err)
	}
	if sub.Code != "package main" {
		t.Fatalf("Code = %q, want package main", sub.Code)
	}
}

type fakeSubmissionRow struct {
	values []any
}

func (r fakeSubmissionRow) Scan(dest ...any) error {
	for i := range dest {
		switch d := dest[i].(type) {
		case *string:
			*d = r.values[i].(string)
		case *domain.Language:
			*d = r.values[i].(domain.Language)
		case *domain.SubmissionStatus:
			*d = r.values[i].(domain.SubmissionStatus)
		case **string:
			*d, _ = r.values[i].(*string)
		case **int:
			*d, _ = r.values[i].(*int)
		case **int64:
			*d, _ = r.values[i].(*int64)
		case *time.Time:
			*d = r.values[i].(time.Time)
		}
	}
	return nil
}
