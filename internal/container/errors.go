package container

import "fmt"

// NotInstalledError means no usable container runtime CLI was found: either
// neither docker nor podman is on $PATH (auto-detection), or the explicitly
// requested one (Preferred) isn't. It's a distinct type from DaemonDownError
// so callers can tell "not installed" from "installed but not running"
// apart (e.g. to skip a smoke test only in the former case) and so both
// produce different, actionable guidance.
type NotInstalledError struct {
	// Preferred is the specific runtime name that was requested and not
	// found, or "" if this came from auto-detecting both docker and
	// podman.
	Preferred string
}

func (e *NotInstalledError) Error() string {
	if e.Preferred != "" {
		return fmt.Sprintf(
			"gosd build-kernel needs %s (explicitly requested) but it wasn't found on $PATH; install %s, then re-run",
			e.Preferred, installHint(e.Preferred),
		)
	}
	return "gosd build-kernel needs Docker or Podman; install Docker Desktop (https://docs.docker.com/desktop/), colima (https://colima.run/) or podman (https://podman.io/docs/installation), then re-run"
}

func installHint(runtime string) string {
	switch runtime {
	case RuntimeDocker:
		return "Docker Desktop (https://docs.docker.com/desktop/) or colima (https://colima.run/)"
	case RuntimePodman:
		return "podman (https://podman.io/docs/installation)"
	default:
		return runtime
	}
}

// DaemonDownError means the runtime CLI was found on $PATH but its daemon
// didn't respond to a liveness check (e.g. `docker info`). This is common
// with Docker Desktop/colima/a podman machine that's installed but not
// currently started.
type DaemonDownError struct {
	Runtime string
	// Err is the underlying error from the liveness check (e.g. the
	// `docker info` invocation), if any.
	Err error
}

func (e *DaemonDownError) Error() string {
	return fmt.Sprintf("gosd build-kernel found %s but its daemon isn't responding; %s, then re-run", e.Runtime, daemonStartHint(e.Runtime))
}

func (e *DaemonDownError) Unwrap() error { return e.Err }

func daemonStartHint(runtime string) string {
	switch runtime {
	case RuntimeDocker:
		return "start Docker (open Docker Desktop, run `colima start`, or `systemctl start docker`, depending on how it's installed)"
	case RuntimePodman:
		return "start podman (`podman machine start` on macOS, or `systemctl start podman` on Linux)"
	default:
		return "start its daemon/service"
	}
}

// RunFailedError means the container ran but the command inside it exited
// non-zero. StderrTail is the last portion of the container's stderr,
// captured while it was being streamed live, to help diagnose the failure
// without having to scroll back through a 20-60 minute build's full log.
type RunFailedError struct {
	Runtime    string
	Image      string
	ExitCode   int
	StderrTail string
}

func (e *RunFailedError) Error() string {
	msg := fmt.Sprintf("%s run of %s failed with exit code %d", e.Runtime, e.Image, e.ExitCode)
	if e.StderrTail == "" {
		return msg + " (no stderr output captured)"
	}
	return fmt.Sprintf("%s; last stderr output:\n%s", msg, e.StderrTail)
}
