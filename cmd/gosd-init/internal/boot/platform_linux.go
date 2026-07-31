//go:build linux

package boot

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/provsnapshot"
)

// NewPlatform wires up the real, Linux-syscall-backed implementations of
// every Deps interface.
func NewPlatform() *Platform {
	return &Platform{
		Mounter:               linuxMounter{},
		Hostname:              linuxHostname{},
		AppStarter:            linuxAppStarter{},
		Reaper:                newLinuxReaper(),
		Rebooter:              linuxRebooter{},
		OpenConsole:           openConsole,
		IgnoreShutdownSignals: ignoreShutdownSignals,
		WriteBootFailure:      writeBootFailure,
		WriteBootFile:         writeBootFile,
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

func (linuxRebooter) Halt() {
	// The kernel reads the command as a u32 bit pattern, but unix.Reboot
	// takes an int: on 32-bit ARM (pi-zero-w) CMD_HALT's high bit overflows
	// a direct int conversion, so reinterpret it through uint32→int32.
	cmd := uint32(unix.LINUX_REBOOT_CMD_HALT)
	// Best-effort, same as Reboot.
	_ = unix.Reboot(int(int32(cmd)))
}

// writeBootFailure records msg as boot-failure.log at the root of the
// (normally read-only) GOSD-BOOT partition mounted at target: remount
// read-write, overwrite the file, sync it, and remount read-only again.
// Overwriting is deliberate — the file always describes the latest run's
// fatal issue, which is the one whoever collects the device needs. The
// restoring remount is best-effort: every caller halts the machine next.
func writeBootFailure(target, msg string) error {
	if err := unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_NOSUID, ""); err != nil {
		return fmt.Errorf("remounting %s read-write: %w", target, err)
	}
	defer func() { _ = unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NOSUID, "") }()

	f, err := os.OpenFile(filepath.Join(target, "boot-failure.log"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(msg); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// writeBootFile durably writes name at the root of the read-only GOSD-BOOT
// partition mounted at target: remount read-write, write through the
// four-step durable sequence FAT needs (see provsnapshot.WriteFileDurably),
// then remount read-only again. The device keeps running afterwards — this
// is the provisioning self-heal's write-back, not a last gasp — so failing
// to restore the read-only mount is reported rather than swallowed: leaving
// GOSD-BOOT writable under a live app is exactly the exposure the read-only
// mount exists to prevent.
func writeBootFile(target, name string, data []byte) error {
	if err := unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_NOSUID, ""); err != nil {
		return fmt.Errorf("remounting %s read-write: %w", target, err)
	}

	writeErr := provsnapshot.WriteFileDurably(filepath.Join(target, name), data)

	if err := unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NOSUID, ""); err != nil {
		if writeErr != nil {
			return fmt.Errorf("writing %s: %w (and remounting %s read-only afterwards also failed: %v)", name, writeErr, target, err)
		}
		return fmt.Errorf("remounting %s read-only after writing %s: %w", target, name, err)
	}
	return writeErr
}

func openConsole() (io.WriteCloser, error) {
	return os.OpenFile("/dev/console", os.O_WRONLY, 0)
}

// ignoreShutdownSignals makes SIGTERM/SIGINT no-ops: PID 1 must not die
// from them.
func ignoreShutdownSignals() {
	signal.Ignore(syscall.SIGTERM, syscall.SIGINT)
}

type linuxAppStarter struct{}

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
	return cmd.Process.Pid, nil
}

// newLinuxReaper starts the SIGCHLD-driven wait4(-1, ...) loop that reaps
// every child reparented to PID 1 — /app and any double-forked grandchildren
// — through one path, avoiding the classic races between os/exec's own
// reaping and a PID-1-wide reaper.
func newLinuxReaper() *reaper {
	r := newReaper()
	sigchld := make(chan os.Signal, 1)
	signal.Notify(sigchld, syscall.SIGCHLD)
	go r.loop(sigchld)
	return r
}

func (r *reaper) loop(sigchld <-chan os.Signal) {
	for range sigchld {
		r.drain()
	}
}

// drain reaps every child currently waitable, without blocking, so that
// signals coalesced while gosd-init was busy don't leave zombies behind.
func (r *reaper) drain() {
	for {
		var ws unix.WaitStatus
		pid, err := unix.Wait4(-1, &ws, unix.WNOHANG, nil)
		if err != nil || pid <= 0 {
			return
		}
		r.deliver(pid, ws.ExitStatus())
	}
}
