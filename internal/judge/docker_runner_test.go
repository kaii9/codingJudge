package judge

import (
	"slices"
	"strings"
	"testing"

	"github.com/kai/codingjudge/internal/domain"
)

func TestDockerRunnerSupportsBatchExecution(t *testing.T) {
	t.Parallel()

	var _ BatchRunner = (*DockerRunner)(nil)
}

func TestLimitedBufferCapsCapturedOutput(t *testing.T) {
	t.Parallel()

	buffer := newLimitedBuffer(5)
	written, err := buffer.Write([]byte("123456789"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if written != 9 {
		t.Fatalf("Write returned %d, want 9", written)
	}
	if got := buffer.String(); got != "12345" {
		t.Fatalf("captured output = %q, want %q", got, "12345")
	}
	if !buffer.Truncated() {
		t.Fatal("buffer should report truncation")
	}
}

func TestDockerRunArgsIncludeSandboxLimits(t *testing.T) {
	t.Parallel()

	args := dockerRunArgs(RunRequest{
		Language:      domain.LanguageGo,
		TimeLimitMS:   1000,
		MemoryLimitMB: 64,
	}, "/tmp/codingjudge-1")

	for _, want := range []string{
		"run",
		"--rm",
		"--network", "none",
		"--memory", "64m",
		"--cpus", "1",
		"--pids-limit", "64",
		"--read-only",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"golang:1.25-alpine",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("docker args missing %q in %#v", want, args)
		}
	}
}

func TestDockerRunArgsSelectLanguageRuntime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		language   domain.Language
		image      string
		runCommand string
	}{
		{name: "go", language: domain.LanguageGo, image: "golang:1.25-alpine", runCommand: "/workspace/program"},
		{name: "cpp", language: domain.LanguageCPP, image: "gcc:13", runCommand: "/workspace/program"},
		{name: "python", language: domain.LanguagePython, image: "python:3.12-alpine", runCommand: "python /workspace/main.py"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := dockerRunArgs(RunRequest{Language: tt.language}, "/tmp/codingjudge-1")
			if !slices.Contains(args, tt.image) {
				t.Fatalf("docker args missing image %q in %#v", tt.image, args)
			}
			if !containsSubstring(args, tt.runCommand) {
				t.Fatalf("docker args missing run command substring %q in %#v", tt.runCommand, args)
			}
			if containsSubstring(args, "go run") || containsSubstring(args, "g++") {
				t.Fatalf("runtime command must not compile source: %#v", args)
			}
		})
	}
}

func TestDockerCompileArgsSelectCompilerAndWritableWorkspace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		language domain.Language
		command  string
	}{
		{name: "go", language: domain.LanguageGo, command: "go build -o /workspace/program /workspace/main.go"},
		{name: "cpp", language: domain.LanguageCPP, command: "g++ /workspace/main.cpp -O2 -std=c++17 -o /workspace/program"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args, ok := dockerCompileArgs(CompileRequest{
				Language:      tt.language,
				MemoryLimitMB: 64,
			}, "/tmp/codingjudge-1")
			if !ok {
				t.Fatal("dockerCompileArgs reported no compile step")
			}
			if !containsSubstring(args, tt.command) {
				t.Fatalf("docker args missing compile command %q in %#v", tt.command, args)
			}
			if containsSubstring(args, "/workspace:ro") {
				t.Fatalf("compile workspace must be writable: %#v", args)
			}
			if !containsAdjacent(args, "--memory", "512m") {
				t.Fatalf("compile memory limit must be independent from runtime limit: %#v", args)
			}
		})
	}
}

func TestDockerCompileArgsSkipsInterpretedLanguage(t *testing.T) {
	t.Parallel()

	if args, ok := dockerCompileArgs(CompileRequest{Language: domain.LanguagePython}, "/tmp/codingjudge-1"); ok || args != nil {
		t.Fatalf("python compile args = %#v, %v; want nil, false", args, ok)
	}
}

func containsSubstring(values []string, substr string) bool {
	for _, value := range values {
		if strings.Contains(value, substr) {
			return true
		}
	}
	return false
}

func containsAdjacent(values []string, first, second string) bool {
	for i := 0; i+1 < len(values); i++ {
		if values[i] == first && values[i+1] == second {
			return true
		}
	}
	return false
}
