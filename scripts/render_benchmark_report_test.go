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
		"| Workers | Offered rate | Created/s | Accepted/s | HTTP rate | HTTP P95 | Judge P95 | Failure rate | Peak pending |",
		"| 1 |",
		"| 2 |",
		"| 4 |",
		"# Fixed-Load Worker Scaling Benchmark",
		"## Environment",
		"## Interpretation",
		"fixed-load benchmark",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in output:\n%s", want, output)
		}
	}
	if strings.Contains(output, "NaN") {
		t.Errorf("output must not contain NaN:\n%s", output)
	}

	// Missing summary file should return non-zero.
	errCmd := exec.Command(binary, "testdata/meta.txt", "nonexistent.json", "nonexistent2.json", "nonexistent3.json")
	if errCmd.Run() == nil {
		t.Error("expected non-zero exit for missing file")
	}
}

func TestParseRealK6Structure(t *testing.T) {
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
	if strings.Contains(output, "NaN") {
		t.Errorf("output must not contain NaN:\n%s", output)
	}
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

	errCmd := exec.Command(binary, "testdata/meta.txt", "testdata/smoke-real-structure.json", "nonexistent.json", "nonexistent2.json")
	if errCmd.Run() == nil {
		t.Error("expected non-zero exit for missing file")
	}
}

func TestMain(m *testing.M) {
	if err := os.Chdir("scripts"); err != nil {
	}
	os.Exit(m.Run())
}
