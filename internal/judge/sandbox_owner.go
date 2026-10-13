package judge

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

func WithSandboxOwner(owner string) DockerRunnerOption {
	return func(r *DockerRunner) { r.owner = owner }
}
func (r *DockerRunner) ownedArgs(args []string) []string {
	if r.owner == "" {
		return args
	}
	return append([]string{"run", "--label", "gojudge.executor=" + r.owner}, args[1:]...)
}

var containerID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

func (r *DockerRunner) CleanupOwnedSandboxes(ctx context.Context) (int, error) {
	if r.owner == "" {
		return 0, nil
	}
	output, err := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label=gojudge.executor="+r.owner, "--filter", "name=codingjudge-exec-").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("list owned sandbox containers: %w", err)
	}
	count := 0
	for _, id := range strings.Fields(string(output)) {
		if !containerID.MatchString(id) {
			return count, fmt.Errorf("invalid owned container ID")
		}
		if err := forceRemoveContainer(ctx, id); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
