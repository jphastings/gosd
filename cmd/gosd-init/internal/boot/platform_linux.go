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
	"time"

	"golang.org/x/sys/unix"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/provsnapshot"
	"github.com/jphastings/gosd/internal/faultreport"
	"github.com/jphastings/gosd/internal/redact"
	"github.com/jphastings/gosd/internal/secretreg"
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
		WriteFatalReport:      writeFatalReport,
		RemoveBootFiles:       removeBootFiles,
		WriteBootFile:         writeBootFile,
		DeviceModel:           deviceModel,
		Uptime:                uptime,
		RegisteredSecrets:     registeredSecrets,
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

// FlushConsole waits for every byte already written to /dev/console to
// actually be transmitted, so a Reboot or Halt that follows immediately
// after can't cut it off mid-flight (gosd-fs34). It opens its own handle
// rather than reusing Run's: the output queue TCSBRK drains belongs to the
// underlying tty device, not to any one file descriptor, so a fresh open
// drains exactly the same queue gosd-init's own logger has been writing
// into all boot. Best-effort, like Sync: a console that was never a real
// serial line — a framebuffer console, a qemu virtio console with no
// backing UART, one that failed to open in the first place — has nothing
// to drain, and there's nothing more useful to do about either failure
// than continue toward the reboot/halt that's already been decided.
func (linuxRebooter) FlushConsole() {
	f, err := os.OpenFile("/dev/console", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	// TCSBRK with a nonzero argument is glibc's tcdrain(3) on Linux: wait
	// for previously-written output to finish transmitting, without
	// sending an actual break (see tty_ioctl(4)'s TCSBRK entry — a zero
	// argument is the break-sending form this deliberately avoids).
	_ = unix.IoctlSetInt(int(f.Fd()), unix.TCSBRK, 1)
}

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

// writeFatalReport records body as LAST_FATAL_ERROR.md at the root of the
// (normally read-only) boot partition mounted at target: remount
// read-write, overwrite the file, sync it, and remount read-only again.
// Overwriting is deliberate — the file always describes the latest fatal
// issue, which is the one whoever collects the device needs. The restoring
// remount is best-effort: every caller halts or reboots the machine next.
//
// Unlike every other on-disk commit in this codebase there is no
// write → sync → marker → sync record here, and that is deliberate: nothing
// ever adopts this file as state. It is read by a human, who can see for
// themselves that it stops mid-sentence — whereas a marker would need a
// second write in the very window the whole design is trying to keep short.
func writeFatalReport(target, body string) error {
	if err := unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_NOSUID, ""); err != nil {
		return fmt.Errorf("remounting %s read-write: %w", target, err)
	}
	defer func() { _ = unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NOSUID, "") }()

	f, err := os.OpenFile(filepath.Join(target, faultreport.FileName), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// removeBootFiles deletes names from the root of the boot partition mounted
// at target, briefly remounting it read-write. Unlike writeFatalReport this
// runs on a device that carries on booting — it's how a recovered device
// stops looking broken — so, like writeBootFile, failing to restore the
// read-only mount is reported rather than swallowed: leaving the boot
// partition writable under a live app is exactly the exposure the read-only
// mount exists to prevent.
//
// A name that isn't there is not an error: the caller already checked, and
// racing itself is not worth failing a cleanup over.
func removeBootFiles(target string, names []string) error {
	if err := unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_NOSUID, ""); err != nil {
		return fmt.Errorf("remounting %s read-write: %w", target, err)
	}

	var removeErr error
	for _, name := range names {
		if err := os.Remove(filepath.Join(target, name)); err != nil && !os.IsNotExist(err) && removeErr == nil {
			removeErr = fmt.Errorf("deleting %s: %w", name, err)
		}
	}
	syncFS(target)

	if err := unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NOSUID, ""); err != nil {
		if removeErr != nil {
			return fmt.Errorf("%w (and remounting %s read-only afterwards also failed: %v)", removeErr, target, err)
		}
		return fmt.Errorf("remounting %s read-only after deleting %v: %w", target, names, err)
	}
	return removeErr
}

// syncFS flushes the boot partition before the read-only remount, so a
// deletion reaches the card rather than merely being promised. The kernel
// syncs a filesystem on its way to read-only anyway; this doesn't lean on
// that, and it's best-effort — there is nothing useful to do about a
// mountpoint that won't open.
func syncFS(target string) {
	dir, err := os.Open(target)
	if err != nil {
		return
	}
	_ = unix.Syncfs(int(dir.Fd()))
	_ = dir.Close()
}

// deviceModelPath is the hardware's own self-description, written by the
// firmware from the DTB. /proc/device-tree/model is the same file by another
// name; this one is used because gosd-init mounts /sys itself.
const deviceModelPath = "/sys/firmware/devicetree/base/model"

// deviceModel returns the board's device-tree model string, or "" when
// there's no device tree (a kernel built without CONFIG_OF, an
// x86 host) or it can't be read. Never an error: a crash report with an
// unknown device name is worth far more than no crash report.
func deviceModel() string {
	data, err := os.ReadFile(deviceModelPath)
	if err != nil {
		return ""
	}
	return parseDeviceModel(data)
}

// uptime reports how long the machine has been up, from /proc/uptime — the
// kernel's own monotonic count, which is why the report can state it as fact
// while refusing to state the wall clock. Reports false when /proc isn't
// mounted or the file doesn't parse.
func uptime() (time.Duration, bool) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, false
	}
	return parseUptime(string(data))
}

// registeredSecrets reads secretreg.Path fresh, at report time, so a
// fault.RegisterSecretString call moments before a panic still redacts.
// Stat first rather than reading straight through: a file bigger than
// secretreg.MaxTotalBytes is never trusted (see secretreg.Parse), so there
// is no reason to read it into memory at all. Any failure — missing file
// (the common case: nothing has ever registered), oversized, unreadable —
// yields no rules, never an error; see secretreg's doc for why an
// untrustworthy file is dropped rather than partially believed.
func registeredSecrets() []redact.Rule {
	info, err := os.Stat(secretreg.Path)
	if err != nil || info.Size() > secretreg.MaxTotalBytes {
		return nil
	}
	data, err := os.ReadFile(secretreg.Path)
	if err != nil {
		return nil
	}
	return secretreg.Parse(data)
}

// writeBootFile durably writes name at the root of the read-only boot
// partition mounted at target: remount read-write, write through the
// four-step durable sequence FAT needs (see provsnapshot.WriteFileDurably),
// then remount read-only again. The device keeps running afterwards — this
// is the provisioning self-heal's write-back, not a last gasp — so failing
// to restore the read-only mount is reported rather than swallowed: leaving
// the boot partition writable under a live app is exactly the exposure the read-only
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
		r.deliver(pid, ExitStatus{
			ExitCode: ws.ExitStatus(),
			Signaled: ws.Signaled(),
			Signal:   ws.Signal(),
		})
	}
}
