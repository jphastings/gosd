// Package boot implements the gosd-init boot sequence: early mounts,
// console logging, the GOSD-BOOT partition mount retry, and /app
// supervision with restart backoff and zombie reaping.
//
// The sequencing and decision logic in this package (Run, Supervisor,
// Backoff, MountBootPartition) takes every syscall-touching dependency as a
// thin interface, so it has no build tags and is fully unit-testable with
// fakes on any OS. The real implementations of those interfaces, which do
// touch Linux syscalls (mount, sethostname, wait4, reboot, /dev/console),
// live in platform_linux.go behind a "linux" build tag; platform_other.go
// provides stub implementations so the package still builds on non-Linux
// hosts (required for `go test ./...` to pass on macOS).
package boot

import "io"

// Mounter mounts a single filesystem, mirroring the Linux mount(2) syscall
// signature so the real implementation is a thin wrapper around
// golang.org/x/sys/unix.Mount.
type Mounter interface {
	Mount(source, target, fstype string, flags uintptr, data string) error
}

// HostnameSetter sets the kernel hostname (sethostname(2)).
type HostnameSetter interface {
	SetHostname(name string) error
}

// AppStarter starts /app as a child process with the given environment and
// stdout/stderr destinations, returning its pid. It must not wait for the
// process to exit: as PID 1, gosd-init reaps children (including this one)
// through Reaper, not through the standard library's process-wait path, so
// that grandchildren reparented to PID 1 are reaped too.
type AppStarter interface {
	Start(path string, env []string, stdout, stderr io.Writer) (pid int, err error)
}

// Reaper reaps every child reparented to PID 1 (via SIGCHLD + wait4), and
// reports the exit status of specifically-awaited pids back to their
// callers. Pids nobody is waiting for (grandchildren orphaned to PID 1) are
// reaped and discarded internally.
type Reaper interface {
	Wait(pid int) (status int, err error)
}

// Rebooter performs the fatal-error shutdown path: flush disks and restart
// the machine. The 5s pause between them is a plain time.Sleep, injected
// separately (see Deps.Sleep) so it can be faked in tests.
type Rebooter interface {
	Sync()
	Reboot()
}
