package container

import (
	"context"
	"io"
)

// Runtime and preference-string names for the two supported engines.
const (
	RuntimeDocker = "docker"
	RuntimePodman = "podman"
)

// autoDetectOrder is the precedence used when no runtime is explicitly
// preferred: docker before podman.
var autoDetectOrder = []string{RuntimeDocker, RuntimePodman}

// Runtime is a container engine that Detect has confirmed is installed and
// whose daemon responded to a liveness check, ready to Run containers.
type Runtime struct {
	name   string
	binary string
	exec   execRunner
}

// Name reports which engine this Runtime drives ("docker" or "podman").
func (r *Runtime) Name() string { return r.name }

// Detect finds a usable container runtime.
//
// If preferred is "docker" or "podman", only that runtime is considered:
// Detect fails with *NotInstalledError if it's not on $PATH, or
// *DaemonDownError if it's installed but its daemon doesn't respond.
//
// If preferred is "", Detect auto-detects: it tries docker then podman (in
// that order) on $PATH, and returns the first one found whose daemon
// responds. Binary presence alone isn't enough — colima/lima/Docker Desktop
// all install a `docker` binary that resolves fine on $PATH even when the
// VM backing it isn't running, so Detect always runs a liveness check
// (`docker info` / `podman info`) before returning a Runtime.
func Detect(ctx context.Context, preferred string) (*Runtime, error) {
	return detect(ctx, preferred, realExec{})
}

func detect(ctx context.Context, preferred string, ex execRunner) (*Runtime, error) {
	candidates := autoDetectOrder
	if preferred != "" {
		candidates = []string{preferred}
	}

	for _, name := range candidates {
		path, err := ex.LookPath(name)
		if err != nil {
			continue
		}
		if err := checkDaemon(ctx, ex, name, path); err != nil {
			return nil, err
		}
		return &Runtime{name: name, binary: path, exec: ex}, nil
	}

	return nil, &NotInstalledError{Preferred: preferred}
}

// checkDaemon runs `<binary> info`, discarding its output, purely to
// confirm the daemon behind the CLI answers. A non-zero exit or launch
// failure means the CLI is present but the daemon isn't reachable.
func checkDaemon(ctx context.Context, ex execRunner, name, path string) error {
	exitCode, err := ex.Run(ctx, path, []string{"info"}, io.Discard, io.Discard)
	if err != nil || exitCode != 0 {
		return &DaemonDownError{Runtime: name, Err: err}
	}
	return nil
}
