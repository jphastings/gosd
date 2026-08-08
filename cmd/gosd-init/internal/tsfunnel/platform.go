package tsfunnel

import (
	"io"
	"os/exec"
)

// StartProcess is the real, os/exec-backed implementation of Deps.StartProcess:
// it launches path with args and env, directs its stdout/stderr, and returns
// its pid without waiting for it to exit — exit status is collected
// separately, through Deps.Wait (see that field's doc comment for why it must
// never be exec.Cmd.Wait).
//
// Unlike boot.AppStarter (used only for /app, started with no arguments at
// all), the shim needs an argv, which is why this package has its own
// StartProcess seam rather than sharing boot's, mirroring
// cloudflared.StartProcess exactly. Starting a process needs no
// Linux-specific syscall the way mount/sethostname/reboot do, so this needs
// no "linux" build tag at all: it compiles, and genuinely runs, on macOS
// too.
func StartProcess(path string, args, env []string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command(path, args...)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}
