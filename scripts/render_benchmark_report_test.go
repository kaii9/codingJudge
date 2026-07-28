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
		"| Workers | Offered rate | Created/s | Accepted/s | HTTP rate | HTTP P95 | Judge P95 | HTTP failure | Logical failure | Peak pending (sampled) |",
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

func TestRenderFailsWhenAcceptedLessThanCreated(t *testing.T) {
	tmp := t.TempDir()
	binary := tmp + "/render"
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// accepted=119, created=120 — must fail (plan requires exact equality).
	badJSON := tmp + "/bad.json"
	os.WriteFile(badJSON, []byte(`{
  "metrics": {
    "iterations": {"count": 120, "rate": 1.0},
    "dropped_iterations": {"count": 0},
    "submissions_created": {"rate": 1.0, "count": 120},
    "submissions_accepted": {"rate": 0.99, "count": 119},
    "logical_failures": {"value": 0.0},
    "http_reqs": {"rate": 45.0, "count": 5400},
    "http_req_duration": {"avg": 1.0, "p(90)": 3.0, "p(95)": 5.0},
    "http_req_failed": {"value": 0.0},
    "judge_terminal_duration": {"avg": 1000.0, "p(90)": 1500.0, "p(95)": 2000.0}
  }
}`), 0644)

	cmd := exec.Command(binary, "testdata/meta.txt", badJSON, badJSON, badJSON)
	if cmd.Run() == nil {
		t.Error("expected non-zero exit for accepted=119 != created=120")
	}
}

func TestRenderFailsOnLogicalFailures(t *testing.T) {
	tmp := t.TempDir()
	binary := tmp + "/render"
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// logical_failures.value=0.0083 while http_req_failed=0 — must fail.
	badJSON := tmp + "/logical.json"
	os.WriteFile(badJSON, []byte(`{
  "metrics": {
    "iterations": {"count": 120, "rate": 1.0},
    "dropped_iterations": {"count": 0},
    "submissions_created": {"rate": 1.0, "count": 120},
    "submissions_accepted": {"rate": 1.0, "count": 120},
    "logical_failures": {"value": 0.0082644628},
    "http_reqs": {"rate": 45.0, "count": 5400},
    "http_req_duration": {"avg": 1.0, "p(90)": 3.0, "p(95)": 5.0},
    "http_req_failed": {"value": 0.0},
    "judge_terminal_duration": {"avg": 1000.0, "p(90)": 1500.0, "p(95)": 2000.0}
  }
}`), 0644)

	cmd := exec.Command(binary, "testdata/meta.txt", badJSON, badJSON, badJSON)
	if cmd.Run() == nil {
		t.Error("expected non-zero exit for logical_failures > 0")
	}
}

func TestRenderFailsOnWrongArrivalCount(t *testing.T) {
	tmp := t.TempDir()
	binary := tmp + "/render"
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// offered rate=1/s for 2m → expected=120, tolerance=max(2,120*0.02)=2.4→2
	// created=100 is outside [118,122] → must fail.
	tmpl := `{
  "metrics": {
    "iterations": {"count": 100, "rate": 0.83},
    "dropped_iterations": {"count": 0},
    "submissions_created": {"rate": 0.83, "count": 100},
    "submissions_accepted": {"rate": 0.83, "count": 100},
    "logical_failures": {"value": 0.0},
    "http_reqs": {"rate": 45.0, "count": 5000},
    "http_req_duration": {"avg": 1.0, "p(90)": 3.0, "p(95)": 5.0},
    "http_req_failed": {"value": 0.0},
    "judge_terminal_duration": {"avg": 1000.0, "p(90)": 1500.0, "p(95)": 2000.0}
  }
}`
	badJSON := tmp + "/wrongcount.json"
	os.WriteFile(badJSON, []byte(tmpl), 0644)

	cmd := exec.Command(binary, "testdata/meta.txt", badJSON, badJSON, badJSON)
	if cmd.Run() == nil {
		t.Error("expected non-zero exit for created=100 (expected ~120)")
	}
}

