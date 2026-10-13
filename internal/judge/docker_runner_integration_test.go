//go:build integration

package judge

import (
	"context"
	"fmt"
	"github.com/kaii9/codingJudge/internal/domain"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFastProgramBudgetExcludesDockerLifecycle(t *testing.T) {
	if os.Getenv("TEST_DOCKER_RUNNER") == "" {
		t.Skip("TEST_DOCKER_RUNNER is not set")
	}
	// Inject a known pre-container delay instead of assuming that Python must
	// start in 300ms on every loaded Docker Desktop/CI machine. This still uses
	// the real daemon, container and interpreter; production limits are unchanged.
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		t.Fatal(err)
	}
	proxyDir := t.TempDir()
	quotedDocker := "'" + strings.ReplaceAll(dockerPath, "'", "'\\''") + "'"
	proxy := "#!/bin/sh\nif [ \"$1\" = run ]; then sleep 2; fi\nexec " + quotedDocker + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(proxyDir, "docker"), []byte(proxy), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", proxyDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runner := NewDockerRunnerWithWorkDir("", "")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	started := time.Now()
	result, err := runner.Run(ctx, RunRequest{Language: domain.LanguagePython, Code: "print(3)", TimeLimitMS: 1000, MemoryLimitMB: 64})
	if err != nil || result.TimedOut || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "3" {
		t.Fatalf("fast program rejected: %+v err=%v", result, err)
	}
	if time.Since(started) < 2*time.Second {
		t.Fatal("the injected lifecycle delay did not run")
	}
}

func TestSandboxRunsUnprivilegedAndCannotReadTimingSecret(t *testing.T) {
	if os.Getenv("TEST_DOCKER_RUNNER") == "" {
		t.Skip("TEST_DOCKER_RUNNER is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runner := NewDockerRunnerWithWorkDir("", "")
	code := "import os\nprint(os.getuid())\ntry:\n open('/proc/1/environ','rb').read()\n print('leaked')\nexcept PermissionError:\n print('isolated')"
	result, err := runner.Run(ctx, RunRequest{Language: domain.LanguagePython, Code: code, TimeLimitMS: 3000, MemoryLimitMB: 64})
	if err != nil || result.TimedOut || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "65534\nisolated" {
		t.Fatalf("unprivileged sandbox: %+v err=%v", result, err)
	}
}

func TestProgramTimeoutLeavesNoOwnedSandbox(t *testing.T) {
	if os.Getenv("TEST_DOCKER_RUNNER") == "" {
		t.Skip("TEST_DOCKER_RUNNER is not set")
	}
	owner := fmt.Sprintf("budget-test-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runner := NewDockerRunnerWithWorkDir("", "", WithSandboxOwner(owner))
	result, err := runner.Run(ctx, RunRequest{Language: domain.LanguagePython, Code: "while True: pass", TimeLimitMS: 150, MemoryLimitMB: 64})
	if err != nil || !result.TimedOut {
		t.Fatalf("timeout result=%+v err=%v", result, err)
	}
	output, err := exec.Command("docker", "ps", "-aq", "--filter", "label=gojudge.executor="+owner).CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "" {
		t.Fatalf("leaked sandbox: %s err=%v", output, err)
	}
}

func TestTimedOutDockerRunRemovesContainer(t *testing.T) {
	if os.Getenv("TEST_DOCKER_RUNNER") == "" {
		t.Skip("TEST_DOCKER_RUNNER is not set")
	}
	if output, err := exec.Command("docker", "image", "inspect", "python:3.12-alpine").CombinedOutput(); err != nil {
		t.Fatalf("python judge image is required: %v\n%s", err, output)
	}

	name := fmt.Sprintf("codingjudge-timeout-test-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	result, err := executeDockerNamed(ctx, []string{
		"run", "--rm", "--network", "none", "python:3.12-alpine",
		"python", "-c", "while True: pass",
	}, name)
	if err != nil {
		t.Fatal(err)
	}
	if !result.TimedOut {
		t.Fatalf("result = %+v, want timeout", result)
	}

	output, inspectErr := exec.Command("docker", "inspect", name).CombinedOutput()
	if inspectErr == nil || !strings.Contains(strings.ToLower(string(output)), "no such object") {
		t.Fatalf("timed-out container still exists: err=%v output=%s", inspectErr, output)
	}
}

func TestOwnedSandboxCleanupLeavesOtherExecutorsUntouched(t *testing.T) {
	if os.Getenv("TEST_DOCKER_RUNNER") == "" {
		t.Skip("TEST_DOCKER_RUNNER is not set")
	}
	owner := fmt.Sprintf("cleanup-test-%d", time.Now().UnixNano())
	ownName := "codingjudge-exec-" + owner
	otherName := ownName + "-other"
	for _, entry := range [][2]string{{ownName, owner}, {otherName, owner + "-other"}} {
		if output, err := exec.Command("docker", "run", "-d", "--rm", "--network", "none", "--name", entry[0], "--label", "gojudge.executor="+entry[1], "python:3.12-alpine", "sleep", "60").CombinedOutput(); err != nil {
			t.Fatalf("create fixture: %v %s", err, output)
		}
		name := entry[0]
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = forceRemoveContainer(ctx, name)
		})
	}
	runner := NewDockerRunnerWithWorkDir("", "", WithSandboxOwner(owner))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	count, err := runner.CleanupOwnedSandboxes(ctx)
	if err != nil || count != 1 {
		t.Fatalf("cleanup count=%d err=%v", count, err)
	}
	if err := exec.Command("docker", "inspect", ownName).Run(); err == nil {
		t.Fatal("own orphan survived")
	}
	if err := exec.Command("docker", "inspect", otherName).Run(); err != nil {
		t.Fatal("other executor sandbox was removed")
	}
}
