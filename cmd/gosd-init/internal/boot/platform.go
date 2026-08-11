package boot

import (
	"io"
	"time"
)

// Platform bundles the real implementations of every syscall-touching
// dependency Run needs. NewPlatform is implemented once per build tag
// (platform_linux.go, platform_other.go) so main.go can wire it up without
// caring which OS it's running on.
type Platform struct {
	Mounter    Mounter
	Hostname   HostnameSetter
	AppStarter AppStarter
	Reaper     Reaper
	Rebooter   Rebooter

	OpenConsole func() (io.WriteCloser, error)

	// IgnoreShutdownSignals makes SIGTERM/SIGINT no-ops: PID 1 must not
	// die from them.
	IgnoreShutdownSignals func()

	// WriteFatalReport records a rendered crash report as
	// LAST_FATAL_ERROR.md at the root of the boot partition mounted
	// (read-only) at target, briefly remounting it read-write to do so.
	WriteFatalReport func(target, body string) error

	// RemoveBootFiles deletes names from the root of the boot partition
	// mounted (read-only) at target, briefly remounting it read-write.
	// Callers establish first that at least one of them is there, so that
	// a device with nothing to clean up never remounts (see
	// FaultReportDeps.Exists).
	RemoveBootFiles func(target string, names []string) error

	// WriteBootFile durably writes name at the root of the boot
	// partition mounted (read-only) at target, briefly remounting it
	// read-write, and restores the read-only mount afterwards. Unlike
	// WriteFatalReport this happens on a device that carries on booting,
	// so the restoring remount is part of the result, not best-effort.
	WriteBootFile func(target, name string, data []byte) error

	// DeviceModel returns the hardware's own self-description from the
	// device tree, or "" when it isn't readable.
	DeviceModel func() string

	// Uptime reports how long the machine has been up, and whether that
	// could be determined at all.
	Uptime func() (time.Duration, bool)
}
