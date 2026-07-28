package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSelectedProblemsUsesExplicitCSV(t *testing.T) {
	t.Parallel()

	problems, err := selectedProblems("testdata/cases", "sum, target-pair", false)
	if err != nil {
		t.Fatalf("selectedProblems returned error: %v", err)
	}
	want := []string{"sum", "target-pair"}
	if !reflect.DeepEqual(problems, want) {
		t.Fatalf("problems = %#v, want %#v", problems, want)
	}
}

func TestSelectedProblemsRejectsMissingSelection(t *testing.T) {
	t.Parallel()

	if _, err := selectedProblems("testdata/cases", "", false); err == nil {
		t.Fatal("selectedProblems should require -problems or -all")
	}
}

func TestSelectedProblemsDiscoversAllProblemDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, problemID := range []string{"sum", "target-pair"} {
		problemDir := filepath.Join(root, problemID)
		if err := os.MkdirAll(problemDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(problemDir, "001.in"), []byte("input\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	problems, err := selectedProblems(root, "", true)
	if err != nil {
		t.Fatalf("selectedProblems returned error: %v", err)
	}
	want := []string{"sum", "target-pair"}
	if !reflect.DeepEqual(problems, want) {
		t.Fatalf("problems = %#v, want %#v", problems, want)
	}
}
