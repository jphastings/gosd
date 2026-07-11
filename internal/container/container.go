// Package container is a small abstraction over shelling out to a
// container CLI (Docker or Podman) so Go code — currently only the future
// internal/kernelbuild — can run a long build inside a container without
// depending on either engine's SDK. It follows the same pattern as
// gosd-init's platform seams (see cmd/gosd-init/internal/netup): pure logic
// behind a small exec interface, with fake-driven tests that run on macOS
// without a container runtime installed.
package container

import (
	"context"
	"io"
)

// execRunner abstracts process execution so Detect and Runtime.Run can be
// tested with a fake, without spawning real processes or requiring a
// container daemon. The real implementation (realExec) wraps os/exec.
type execRunner interface {
	// LookPath resolves name to an absolute path, exactly like
	// exec.LookPath: it returns an error if name isn't found on $PATH.
	LookPath(name string) (string, error)

	// Run executes path with args, streaming stdout/stderr live to the
	// given writers as the process produces output (never buffering a
	// build's full output before it's visible, since kernel builds run
	// 20-60 minutes). It returns the process's exit code (best-effort;
	// -1 if the process never started) and any error from launching or
	// waiting for it.
	Run(ctx context.Context, path string, args []string, stdout, stderr io.Writer) (exitCode int, err error)
}
