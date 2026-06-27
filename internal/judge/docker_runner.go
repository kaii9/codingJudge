package judge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kai/codingjudge/internal/domain"
)

const maxCapturedOutputBytes = 1 << 20

type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	originalLength := len(p)
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = b.truncated || originalLength > 0
		return originalLength, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, err := b.buffer.Write(p)
	return originalLength, err
}

func (b *limitedBuffer) String() string {
	return b.buffer.String()
}

func (b *limitedBuffer) Truncated() bool {
	return b.truncated
}

type DockerRunner struct {
	image       string
	workDirRoot string
}

type CompileRequest struct {
	Language      domain.Language
	MemoryLimitMB int
}

func NewDockerRunner(image string) *DockerRunner {
	return NewDockerRunnerWithWorkDir(image, "")
}

func NewDockerRunnerWithWorkDir(image, workDirRoot string) *DockerRunner {
	return &DockerRunner{image: image, workDirRoot: workDirRoot}
}

func (r *DockerRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	results, err := r.RunBatch(ctx, req, []string{req.Input})
	if err != nil {
		return RunResult{}, err
	}
	if len(results) == 0 {
		return RunResult{}, fmt.Errorf("docker runner returned no result")
	}
	return results[0], nil
}

func (r *DockerRunner) RunBatch(ctx context.Context, req RunRequest, inputs []string) ([]RunResult, error) {
	spec, ok := languageSpecFor(req.Language)
	if !ok {
		return nil, fmt.Errorf("unsupported language %q", req.Language)
	}

	workdir, err := os.MkdirTemp(r.workDirRoot, "codingjudge-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workdir)

	if err := os.WriteFile(filepath.Join(workdir, spec.filename), []byte(req.Code), 0o600); err != nil {
		return nil, err
	}

	if compileArgs, ok := dockerCompileArgs(CompileRequest{
		Language:      req.Language,
		MemoryLimitMB: req.MemoryLimitMB,
	}, workdir); ok {
		if r.image != "" {
			compileArgs = replaceImage(compileArgs, r.image)
		}
		compileCtx, cancelCompile := context.WithTimeout(ctx, 10*time.Second)
		compileResult, err := executeDocker(compileCtx, compileArgs)
		cancelCompile()
		if err != nil {
			return nil, err
		}
		if compileResult.TimedOut {
			compileResult.Stderr = "compilation timed out"
			return []RunResult{compileResult}, nil
		}
		if compileResult.ExitCode != 0 {
			return []RunResult{compileResult}, nil
		}
	}

	results := make([]RunResult, 0, len(inputs))
	for _, input := range inputs {
		if err := os.WriteFile(filepath.Join(workdir, "input.txt"), []byte(input), 0o600); err != nil {
			return nil, err
		}
		result, err := r.runPrepared(ctx, req, workdir)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
		if result.TimedOut || result.ExitCode != 0 {
			break
		}
	}
	return results, nil
}

func (r *DockerRunner) runPrepared(ctx context.Context, req RunRequest, workdir string) (RunResult, error) {
	runLimit := time.Duration(req.TimeLimitMS) * time.Millisecond
	if runLimit <= 0 {
		runLimit = time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, runLimit+500*time.Millisecond)
	defer cancel()

	args := dockerRunArgs(req, workdir)
	if r.image != "" {
		args = replaceImage(args, r.image)
	}
	return executeDocker(runCtx, args)
}

func executeDocker(ctx context.Context, args []string) (RunResult, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	started := time.Now()
	stdout := newLimitedBuffer(maxCapturedOutputBytes)
	stderr := newLimitedBuffer(maxCapturedOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	duration := time.Since(started).Milliseconds()
	result := RunResult{
		Stdout:   capturedOutput(stdout, "stdout"),
		Stderr:   capturedOutput(stderr, "stderr"),
		Duration: duration,
	}
	if ctx.Err() != nil {
		result.TimedOut = true
		return result, nil
	}
	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

func capturedOutput(buffer *limitedBuffer, name string) string {
	output := buffer.String()
	if buffer.Truncated() {
		output += fmt.Sprintf("\n[%s truncated at %d bytes]", name, maxCapturedOutputBytes)
	}
	return output
}

func dockerRunArgs(req RunRequest, workdir string) []string {
	spec, ok := languageSpecFor(req.Language)
	if !ok {
		spec, _ = languageSpecFor(domain.LanguageGo)
	}
	args := dockerSandboxArgs(req.MemoryLimitMB)
	args = append(args,
		"-v", workdir+":/workspace:ro",
		"-w", "/workspace",
		spec.image,
		"sh", "-c", spec.runCommand,
	)
	return args
}

func dockerCompileArgs(req CompileRequest, workdir string) ([]string, bool) {
	spec, ok := languageSpecFor(req.Language)
	if !ok || spec.compileCommand == "" {
		return nil, false
	}
	args := dockerSandboxArgs(512)
	args = append(args,
		"-v", workdir+":/workspace:rw",
		"-w", "/workspace",
		spec.image,
		"sh", "-c", spec.compileCommand,
	)
	return args, true
}

func dockerSandboxArgs(memoryLimitMB int) []string {
	memoryMB := memoryLimitMB
	if memoryMB <= 0 {
		memoryMB = 64
	}
	return []string{
		"run",
		"--rm",
		"--network", "none",
		"--memory", strconv.Itoa(memoryMB) + "m",
		"--cpus", "1",
		"--pids-limit", "64",
		"--read-only",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
	}
}

type languageSpec struct {
	filename       string
	image          string
	compileCommand string
	runCommand     string
}

func languageSpecFor(language domain.Language) (languageSpec, bool) {
	switch language {
	case "", domain.LanguageGo:
		return languageSpec{
			filename:       "main.go",
			image:          "golang:1.25-alpine",
			compileCommand: "GOCACHE=/tmp/go-build HOME=/tmp go build -o /workspace/program /workspace/main.go",
			runCommand:     "/workspace/program < /workspace/input.txt",
		}, true
	case domain.LanguageCPP:
		return languageSpec{
			filename:       "main.cpp",
			image:          "gcc:13",
			compileCommand: "g++ /workspace/main.cpp -O2 -std=c++17 -o /workspace/program",
			runCommand:     "/workspace/program < /workspace/input.txt",
		}, true
	case domain.LanguagePython:
		return languageSpec{
			filename:   "main.py",
			image:      "python:3.12-alpine",
			runCommand: "python /workspace/main.py < /workspace/input.txt",
		}, true
	default:
		return languageSpec{}, false
	}
}

func replaceImage(args []string, image string) []string {
	replaced := append([]string(nil), args...)
	for i, arg := range replaced {
		if arg == "golang:1.25-alpine" || arg == "gcc:13" || arg == "python:3.12-alpine" {
			replaced[i] = image
			return replaced
		}
	}
	return replaced
}
