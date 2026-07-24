//go:build linux

package boot

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// NewPlatform wires up the real, Linux-syscall-backed implementations of
// every Deps interface.
func NewPlatform() *Platform {
	reaper := newLinuxReaper()
	return &Platform{
		Mounter:               linuxMounter{},
		Hostname:              linuxHostname{},
		AppStarter:            linuxAppStarter{reaper: reaper},
		Reaper:                reaper,
		Rebooter:              linuxRebooter{},
		OpenConsole:           openConsole,
		IgnoreShutdownSignals: ignoreShutdownSignals,
	}
}

type linuxMounter struct{}

func (linuxMounter) Mount(source, target, fstype string, flags uintptr, data string) error {
	return unix.Mount(source, target, fstype, flags, data)
}

func (linuxMounter) Unmount(target string) error {
	return unix.Unmount(target, 0)
}

type linuxHostname struct{}

func (linuxHostname) SetHostname(name string) error {
	return unix.Sethostname([]byte(name))
}

type linuxRebooter struct{}

func (linuxRebooter) Sync() { unix.Sync() }

func (linuxRebooter) Reboot() {
	// Best-effort: if this fails there is nothing more gosd-init can do.
	_ = unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
}

func openConsole() (io.WriteCloser, error) {
	return os.OpenFile("/dev/console", os.O_WRONLY, 0)
}

// ignoreShutdownSignals makes SIGTERM/SIGINT no-ops: PID 1 must not die
// from them.
func ignoreShutdownSignals() {
	signal.Ignore(syscall.SIGTERM, syscall.SIGINT)
}

// linuxAppStarter starts /app and immediately tells reaper to expect its
// pid, so an exit that races with the caller's later Wait call is still
// attributed correctly instead of being discarded as an unrelated
// grandchild.
type linuxAppStarter struct {
	reaper *linuxReaper
}

// Start launches path as a child process and returns its pid without
// waiting for it: as PID 1, exit status is collected by linuxReaper's
// central wait4 loop instead, so a single reaper handles /app and any
// grandchildren reparented to us.
func (s linuxAppStarter) Start(path string, env []string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command(path)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	s.reaper.expect(pid)
	return pid, nil
}

// linuxReaper reaps every child reparented to PID 1 through a single
// SIGCHLD-driven wait4(-1, ...) loop, and delivers the exit status of pids
// someone is waiting for back to them. Pids nobody is waiting for
// (double-forked grandchildren orphaned to PID 1) are reaped and discarded.
//
// A pid can be reaped before Wait is called for it (the child exits in the
// narrow window between Start returning and the caller registering
// interest), so expect marks pids we know we'll be asked about; deliver
// stashes their result until Wait claims it instead of discarding it.
type linuxReaper struct {
	mu       sync.Mutex
	waiters  map[int]chan waitResult
	expected map[int]bool
	results  map[int]waitResult
}

// waitResult carries only an exit status, not an error: once wait4 has
// confirmed a pid is reaped (the only way deliver is ever called), getting
// its exit status cannot itself fail. Reaper.Wait still returns an error to
// satisfy the general interface fakes use in tests, but the real
// implementation always returns nil for it.
type waitResult struct {
	status int
}

func newLinuxReaper() *linuxReaper {
	r := &linuxReaper{
		waiters:  make(map[int]chan waitResult),
		expected: make(map[int]bool),
		results:  make(map[int]waitResult),
	}
	sigchld := make(chan os.Signal, 1)
	signal.Notify(sigchld, syscall.SIGCHLD)
	go r.loop(sigchld)
	return r
}

func (r *linuxReaper) loop(sigchld <-chan os.Signal) {
	for range sigchld {
		r.drain()
	}
}

// drain reaps every child currently waitable, without blocking, so that
// signals coalesced while gosd-init was busy don't leave zombies behind.
func (r *linuxReaper) drain() {
	for {
		var ws unix.WaitStatus
		pid, err := unix.Wait4(-1, &ws, unix.WNOHANG, nil)
		if err != nil || pid <= 0 {
			return
		}
		r.deliver(pid, ws.ExitStatus())
	}
}

func (r *linuxReaper) expect(pid int) {
	r.mu.Lock()
	r.expected[pid] = true
	r.mu.Unlock()
}

func (r *linuxReaper) deliver(pid, status int) {
	r.mu.Lock()
	ch, waiting := r.waiters[pid]
	if waiting {
		delete(r.waiters, pid)
		delete(r.expected, pid)
	} else if r.expected[pid] {
		r.results[pid] = waitResult{status: status}
		delete(r.expected, pid)
	}
	r.mu.Unlock()

	if waiting {
		ch <- waitResult{status: status}
	}
	// else: either stashed above for a not-yet-awaited expected pid, or an
	// unrelated (grandchild) pid that's already reaped and can be
	// discarded.
}

func (r *linuxReaper) Wait(pid int) (int, error) {
	r.mu.Lock()
	if res, ok := r.results[pid]; ok {
		delete(r.results, pid)
		r.mu.Unlock()
		return res.status, nil
	}
	ch := make(chan waitResult, 1)
	r.waiters[pid] = ch
	r.mu.Unlock()

	res := <-ch
	return res.status, nil
}
