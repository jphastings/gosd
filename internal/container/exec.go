package container

import (
	"context"
	"errors"
	"io"
	"os/exec"
)

// realExec is the production execRunner, backed by os/exec. exec.Command is
// portable (unlike gosd-init's netlink/DHCP platform seams), so unlike
// netup/wifiup this package needs no //go:build-tagged platform files.
type realExec struct{}

func (realExec) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// Run streams stdout/stderr live because os/exec copies from the child
// process to cmd.Stdout/cmd.Stderr as data arrives, rather than buffering
// until the process exits.
func (realExec) Run(ctx context.Context, path string, args []string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), err
	}
	return -1, err
}
