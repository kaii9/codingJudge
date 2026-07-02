package domain_test

import (
	"testing"

	"github.com/kai/codingjudge/internal/domain"
)

func TestIsTerminalSubmissionStatus(t *testing.T) {
	t.Parallel()

	for _, status := range []domain.SubmissionStatus{
		domain.StatusAccepted,
		domain.StatusWrongAnswer,
		domain.StatusRuntimeError,
		domain.StatusTimeLimitExceeded,
		domain.StatusInternalError,
	} {
		if !domain.IsTerminalSubmissionStatus(status) {
			t.Fatalf("%q should be terminal", status)
		}
	}
	if domain.IsTerminalSubmissionStatus(domain.StatusRunning) {
		t.Fatal("running must not be terminal")
	}
}
