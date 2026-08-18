// Package boot implements the gosd-init boot sequence: early mounts,
// console logging, the boot partition mount retry, and /app
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

import (
	"io"
	"syscall"
)

// Mounter mounts and unmounts a single filesystem, mirroring the Linux
// mount(2)/umount(2) syscall signatures so the real implementation is a thin
// wrapper around golang.org/x/sys/unix. Unmount is only needed to reverse a
// mount that turns out to be wrong after the fact — see
// MountBootPartition's boot-partition sentinel check.
type Mounter interface {
	Mount(source, target, fstype string, flags uintptr, data string) error
	Unmount(target string) error
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

// ExitStatus is everything the reaper can tell us about how a reaped child
// died. unix.WaitStatus.ExitStatus() alone can't tell a signal death apart
// from "hasn't exited" — both read -1 — which is fine for a consumer that
// only ever logs a bare exit code (cloudflared, tsfunnel), but not for
// /app's own supervision: a crash report has to name a signal death in
// human terms ("ran out of memory", not "signal 9" — gosd-s9uq), which needs
// the fuller picture.
type ExitStatus struct {
	// ExitCode is the process's exit(2) code when it exited normally, or -1
	// when it was killed by a signal instead — exactly what
	// unix.WaitStatus.ExitStatus() returns, so a caller that only wants
	// what Reaper.Wait has always reported sees no change in this field
	// (see cmd/gosd-init/main.go's exitCodeOnly).
	ExitCode int
	// Signaled reports whether the child was killed by a signal rather than
	// exiting (via exit(2), a return from main, or an uncaught Go panic,
	// which itself exits via exit(2)) on its own.
	Signaled bool
	// Signal is the signal that killed the child. Only meaningful when
	// Signaled is true.
	Signal syscall.Signal
}

// Reaper reaps every child reparented to PID 1 (via SIGCHLD + wait4), and
// reports the exit status of specifically-awaited pids back to their
// callers. Pids nobody is waiting for (grandchildren orphaned to PID 1) are
// reaped and discarded internally.
type Reaper interface {
	Wait(pid int) (ExitStatus, error)
}

// StatusLED drives the board's onboard status LED through its three boot
// states, for a board that has one wired at all — see
// cmd/gosd-init/internal/statusled for how it discovers which sysfs LED to
// use and why the kernel's own "timer" trigger does the actual blinking,
// never a goroutine (that's what lets the blink survive the very halt or
// wedge it's reporting on). A nil StatusLED (Deps.StatusLED left unset) must
// be a silent no-op everywhere it's called — qemu-virt has no LED at all,
// so CI can never exercise the blink end-to-end, and none of boot's own
// tests wire one up unless they're specifically testing this.
type StatusLED interface {
	Booting() error
	Running() error
	Fatal() error
}

// StatusLEDDescriber is an optional companion to StatusLED: when the wired
// implementation also provides it, Run logs its description once, right
// after the first state is applied (the earliest point at which the sysfs
// implementation's lazy discovery has actually run). Optional, so no
// existing fake has to grow a method it doesn't care about, and a nil
// StatusLED stays a silent no-op — a type assertion on a nil interface
// simply doesn't match.
//
// This one line matters more than its size suggests: gosd-init has no
// shell, no SSH and no remote debug, so the console is the only way to tell
// an empty /sys/class/leds apart from a candidate filter that rejected
// every entry, or from having picked the wrong LED. Guessing between those
// costs a reflash cycle each (bean gosd-ddz6).
type StatusLEDDescriber interface {
	Describe() string
}

// Rebooter performs the fatal-error shutdown paths: flush disks and the
// console, then either restart the machine (transient failures, where a
// retry may succeed) or halt it (states no retry can improve, like a
// corrupt data partition — see Deps.FaultReport). The 5s pause before a
// reboot is a plain time.Sleep, injected separately (see Deps.Sleep) so it
// can be faked in tests.
type Rebooter interface {
	Sync()
	// FlushConsole blocks until every byte already written to the console
	// has actually left the machine, so the crash report gosd-init just
	// logged there survives the Reboot/Halt call that follows it
	// (gosd-fs34). A write(2) to a tty returns once the kernel has queued
	// the bytes for a slow serial line, not once they've been physically
	// transmitted; unix.Reboot cuts that queue off without warning, which
	// is why a card plugged into a computer showed the complete report
	// while a cable attached to the same crash showed none of it. Called
	// on every path that halts or reboots, alongside Sync — a console
	// that isn't a real serial line (a fake, or one that was never opened)
	// has nothing to drain and nothing useful to do about a failure here,
	// so this is best-effort, like Sync.
	FlushConsole()
	Reboot()
	Halt()
}
