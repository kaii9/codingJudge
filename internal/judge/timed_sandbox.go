package judge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type controlEvent struct {
	kind string
	exit int
	at   time.Time
}
type controlWriter struct {
	output   *limitedBuffer
	token    string
	events   chan<- controlEvent
	line     []byte
	overflow bool
}

func (w *controlWriter) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 {
		pos := bytes.IndexByte(data, '\n')
		complete := pos >= 0
		var part []byte
		if complete {
			part = data[:pos+1]
			data = data[pos+1:]
		} else {
			part = data
			data = nil
		}
		if w.overflow {
			_, _ = w.output.Write(part)
			if complete {
				w.overflow = false
			}
			continue
		}
		if len(w.line)+len(part) > 256 {
			_, _ = w.output.Write(w.line)
			_, _ = w.output.Write(part)
			w.line = nil
			w.overflow = !complete
			continue
		}
		w.line = append(w.line, part...)
		if !complete {
			continue
		}
		text := strings.TrimSuffix(string(w.line), "\n")
		if strings.HasPrefix(text, w.token+":") {
			payload := strings.TrimPrefix(text, w.token+":")
			event := controlEvent{at: time.Now()}
			if payload == "start" {
				event.kind = "start"
			} else if strings.HasPrefix(payload, "exit:") {
				event.kind = "exit"
				event.exit, _ = strconv.Atoi(strings.TrimPrefix(payload, "exit:"))
			}
			if event.kind != "" {
				select {
				case w.events <- event:
				default:
				}
			}
		} else {
			_, _ = w.output.Write(w.line)
		}
		w.line = nil
	}
	return n, nil
}
func (w *controlWriter) flush() { _, _ = w.output.Write(w.line); w.line = nil }

// Root emits timing frames; nobody executes code with a cleared environment.
// The frame token remains in root's environment and is not readable by user code.
func executeTimedSandbox(ctx context.Context, args []string, limit time.Duration) (result RunResult, err error) {
	name, err := newContainerName()
	if err != nil {
		return result, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return result, err
	}
	token := "CJ_CONTROL_" + hex.EncodeToString(random[:])
	quoted := "'" + strings.ReplaceAll(args[len(args)-1], "'", "'\\''") + "'"
	wrapper := "printf '%s:start\\n' \"$CJ_CONTROL_TOKEN\" >&2; env -i PATH=\"$PATH\" su -s /bin/sh nobody -c " + quoted + "; status=$?; printf '\\n%s:exit:%s\\n' \"$CJ_CONTROL_TOKEN\" \"$status\" >&2; exit \"$status\""
	wrapped := append([]string{"run", "--cap-add", "SETUID", "--cap-add", "SETGID", "--env", "CJ_CONTROL_TOKEN=" + token}, args[1:len(args)-3]...)
	wrapped = append(wrapped, "sh", "-c", wrapper)
	named, err := dockerArgsWithName(wrapped, name)
	if err != nil {
		return result, err
	}
	transportCtx, cancelTransport := context.WithTimeout(ctx, limit+20*time.Second)
	defer cancelTransport()
	stdout, stderr := newLimitedBuffer(maxCapturedOutputBytes), newLimitedBuffer(maxCapturedOutputBytes)
	events := make(chan controlEvent, 4)
	control := &controlWriter{output: stderr, token: token, events: events}
	command := exec.CommandContext(transportCtx, "docker", named...)
	command.Stdout, command.Stderr = stdout, control
	done := make(chan error, 1)
	go func() { done <- command.Run() }()
	defer func() {
		cancelTransport()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if cleanupErr := forceRemoveContainer(cleanupCtx, name); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("sandbox cleanup: %w", cleanupErr))
		}
	}()
	var started, finished time.Time
	var timer *time.Timer
	var deadline <-chan time.Time
	var exitCode int
	timedOut := false
	transportDone := transportCtx.Done()
	for {
		select {
		case event := <-events:
			if event.kind == "start" && started.IsZero() {
				started = event.at
				timer = time.NewTimer(limit)
				deadline = timer.C
				defer timer.Stop()
			}
			if event.kind == "exit" && !started.IsZero() {
				finished = event.at
				exitCode = event.exit
				deadline = nil
				if timer != nil {
					timer.Stop()
				}
			}
		case <-deadline:
			select {
			case event := <-events:
				if event.kind == "exit" {
					finished = event.at
					exitCode = event.exit
					deadline = nil
					continue
				}
			default:
			}
			timedOut = true
			cancelTransport()
			deadline = nil
			transportDone = nil
		case commandErr := <-done:
			for len(events) > 0 {
				event := <-events
				if event.kind == "start" && started.IsZero() {
					started = event.at
				}
				if event.kind == "exit" {
					finished = event.at
					exitCode = event.exit
				}
			}
			control.flush()
			duration := time.Duration(0)
			if !started.IsZero() {
				end := finished
				if end.IsZero() {
					end = time.Now()
				}
				duration = end.Sub(started)
			}
			result = RunResult{Stdout: capturedOutput(stdout, "stdout"), Stderr: strings.TrimSpace(capturedOutput(stderr, "stderr")), ExitCode: exitCode, Duration: duration.Milliseconds(), TimedOut: timedOut || (!finished.IsZero() && duration > limit)}
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			if timedOut {
				return result, nil
			}
			if started.IsZero() || finished.IsZero() {
				return result, fmt.Errorf("sandbox lifecycle incomplete: %v", commandErr)
			}
			return result, nil
		case <-transportDone:
			if !timedOut {
				commandErr := <-done
				control.flush()
				return result, fmt.Errorf("sandbox transport failed: %w: %v", transportCtx.Err(), commandErr)
			}
		}
	}
}
