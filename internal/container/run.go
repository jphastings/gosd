package container

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
)

// Mount is a host directory bind-mounted into the container.
type Mount struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// RunSpec describes a single container invocation.
type RunSpec struct {
	// Image is the image reference to run, normally container.KernelBuildImage.
	Image string
	// Env is the container's environment, as a map since build callers
	// assemble it from named values rather than caring about order.
	Env map[string]string
	// Mounts are host directories bind-mounted into the container.
	Mounts []Mount
	// WorkDir sets the container's working directory. Empty leaves the
	// image's default.
	WorkDir string
	// Cmd is the command (and its arguments) to run inside the
	// container, e.g. []string{"make", "-j8", "Image"}.
	Cmd []string

	// Stdout and Stderr receive the container's output live, as it's
	// produced — required for kernel builds, which run 20-60 minutes
	// and need visible progress rather than a wall of output at the
	// end. Either may be nil to discard that stream.
	Stdout io.Writer
	Stderr io.Writer
}

// tailStderrBytes caps how much of a failed run's stderr RunFailedError
// quotes, so a 20-60 minute build's error doesn't dump its entire log.
const tailStderrBytes = 4096

// Run runs spec.Cmd inside a container of spec.Image, streaming output live
// to spec.Stdout/spec.Stderr. On a non-zero container exit it returns a
// *RunFailedError carrying the exit code and the tail of stderr.
func (r *Runtime) Run(ctx context.Context, spec RunSpec) error {
	stdout := spec.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := spec.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	tail := &tailBuffer{limit: tailStderrBytes}
	teeStderr := io.MultiWriter(stderr, tail)

	args := buildRunArgs(spec)
	exitCode, err := r.exec.Run(ctx, r.binary, args, stdout, teeStderr)
	if err == nil {
		return nil
	}
	if exitCode < 0 {
		return fmt.Errorf("launching %s: %w", r.binary, err)
	}
	return &RunFailedError{
		Runtime:    r.name,
		Image:      spec.Image,
		ExitCode:   exitCode,
		StderrTail: tail.String(),
	}
}

// buildRunArgs assembles the `<runtime> run` argument list. Podman's CLI is
// deliberately docker-compatible for every flag used here, so the same
// argument list works unmodified for both engines — only the binary name
// (r.binary) differs.
func buildRunArgs(spec RunSpec) []string {
	args := []string{"run", "--rm"}

	keys := make([]string, 0, len(spec.Env))
	for k := range spec.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, spec.Env[k]))
	}

	for _, m := range spec.Mounts {
		v := fmt.Sprintf("%s:%s", m.HostPath, m.ContainerPath)
		if m.ReadOnly {
			v += ":ro"
		}
		args = append(args, "-v", v)
	}

	if spec.WorkDir != "" {
		args = append(args, "-w", spec.WorkDir)
	}

	args = append(args, spec.Image)
	args = append(args, spec.Cmd...)
	return args
}

// tailBuffer keeps only the most recent limit bytes written to it, so
// capturing a failed run's stderr for RunFailedError can't grow unbounded
// across a long build.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.limit {
		t.buf = t.buf[len(t.buf)-t.limit:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
