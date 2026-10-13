package judge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestForceRemovalWaitsForConcurrentAutoRemove(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ ! -f '" + dir + "/once' ]; then\n touch '" + dir + "/once'\n echo 'Error response from daemon: removal of container fixture is already in progress' >&2\n exit 1\nfi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := forceRemoveContainer(ctx, "fixture"); err != nil {
		t.Fatalf("idempotent concurrent removal failed: %v", err)
	}
}
