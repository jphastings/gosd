package boot

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/provision"
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
	// even though step 4 already applied config.json's value. The
	// warnings return mirrors gosdtoml.Parse's own (bare-scalar [env]
	// coercions, dropped non-scalar entries): Run logs each one, since
	// gosd-init has no interactive surface to surface them any other way.
	ReadGosdToml func() (gosdtoml.Config, []string, error)

	// ReadProvisioning reads cloud-init's user-data/network-config (and
	// checks for firstrun.sh) on the just-mounted GOSD-BOOT partition —
	// see internal/provision. Nil-checked like ReadGosdToml; it sits
	// between config.json and gosd.toml in the locked precedence chain
	// (gosd.toml > cloud-init > config.json), so Run reads it first and
	// lets a subsequent gosd.toml value override it. log is passed
	// through so provision.Read can report per-file problems (missing,
	// unreadable, malformed) at the point they're found, the same as
	// every other package in gosd-init that owns multi-step diagnostics.
	ReadProvisioning func(log func(format string, args ...any)) provision.Result

	// EnsureDataMountpoint creates the data mount target directory on the
	// RAM-backed rootfs (the initramfs archive carries no empty
	// directories, so /data doesn't exist until something makes it).
	// Nil-checked, like ReadGosdToml: tests that don't exercise the data
	// partition leave it unset.
	EnsureDataMountpoint func() error
	// EnsureDataMarker creates the .gosd-data marker file on the mounted
	// data partition if it isn't already there (i.e. on the partition's
	// first boot). Only called after the data partition mounts.
	EnsureDataMarker func() error

	Sleep func(time.Duration)
	Now   func() time.Time

	// StartNetworking, if non-nil, is called in its own goroutine
	// immediately before /app supervision begins, and is passed the
	// fully-resolved config (cmdline overrides already applied), the
	// parsed gosd.toml (zero value if absent, unreadable, or garbage) so
	// wifiup can prefer its wifi block over cfg's, every WiFi network
	// cloud-init's network-config named (nil if none/absent — see
	// internal/provision), and Run's current logger (the console, if
	// opening it succeeded) so its output goes to the same place as the
	// rest of gosd-init's. Networking (link up, DHCP, DNS, WiFi) must
	// never block or delay /app's start, so Run doesn't wait for it and
	// doesn't know or care what it does beyond that; production wires
	// this to start both netup.Run (wired) and wifiup.Run (WiFi), tests
	// leave it nil.
	StartNetworking func(cfg initcfg.Config, gosdToml gosdtoml.Config, provisionWifi []provision.WifiNetwork, log func(format string, args ...any))
}

