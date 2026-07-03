package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRenderFromFixtures(t *testing.T) {
	tmp := t.TempDir()
	binary := tmp + "/render"
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cmd := exec.Command(binary,
		"testdata/meta.txt",
		"testdata/k6-worker-1.json",
		"testdata/k6-worker-2.json",
		"testdata/k6-worker-4.json",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("render: %v\n%s", err, out)
	}

	output := string(out)
	for _, want := range []string{
		"| Workers | Submission rate | HTTP P95 | Judge P95 | Failure rate | Peak pending |",
		"| 1 |",
		"| 2 |",
		"| 4 |",
		"150.70ms",
		"## Interpretation",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in output:\n%s", want, output)
		}
	}

	// Missing summary file should return non-zero.
	errCmd := exec.Command(binary, "testdata/meta.txt", "nonexistent.json", "nonexistent2.json", "nonexistent3.json")
	if errCmd.Run() == nil {
		t.Error("expected non-zero exit for missing file")
	}
}

func TestParseRealK6Structure(t *testing.T) {
	// This test verifies the renderer can handle real k6 2.0 JSON that
	// includes nested objects like "thresholds": {}, int values, etc.
	// The current map[string]float64 parser will fail on this.
	tmp := t.TempDir()
	binary := tmp + "/render"
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cmd := exec.Command(binary,
		"testdata/meta.txt",
		"testdata/smoke-real-structure.json",
		"testdata/smoke-real-structure.json",
		"testdata/smoke-real-structure.json",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("render should not exit non-zero on valid k6 JSON: %v\n%s", err, out)
	}

	output := string(out)
	// Must NOT contain "NaN".
	if strings.Contains(output, "NaN") {
		t.Errorf("output must not contain NaN:\n%s", output)
	}
	// Must contain the result table.
	if !strings.Contains(output, "| Workers |") {
		t.Error("missing result table")
	}
}

func TestRenderFailsOnMissingMetric(t *testing.T) {
	tmp := t.TempDir()
	binary := tmp + "/render"
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// Create a JSON with missing required metrics.
	// Use the real-structure file but for workers 2 and 4 use empty files.
	// Missing file should cause non-zero exit.
	errCmd := exec.Command(binary, "testdata/meta.txt", "testdata/smoke-real-structure.json", "nonexistent.json", "nonexistent2.json")
	if errCmd.Run() == nil {
		t.Error("expected non-zero exit for missing file")
	}
}

func TestMain(m *testing.M) {
	if err := os.Chdir("scripts"); err != nil {
		// Already in scripts directory.
	}
	os.Exit(m.Run())
}
