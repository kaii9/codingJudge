package caseassets

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaii9/codingJudge/internal/domain"
)

type fakeObjectWriter struct {
	puts map[string][]byte
}

func (w *fakeObjectWriter) Put(ctx context.Context, key string, data []byte, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.puts[key] = append([]byte(nil), data...)
	return nil
}

type fakeCaseRepository struct {
	problemID string
	cases     []domain.TestCase
}

func (r *fakeCaseRepository) ReplaceProblemTestCases(ctx context.Context, problemID string, cases []domain.TestCase) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.problemID = problemID
	r.cases = append([]domain.TestCase(nil), cases...)
	return nil
}

func TestUploaderUploadsPairedCasesAndStoresObjectMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	problemDir := filepath.Join(root, "sum")
	if err := os.MkdirAll(problemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(problemDir, "001.in"), []byte("1 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(problemDir, "001.out"), []byte("3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	objects := &fakeObjectWriter{puts: make(map[string][]byte)}
	repo := &fakeCaseRepository{}

	report, err := NewUploader(repo, objects).UploadProblem(context.Background(), root, "sum")
	if err != nil {
		t.Fatalf("UploadProblem returned error: %v", err)
	}

	if report.ProblemID != "sum" || report.CaseCount != 1 || repo.problemID != "sum" {
		t.Fatalf("report=%+v repo.problemID=%q", report, repo.problemID)
	}
	if len(repo.cases) != 1 {
		t.Fatalf("stored cases = %+v, want 1 case", repo.cases)
	}
	tc := repo.cases[0]
	if tc.Input != "" || tc.ExpectedOutput != "" {
		t.Fatalf("object-backed case should not store plaintext input/output: %+v", tc)
	}
	if !tc.Hidden || tc.InputObjectKey == "" || tc.ExpectedOutputObjectKey == "" || tc.InputSHA256 == "" || tc.ExpectedOutputSHA256 == "" {
		t.Fatalf("object metadata incomplete: %+v", tc)
	}
	if tc.InputSizeBytes != 4 || tc.ExpectedOutputSizeBytes != 2 {
		t.Fatalf("sizes = input %d output %d", tc.InputSizeBytes, tc.ExpectedOutputSizeBytes)
	}
	if string(objects.puts[tc.InputObjectKey]) != "1 2\n" || string(objects.puts[tc.ExpectedOutputObjectKey]) != "3\n" {
		t.Fatalf("uploaded objects = %#v", objects.puts)
	}
}

func TestUploaderRejectsMissingOutputPair(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	problemDir := filepath.Join(root, "sum")
	if err := os.MkdirAll(problemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(problemDir, "001.in"), []byte("1 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewUploader(&fakeCaseRepository{}, &fakeObjectWriter{puts: make(map[string][]byte)}).UploadProblem(context.Background(), root, "sum")
	if err == nil {
		t.Fatal("UploadProblem should reject missing .out pair")
	}
}

func TestDiscoverProblemsReturnsSortedProblemDirectoriesWithCases(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, problemID := range []string{"target-pair", "balanced-delimiters"} {
		problemDir := filepath.Join(root, problemID)
		if err := os.MkdirAll(problemDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(problemDir, "001.in"), []byte("input\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("ignore me"), 0o600); err != nil {
		t.Fatal(err)
	}
	emptyDir := filepath.Join(root, "empty-problem")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	problems, err := DiscoverProblems(root)
	if err != nil {
		t.Fatalf("DiscoverProblems returned error: %v", err)
	}
	want := []string{"balanced-delimiters", "target-pair"}
	if len(problems) != len(want) {
		t.Fatalf("problems = %#v, want %#v", problems, want)
	}
	for i := range want {
		if problems[i] != want[i] {
			t.Fatalf("problems = %#v, want %#v", problems, want)
		}
	}
}
