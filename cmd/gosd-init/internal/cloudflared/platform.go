package cloudflared

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// StartProcess is the real, os/exec-backed implementation of Deps.StartProcess:
// it launches path with args and env, directs its stdout/stderr, and returns
// its pid without waiting for it to exit — exit status is collected
// separately, through Deps.Wait (see that field's doc comment for why it must
// never be exec.Cmd.Wait).
//
// Unlike boot.AppStarter (used only for /app, started with no arguments at
// all), cloudflared needs an argv, which is why this package has its own
// StartProcess seam rather than sharing boot's. Starting a process needs no
// Linux-specific syscall the way mount/sethostname/reboot do, so — like
// mdnsresponder's pion/mdns-backed server (see that package's doc comment) —
// this needs no "linux" build tag at all: it compiles, and genuinely runs,
// on macOS too.
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

// Kill is the real implementation of Deps.Kill: it sends pid a SIGTERM,
// asking cloudflared to shut down on its own terms. Used only for a
// deliberate restart (Deps.RestartSignal firing, see supervise) — a network
// blip or crash that ends cloudflared on its own is observed entirely
// through Deps.Wait, never this. Like StartProcess, this needs no "linux"
// build tag: os.Process.Signal(syscall.SIGTERM) works identically on macOS,
// so it compiles and runs in tests there too.
func Kill(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding pid %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signaling pid %d: %w", pid, err)
	}
	return nil
}
