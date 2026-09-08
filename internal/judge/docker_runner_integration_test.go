//go:build integration

package judge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

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
