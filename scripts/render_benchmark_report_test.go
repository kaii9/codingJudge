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
		"| Workers | Offered rate | Created/s | Accepted/s | HTTP rate | HTTP P95 | Judge P95 | Failure rate | Peak pending (sampled) |",
		"| 1 | 1.00/s | 1.00/s | 1.00/s |",
		"| 2 | 1.00/s | 1.00/s | 1.00/s |",
		"| 4 | 1.00/s | 1.00/s | 1.00/s |",
		"# Fixed-Load Worker Scaling Benchmark",
		"constant-arrival-rate",
		"fixed-load benchmark",
		"not a maximum-throughput test",
		"Docker socket passthrough",
		"returns to 0 after each round",
		"sampled every 5 seconds",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in output:\n%s", want, output)
		}
	}

	for _, forbidden := range []string{
		"Zero peak pending",
		"NaN",
	} {
		if strings.Contains(output, forbidden) {
			t.Errorf("found forbidden text %q in output", forbidden)
		}
	}
}

func TestRenderFailsOnDroppedIterations(t *testing.T) {
	tmp := t.TempDir()
	binary := tmp + "/render"
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// Create a summary with dropped_iterations > 0.
	badJSON := tmp + "/bad.json"
	os.WriteFile(badJSON, []byte(`{
  "metrics": {
    "iterations": {"count": 100, "rate": 1.0},
    "dropped_iterations": {"count": 5},
    "submissions_created": {"rate": 1.0, "count": 100},
    "submissions_accepted": {"rate": 1.0, "count": 100},
    "http_reqs": {"rate": 45.0, "count": 5000},
    "http_req_duration": {"avg": 1.0, "p(90)": 3.0, "p(95)": 5.0},
    "http_req_failed": {"value": 0.0},
    "judge_terminal_duration": {"avg": 1000.0, "p(90)": 1500.0, "p(95)": 2000.0}
  }
}`), 0644)

	cmd := exec.Command(binary, "testdata/meta.txt", badJSON, badJSON, badJSON)
	if cmd.Run() == nil {
		t.Error("expected non-zero exit for dropped_iterations > 0")
	}
}

func TestRenderFailsOnMismatchedCounts(t *testing.T) {
	tmp := t.TempDir()
	binary := tmp + "/render"
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// accepted != created.
	badJSON := tmp + "/mismatch.json"
	os.WriteFile(badJSON, []byte(`{
  "metrics": {
    "iterations": {"count": 100, "rate": 1.0},
    "dropped_iterations": {"count": 0},
    "submissions_created": {"rate": 1.0, "count": 100},
    "submissions_accepted": {"rate": 0.8, "count": 80},
    "http_reqs": {"rate": 45.0, "count": 5000},
    "http_req_duration": {"avg": 1.0, "p(90)": 3.0, "p(95)": 5.0},
    "http_req_failed": {"value": 0.0},
    "judge_terminal_duration": {"avg": 1000.0, "p(90)": 1500.0, "p(95)": 2000.0}
  }
}`), 0644)

	cmd := exec.Command(binary, "testdata/meta.txt", badJSON, badJSON, badJSON)
	if cmd.Run() == nil {
		t.Error("expected non-zero exit for accepted != created")
	}
}

func TestRenderFailsOnWrongIterationCount(t *testing.T) {
	tmp := t.TempDir()
	binary := tmp + "/render"
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// created != iterations.
	badJSON := tmp + "/baditer.json"
	os.WriteFile(badJSON, []byte(`{
  "metrics": {
    "iterations": {"count": 80, "rate": 0.8},
    "dropped_iterations": {"count": 0},
    "submissions_created": {"rate": 1.0, "count": 100},
    "submissions_accepted": {"rate": 1.0, "count": 100},
    "http_reqs": {"rate": 45.0, "count": 5000},
    "http_req_duration": {"avg": 1.0, "p(90)": 3.0, "p(95)": 5.0},
    "http_req_failed": {"value": 0.0},
    "judge_terminal_duration": {"avg": 1000.0, "p(90)": 1500.0, "p(95)": 2000.0}
  }
}`), 0644)

	cmd := exec.Command(binary, "testdata/meta.txt", badJSON, badJSON, badJSON)
	if cmd.Run() == nil {
		t.Error("expected non-zero exit for created != iterations")
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
	if strings.Contains(string(out), "NaN") {
		t.Error("output must not contain NaN")
	}
	if !strings.Contains(string(out), "| Workers |") {
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