func TestRenderFailsOnDroppedIterations(t *testing.T) {
	tmp := t.TempDir()
	binary := tmp + "/render"
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	badJSON := tmp + "/dropped.json"
	os.WriteFile(badJSON, []byte(`{
  "metrics": {
    "iterations": {"count": 120, "rate": 1.0},
    "dropped_iterations": {"count": 5},
    "submissions_created": {"rate": 1.0, "count": 120},
    "submissions_accepted": {"rate": 1.0, "count": 120},
    "logical_failures": {"value": 0.0},
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

	badJSON := tmp + "/mismatch.json"
	os.WriteFile(badJSON, []byte(`{
  "metrics": {
    "iterations": {"count": 80, "rate": 0.8},
    "dropped_iterations": {"count": 0},
    "submissions_created": {"rate": 1.0, "count": 100},
    "submissions_accepted": {"rate": 1.0, "count": 100},
    "logical_failures": {"value": 0.0},
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

func TestMinIOAssetSmokeUsesAuthenticatedAlias(t *testing.T) {
	script, err := os.ReadFile("smoke-minio-assets.sh")
	if err != nil {
		t.Fatalf("read smoke script: %v", err)
	}
	body := string(script)
	if !strings.Contains(body, "mc alias set cj http://localhost:9000 minioadmin minioadmin") {
		t.Fatal("smoke script must configure an authenticated MinIO alias before checking objects")
	}
	if strings.Contains(body, "mc stat \"local/") {
		t.Fatal("smoke script must not check objects through the unauthenticated default local alias")
	}
}

func TestMinIOAssetSmokeCleansComposeServices(t *testing.T) {
	script, err := os.ReadFile("smoke-minio-assets.sh")
	if err != nil {
		t.Fatalf("read smoke script: %v", err)
	}
	body := string(script)
	if !strings.Contains(body, "docker compose stop api worker postgres redis minio") {
		t.Fatal("smoke script should stop services on exit by default")
	}
	if !strings.Contains(body, "SMOKE_KEEP_STACK") {
		t.Fatal("smoke script should allow preserving the stack for debugging")
	}
}

func TestFrontendBuildRemovesStaleNextOutput(t *testing.T) {
	makefile, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	body := string(makefile)
	if !strings.Contains(body, "rm -rf frontend/.next") {
		t.Fatal("frontend-build should remove stale .next output before running next build")
	}
}

func TestFrontendDepsRemovesStaleNodeModules(t *testing.T) {
	makefile, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	if !strings.Contains(string(makefile), "rm -rf frontend/node_modules") {
		t.Fatal("frontend-deps should remove stale node_modules before npm ci")
	}
}

func TestCaseUploaderDefaultsToAllCaseAssets(t *testing.T) {
	makefile, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	compose, err := os.ReadFile("../docker-compose.yml")
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}
	if !strings.Contains(string(makefile), "$${CASE_UPLOAD_FLAGS:--all}") {
		t.Fatal("Makefile upload-cases target should default to -all")
	}
	if !strings.Contains(string(compose), "${CASE_UPLOAD_FLAGS:--all}") {
		t.Fatal("case-uploader compose service should default to -all")
	}
}

func TestMinIOAssetSmokeChecksHot20ObjectBackedCases(t *testing.T) {
	script, err := os.ReadFile("smoke-minio-assets.sh")
	if err != nil {
		t.Fatalf("read smoke script: %v", err)
	}
	body := string(script)
	if !strings.Contains(body, "collection='hot20'") || !strings.Contains(body, "expected Hot20 object-backed cases") {
		t.Fatal("MinIO smoke should verify Hot20 cases are object-backed")
	}
}

func TestMinIOAssetSmokeBuildsCaseUploaderImage(t *testing.T) {
	script, err := os.ReadFile("smoke-minio-assets.sh")
	if err != nil {
		t.Fatalf("read smoke script: %v", err)
	}
	if !strings.Contains(string(script), "docker compose --profile assets build case-uploader") {
		t.Fatal("MinIO smoke should rebuild the case-uploader profile image before running it")
	}
}

func TestCIWorkflowCoversMinIOObjectStoragePath(t *testing.T) {
	workflow, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	body := string(workflow)
	for _, want := range []string{
		"minio/minio:",
		"TEST_MINIO_ENDPOINT:",
		"./internal/objectstore",
		"scripts/smoke-minio-assets.sh",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("CI workflow missing %q", want)
		}
	}
}

func TestMain(m *testing.M) {
	if err := os.Chdir("scripts"); err != nil {
	}
	os.Exit(m.Run())
}
