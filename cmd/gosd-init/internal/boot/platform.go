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
}
