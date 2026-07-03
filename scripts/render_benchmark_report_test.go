package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRenderFromFixtures(t *testing.T) {
	// Build the report binary once.
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
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("render: %v\n%s", err, out)
	}

	output := string(out)
	// Verify the ordered table is present.
	for _, want := range []string{
		"| Workers | Submission rate | HTTP P95 | Judge P95 | Failure rate | Peak pending |",
		"| 1 |",
		"| 2 |",
		"| 4 |",
		"## Interpretation",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in output", want)
		}
	}

	// Missing summary file should return non-zero.
	errCmd := exec.Command(binary, "testdata/meta.txt", "nonexistent.json", "nonexistent2.json", "nonexistent3.json")
	errCmd.Dir = "."
	if errCmd.Run() == nil {
		t.Error("expected non-zero exit for missing file")
	}
}

func TestMain(m *testing.M) {
	// Change to script directory to find testdata.
	if err := os.Chdir("scripts"); err != nil {
		// Already in scripts dir or running from project root.
	}
	os.Exit(m.Run())
}
