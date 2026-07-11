package container

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
)

// fakeExec is an execRunner that never spawns a real process, so
// detection- and arg-construction tests run on macOS with no container
// runtime installed, following the same fake-driven pattern as gosd-init's
// platform seams.
type fakeExec struct {
	mu sync.Mutex

	// paths maps a binary name to the path LookPath should resolve it
	// to; a name absent from paths looks not-installed.
	paths map[string]string

	// runFn, if set, is invoked for every Run call (including `info`
	// liveness checks) and controls its outcome.
	runFn func(path string, args []string, stdout, stderr io.Writer) (int, error)

	calls []fakeCall
}

type fakeCall struct {
	path string
	args []string
}

func newFakeExec(paths map[string]string) *fakeExec {
	return &fakeExec{paths: paths}
}

func (f *fakeExec) LookPath(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.paths[name]; ok {
		return p, nil
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

func (f *fakeExec) Run(_ context.Context, path string, args []string, stdout, stderr io.Writer) (int, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fakeCall{path: path, args: append([]string(nil), args...)})
	fn := f.runFn
	f.mu.Unlock()

	if fn == nil {
		return 0, nil
	}
	return fn(path, args, stdout, stderr)
}

func (f *fakeExec) lastCall() fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

func (f *fakeExec) calledWith(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c.path == path {
			return true
		}
	}
	return false
}

// errDaemonUnreachable is a stand-in for the error `docker info`/`podman
// info` returns (via exec.ExitError in reality) when the daemon isn't
// running.
var errDaemonUnreachable = errors.New("cannot connect to the Docker daemon")
