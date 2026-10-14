//go:build integration

package judge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kaii9/codingJudge/internal/domain"
)

func requireGoCompilerImage(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_GO_COMPILER") != "1" {
		t.Skip("TEST_GO_COMPILER=1 is required for real cold Go compilation")
	}
	if output, err := exec.Command("docker", "image", "inspect", "golang:1.25-alpine").CombinedOutput(); err != nil {
		t.Fatalf("Go compiler image required: %v %s", err, output)
	}
}

func TestColdGoCompilerReturnsRunAndCompileVerdicts(t *testing.T) {
	requireGoCompilerImage(t)
	for _, fixture := range []struct {
		name, code   string
		compileError bool
	}{
		{"sum", `package main
import "fmt"
func main(){var a,b int;fmt.Scan(&a,&b);fmt.Println(a+b)}`, false},
		{"syntax-error", `package main
func main(){`, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			started := time.Now()
			results, err := NewDockerRunner("").RunBatch(ctx, RunRequest{Language: domain.LanguageGo, Code: fixture.code, TimeLimitMS: 2000, MemoryLimitMB: 64}, []string{"1 2\n", "-3 5\n"})
			if err != nil {
				t.Fatal(err)
			}
			if fixture.compileError {
				if len(results) != 1 || results[0].Stage != StageCompile || results[0].ExitCode == 0 || results[0].TimedOut || !strings.Contains(results[0].Stderr, "syntax error") {
					t.Fatalf("compile result=%+v", results)
				}
			} else {
				if len(results) != 2 {
					t.Fatalf("results=%+v", results)
				}
				for i, want := range []string{"3", "2"} {
					if results[i].Stage != StageRun || results[i].ExitCode != 0 || results[i].TimedOut || strings.TrimSpace(results[i].Stdout) != want {
						t.Fatalf("run%d=%+v", i, results[i])
					}
				}
			}
			t.Logf("cold compiler %s completed in %s, budget unchanged", fixture.name, time.Since(started))
		})
	}
}

func TestInFlightGoCompileCancellationRemovesSandbox(t *testing.T) {
	requireGoCompilerImage(t)
	owner := fmt.Sprintf("go-compile-cancel-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := NewDockerRunnerWithWorkDir("", "", WithSandboxOwner(owner)).Run(ctx, RunRequest{Language: domain.LanguageGo, Code: `package main
import "fmt"
func main(){fmt.Println(3)}`, TimeLimitMS: 1000, MemoryLimitMB: 64})
		done <- err
	}()
	// Cancel only after Docker confirms the compiler container is running.
	for {
		queryCtx, queryCancel := context.WithTimeout(ctx, time.Second)
		output, err := exec.CommandContext(queryCtx, "docker", "ps", "-q", "--filter", "label=gojudge.executor="+owner).CombinedOutput()
		queryCancel()
		if err == nil && strings.TrimSpace(string(output)) != "" {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("runner ended before in-flight cancellation: %v", err)
		case <-ctx.Done():
			t.Fatal("no active compiler observed")
		case <-time.After(50 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error=%v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("canceled compiler did not return")
	}
	output, err := exec.Command("docker", "ps", "-aq", "--filter", "label=gojudge.executor="+owner).CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "" {
		t.Fatalf("owned compiler leaked: %s error=%v", output, err)
	}
}
