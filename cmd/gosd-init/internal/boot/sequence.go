package boot

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/initcfg"
)

// Deps bundles every side-effecting dependency the boot sequence needs.
// Production wiring (main.go) supplies the real, Linux-syscall-backed
// implementations from Platform; tests supply fakes.
type Deps struct {
	Mounter    Mounter
	Hostname   HostnameSetter
	AppStarter AppStarter
	Reaper     Reaper
	Rebooter   Rebooter

	// OpenConsole opens /dev/console for logging (step 2). If it fails,
	// gosd-init falls back to logging on FallbackLog and continues:
	// losing the console is bad, but not fatal on its own.
	OpenConsole func() (io.WriteCloser, error)
	// FallbackLog is used for anything logged before the console is open
	// (or if opening it fails outright).
	FallbackLog func(format string, args ...any)

	// ReadConfig reads and parses /etc/gosd/config.json. It's baked into
	// the initramfs itself, so it doesn't actually depend on any mount
	// having happened — but Run still calls it at the point the locked
	// boot sequence specifies (step 3), not before.
	ReadConfig func() (initcfg.Config, error)
	// ReadCmdline reads and parses the kernel command line for the
	// gosd.board / gosd.debug overrides. Unlike ReadConfig, this DOES
	// require /proc to be mounted (it reads /proc/cmdline), which is why
	// Run only calls it after mountEarly has succeeded: calling it any
	// earlier would silently and permanently disable both overrides on
	// real hardware, where /proc isn't mounted until step 1 runs.
	ReadCmdline func() (initcfg.CmdlineArgs, error)

	// ReadGosdToml reads and parses /boot/gosd.toml, the hand-editable
	// fallback config on the GOSD-BOOT partition. It's nil-checked (like
	// StartNetworking) rather than required, so tests that don't care
	// about gosd.toml can leave it unset. Unlike ReadConfig, this can only
	// be called after the GOSD-BOOT partition is mounted (step 5), which
	// is why Run calls it right after MountBootPartition succeeds — and
	// why the hostname it may override has to be re-applied there too,
	// even though step 4 already applied config.json's value.
	ReadGosdToml func() (gosdtoml.Config, error)

	Sleep func(time.Duration)
	Now   func() time.Time

	// StartNetworking, if non-nil, is called in its own goroutine
	// immediately before /app supervision begins, and is passed the
	// fully-resolved config (cmdline overrides already applied), the
	// parsed gosd.toml (zero value if absent, unreadable, or garbage) so
	// wifiup can prefer its wifi block over cfg's, and Run's current
	// logger (the console, if opening it succeeded) so its output goes to
	// the same place as the rest of gosd-init's. Networking (link up,
	// DHCP, DNS, WiFi) must never block or delay /app's start, so Run
	// doesn't wait for it and doesn't know or care what it does beyond
	// that; production wires this to start both netup.Run (wired) and
	// wifiup.Run (WiFi), tests leave it nil.
	StartNetworking func(cfg initcfg.Config, gosdToml gosdtoml.Config, log func(format string, args ...any))
}

// Options holds the per-boot paths the sequence acts on.
type Options struct {
	AppPath string

	BootTarget  string
	BootDevices []string
	BootTimeout time.Duration

	// Stop, if non-nil, ends app supervision when closed. Production
	// leaves this nil so supervision runs forever, as PID 1 must; tests
	// set it to bound the otherwise-infinite supervise loop.
	Stop <-chan struct{}
}

// Run executes the locked gosd-init boot sequence: early mounts, console
// logging, config/cmdline, hostname, the GOSD-BOOT partition mount, then
// /app supervision for the rest of the process's life. It only returns if
// supervision is stopped (tests) or a fatal error triggers the
// log+sync+sleep+reboot path (step 8); in the latter case it returns the
// error that caused it, after the reboot has already been requested.
func Run(deps Deps, opts Options) error {
	log := deps.FallbackLog

	if err := mountEarly(deps.Mounter); err != nil {
		return fatal(deps, log, "mounting early filesystems", err)
	}

	var console io.Writer = os.Stderr
	if w, err := deps.OpenConsole(); err != nil {
		log("opening /dev/console failed, continuing with fallback logging: %v", err)
	} else {
		console = w
		log = NewLogger(w).Printf
	}

	cfg, err := deps.ReadConfig()
	if err != nil {
		log("reading config.json failed, using defaults: %v", err)
		cfg = initcfg.Config{}
	}

	// Only reachable now that /proc is mounted (mountEarly above), which
	// is what makes /proc/cmdline readable in the first place.
	if cmdline, err := deps.ReadCmdline(); err != nil {
		log("reading kernel cmdline failed, no gosd.* overrides applied: %v", err)
	} else {
		if cmdline.Board != "" {
			cfg.Board = cmdline.Board
		}
		if cmdline.Debug {
			log("debug mode enabled (gosd.debug)")
		}
	}

	if err := deps.Hostname.SetHostname(cfg.Hostname); err != nil {
		return fatal(deps, log, "setting hostname", err)
	}
	log("hostname set to %q", cfg.Hostname)

	if err := MountBootPartition(deps.Mounter, opts.BootTarget, opts.BootDevices, opts.BootTimeout, deps.Sleep, deps.Now); err != nil {
		return fatal(deps, log, "mounting boot partition", err)
	}
	log("boot partition mounted at %s", opts.BootTarget)

	// gosd.toml lives on the just-mounted GOSD-BOOT partition, so it can
	// only be read from here on — never before. Its hostname (if set)
	// takes precedence over config.json's, which means the hostname
	// applied at step 4 above may already be stale and has to be
	// re-applied now, before /app starts.
	var gosdToml gosdtoml.Config
	if deps.ReadGosdToml != nil {
		parsed, err := deps.ReadGosdToml()
		if err != nil {
			log("reading gosd.toml failed, using config.json instead: %v", err)
		} else {
			gosdToml = parsed
			if gosdToml.Hostname != "" {
				cfg.Hostname = gosdToml.Hostname
			}
		}

		if err := deps.Hostname.SetHostname(cfg.Hostname); err != nil {
			return fatal(deps, log, "re-applying hostname after gosd.toml", err)
		}
		log("hostname set to %q (gosd.toml applied)", cfg.Hostname)
	}

	if deps.StartNetworking != nil {
		go deps.StartNetworking(cfg, gosdToml, log)
	}

	env := []string{
		"GOSD_BOARD=" + cfg.Board,
		"GOSD_HOSTNAME=" + cfg.Hostname,
	}
	sup := &Supervisor{
		Start: func() (int, error) {
			return deps.AppStarter.Start(opts.AppPath, env, console, console)
		},
		Wait:        deps.Reaper.Wait,
		Sleep:       deps.Sleep,
		Now:         deps.Now,
		Backoff:     NewBackoff(DefaultBackoffBase, DefaultBackoffCap),
		StableAfter: StableRunThreshold,
		Log:         log,
	}
	sup.Run(opts.Stop)
	return nil
}

// fatal implements step 8 of the boot sequence: log, sync, sleep 5s, then
// reboot. It returns the wrapped error so callers (and tests) can observe
// what happened; in production the machine reboots before that return ever
// matters.
func fatal(deps Deps, log func(format string, args ...any), action string, err error) error {
	wrapped := fmt.Errorf("%s failed: %w", action, err)
	log("fatal: %v; rebooting in 5s", wrapped)
	deps.Rebooter.Sync()
	deps.Sleep(5 * time.Second)
	deps.Rebooter.Reboot()
	return wrapped
}