// Options holds the per-boot paths the sequence acts on.
type Options struct {
	AppPath string

	BootTarget  string
	BootDevices []string
	BootTimeout time.Duration

	// DataTarget is where the GOSD-DATA partition is mounted read-write;
	// empty skips the data mount entirely (tests that don't care about
	// it). A missing or unmountable data partition is never fatal — the
	// app just doesn't get GOSD_DATA.
	DataTarget  string
	DataDevices []string
	DataTimeout time.Duration

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

	// gosd.toml and cloud-init provisioning both live on the just-mounted
	// GOSD-BOOT partition, so neither can be read before now. Precedence
	// (locked, see docs/provisioning-formats.md) is
	// gosd.toml > cloud-init > config.json: cloud-init is read first so a
	// subsequent gosd.toml value can still override it, and either one
	// overriding the hostname applied at step 4 above means it has to be
	// re-applied here, before /app starts.
	var provisionResult provision.Result
	if deps.ReadProvisioning != nil {
		provisionResult = deps.ReadProvisioning(log)
		if provisionResult.Hostname != "" {
			cfg.Hostname = provisionResult.Hostname
			log("hostname from cloud-init user-data")
		}
	}

	var gosdToml gosdtoml.Config
	if deps.ReadGosdToml != nil {
		parsed, warnings, err := deps.ReadGosdToml()
		if err != nil {
			log("reading gosd.toml failed, using cloud-init/config.json instead: %v", err)
		} else {
			gosdToml = parsed
			if gosdToml.Hostname != "" {
				cfg.Hostname = gosdToml.Hostname
				log("hostname from gosd.toml")
			}
		}
		for _, warning := range warnings {
			log("%s", warning)
		}

		if err := deps.Hostname.SetHostname(cfg.Hostname); err != nil {
			return fatal(deps, log, "re-applying hostname after gosd.toml", err)
		}
		log("hostname set to %q (gosd.toml applied)", cfg.Hostname)
	} else if provisionResult.Hostname != "" {
		if err := deps.Hostname.SetHostname(cfg.Hostname); err != nil {
			return fatal(deps, log, "re-applying hostname after cloud-init", err)
		}
		log("hostname set to %q (cloud-init applied)", cfg.Hostname)
	}

	switch {
	case gosdToml.Wifi.SSID != "":
		log("wifi from gosd.toml")
	case len(provisionResult.Wifi) > 0:
		log("wifi from cloud-init network-config")
		if len(provisionResult.Wifi) > 1 {
			log("cloud-init network-config named %d WiFi networks; gosd-init only ever joins the first (%q)", len(provisionResult.Wifi), provisionResult.Wifi[0].SSID)
		}
	case cfg.Wifi.SSID != "":
		log("wifi from config.json")
	}

	env := []string{
		"GOSD_BOARD=" + cfg.Board,
		"GOSD_HOSTNAME=" + cfg.Hostname,
	}
	if dataDir := mountDataPartition(deps, opts, log); dataDir != "" {
		env = append(env, "GOSD_DATA="+dataDir)
	}
	env = append(env, mergeUserEnv(cfg.Env, gosdToml.Env, log)...)

	if deps.StartNetworking != nil {
		go deps.StartNetworking(cfg, gosdToml, provisionResult.Wifi, log)
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

// mountDataPartition performs the optional GOSD-DATA mount and returns the
// mounted directory, or "" when the app should get no GOSD_DATA. Nothing in
// here is ever fatal: a missing partition (an image built with
// --data-size=0, or from before the partition existed) or a failing mount
// just means no persistent storage this boot.
func mountDataPartition(deps Deps, opts Options, log func(format string, args ...any)) string {
	if opts.DataTarget == "" {
		return ""
	}

	if deps.EnsureDataMountpoint != nil {
		if err := deps.EnsureDataMountpoint(); err != nil {
			log("creating %s failed, continuing without persistent storage: %v", opts.DataTarget, err)
			return ""
		}
	}

	if err := MountDataPartition(deps.Mounter, opts.DataTarget, opts.DataDevices, opts.DataTimeout, deps.Sleep, deps.Now); err != nil {
		if errors.Is(err, ErrDataPartitionMissing) {
			log("no data partition on this image, continuing without persistent storage")
		} else {
			log("mounting data partition failed, continuing without persistent storage: %v", err)
		}
		return ""
	}
	log("data partition mounted read-write at %s", opts.DataTarget)

	if deps.EnsureDataMarker != nil {
		if err := deps.EnsureDataMarker(); err != nil {
			// Worth surfacing (first sign of a bad card), but the mount
			// itself succeeded, so the app still gets GOSD_DATA.
			log("creating the data partition marker file failed: %v", err)
		}
	}
	return opts.DataTarget
}

// reservedEnvPrefix is the namespace gosd-init itself owns (GOSD_BOARD,
// GOSD_HOSTNAME, GOSD_DATA, and any future GOSD_* var): per gosd.toml
// [env]'s locked rules, neither baked config.json env nor a hand-edited
// gosd.toml [env] may override it.
const reservedEnvPrefix = "GOSD_"

// mergeUserEnv merges the app's user-set environment variables per
// gosd.toml [env]'s locked precedence — gosd.toml overrides baked
// config.json env, per key, not as a whole-map replace — drops any key in
// gosd-init's reserved GOSD_* namespace (logging each rejection so a
// hand-edited gosd.toml can't silently fail to override GOSD_BOARD etc.),
// and returns the survivors as sorted NAME=VALUE strings for deterministic
// env ordering. Only keys and their source are ever logged, never values:
// they may be secrets.
func mergeUserEnv(baked, card map[string]string, log func(format string, args ...any)) []string {
	source := make(map[string]string, len(baked)+len(card))
	merged := make(map[string]string, len(baked)+len(card))
	for key, value := range baked {
		source[key] = "baked"
		merged[key] = value
	}
	for key, value := range card {
		source[key] = "gosd.toml"
		merged[key] = value
	}

	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var env []string
	var fromGosdToml, fromBaked []string
	for _, key := range keys {
		if strings.HasPrefix(key, reservedEnvPrefix) {
			log("ignoring reserved env key %s from %s (gosd-init owns the %s namespace)", key, source[key], reservedEnvPrefix)
			continue
		}
		env = append(env, key+"="+merged[key])
		if source[key] == "gosd.toml" {
			fromGosdToml = append(fromGosdToml, key)
		} else {
			fromBaked = append(fromBaked, key)
		}
	}

	if len(fromGosdToml) > 0 || len(fromBaked) > 0 {
		log("app env: %s", describeEnvSources(fromGosdToml, fromBaked))
	}

	return env
}

// describeEnvSources formats the "app env: ..." summary line, e.g.
// "app env: API_URL, LOG_LEVEL (gosd.toml); PORT (baked)". Either slice may
// be empty (but not both, since the caller only invokes this when there's
// something to report).
func describeEnvSources(fromGosdToml, fromBaked []string) string {
	var parts []string
	if len(fromGosdToml) > 0 {
		parts = append(parts, strings.Join(fromGosdToml, ", ")+" (gosd.toml)")
	}
	if len(fromBaked) > 0 {
		parts = append(parts, strings.Join(fromBaked, ", ")+" (baked)")
	}
	return strings.Join(parts, "; ")
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
