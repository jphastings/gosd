package boot

import "io"

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

	// WriteBootFailure records a fatal failure as boot-failure.log at the
	// root of the GOSD-BOOT partition mounted (read-only) at target,
	// briefly remounting it read-write to do so.
	WriteBootFailure func(target, msg string) error

	// WriteBootFile durably writes name at the root of the GOSD-BOOT
	// partition mounted (read-only) at target, briefly remounting it
	// read-write, and restores the read-only mount afterwards. Unlike
	// WriteBootFailure this happens on a device that carries on booting,
	// so the restoring remount is part of the result, not best-effort.
	WriteBootFile func(target, name string, data []byte) error
}
