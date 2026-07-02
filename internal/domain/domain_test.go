package domain_test

import (
	"encoding/json"
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

func TestProblemMetadataSerializesExactValues(t *testing.T) {
	t.Parallel()
	problem := domain.Problem{
		ID: "target-pair", Difficulty: domain.DifficultyMedium,
		Collection: domain.CollectionHot20, SortOrder: 1,
		Tags: []string{"array", "hash-table"},
	}
	data, err := json.Marshal(problem)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["difficulty"] != "medium" || got["collection"] != "hot20" || got["sortOrder"] != float64(1) {
		t.Fatalf("metadata = %s", data)
	}
}
