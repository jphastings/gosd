package boot

import (
	"io"
	"time"

	"github.com/jphastings/gosd/internal/redact"
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

	// EditBootPartition runs edit against the root of the boot partition
	// mounted (read-only) at target, with it briefly remounted read-write
	// and everything edit wrote flushed to the card before the read-only
	// mount is restored. Unlike WriteFatalReport this happens on a device
	// that carries on booting, so the restoring remount is part of the
	// result, not best-effort.
	EditBootPartition func(target string, edit func(root string) error) error

	// DeviceModel returns the hardware's own self-description from the
	// device tree, or "" when it isn't readable.
	DeviceModel func() string

	// Uptime reports how long the machine has been up, and whether that
	// could be determined at all.
	Uptime func() (time.Duration, bool)

	// RegisteredSecrets reads the /run registration file
	// fault.RegisterSecretString writes (see internal/secretreg), or nil
	// when there's nothing registered or nothing readable.
	RegisteredSecrets func() []redact.Rule
}
