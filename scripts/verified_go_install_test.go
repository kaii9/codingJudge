package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVerifiedGoInstallRetriesOnlyTransientTransport(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		attempts int
		succeeds bool
	}{
		{"network-once", 2, true},
		{"network-always", 3, false},
		{"permanent", 1, false},
		{"checksum", 1, false},
		{"success", 1, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			dir := t.TempDir()
			calls := filepath.Join(dir, "calls")
			fake := `#!/bin/sh
[ "$GOSUMDB" = sum.golang.org ] || { echo 'checksum environment changed' >&2; exit 99; }
printf '%s\n' "$*" >> "$TASK_GO_CALLS"
count=$(wc -l < "$TASK_GO_CALLS")
case "$TASK_GO_SCENARIO" in
  network-once) if [ "$count" -gt 1 ]; then exit 0; fi ;;
  success) exit 0 ;;
  permanent) echo 'compile error: undefined symbol' >&2; exit 2 ;;
  checksum) echo 'SECURITY ERROR: checksum mismatch (unexpected EOF)' >&2; exit 1 ;;
esac
echo 'reading https://sum.golang.org/tile/8/0/x135/430: stream error: INTERNAL_ERROR; received from peer' >&2
exit 1
`
			if err := os.WriteFile(filepath.Join(dir, "go"), []byte(fake), 0o700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, "sh", "../deploy/verified-go-install.sh", "-trimpath", "github.com/minio/minio@0d7408fc9969caf07de6a8c3a84f9fbb10a6739e")
			command.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "TASK_GO_CALLS="+calls, "TASK_GO_SCENARIO="+fixture.name, "GOSUMDB=sum.golang.org")
			output, err := command.CombinedOutput()
			if (err == nil) != fixture.succeeds {
				t.Fatalf("error=%v, output=%s", err, output)
			}
			if err != nil {
				wantStatus := 1
				if fixture.name == "permanent" {
					wantStatus = 2
				}
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != wantStatus {
					t.Fatalf("exit status not preserved: %v, want %d", err, wantStatus)
				}
			}
			data, readErr := os.ReadFile(calls)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if count := strings.Count(string(data), "\n"); count != fixture.attempts {
				t.Fatalf("attempts=%d, want%d; output=%s", count, fixture.attempts, output)
			}
			for _, call := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				if call != "install -trimpath github.com/minio/minio@0d7408fc9969caf07de6a8c3a84f9fbb10a6739e" {
					t.Fatalf("install arguments changed: %q", call)
				}
			}
			if strings.Contains(string(output), "GOSUMDB=off") {
				t.Fatal("integrity verification disabled")
			}
		})
	}
}
