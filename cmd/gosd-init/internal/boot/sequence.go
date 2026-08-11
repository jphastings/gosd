package boot

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/consoletail"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/dataexpand"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/provsnapshot"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/faultreport"
	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/naming"
	"github.com/jphastings/gosd/internal/provision"
	"github.com/jphastings/gosd/internal/redact"
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

	// PathExists checks whether a path exists, used by MountBootPartition
	// to confirm a freshly-mounted FAT candidate is really the boot
	// partition (see gosd-pcwl) rather than just a filesystem the kernel
	// was willing to mount as FAT. Nil-checked like the other optional
	// deps below: Run defaults it to "always true" so tests that don't
	// care about this check don't have to wire it up.
	PathExists func(path string) bool

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
	// fallback config on the boot partition. It's nil-checked (like
	// StartNetworking) rather than required, so tests that don't care
	// about gosd.toml can leave it unset. Unlike ReadConfig, this can only
	// be called after the boot partition is mounted (step 5), which
	// is why Run calls it right after MountBootPartition succeeds — and
	// why the hostname it may override has to be re-applied there too,
	// even though step 4 already applied config.json's value. The
	// warnings return mirrors gosdtoml.Parse's own (bare-scalar [env]
	// coercions, dropped non-scalar entries): Run logs each one, since
	// gosd-init has no interactive surface to surface them any other way.
	ReadGosdToml func() (gosdtoml.Config, []string, error)

	// ReadProvisioning reads cloud-init's user-data/network-config (and
	// checks for firstrun.sh) on the just-mounted boot partition —
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

	// ExpandData does the data partition's first-boot work: for images
	// built with --data-size=expand (config.json's dataExpand), the image
	// ships with no partition 2 at all, and this fills the rest of the
	// card with one; for a fixed-size --data-filesystem=ext4 image
	// (config.json's dataFilesystem), partition 2 already exists
	// pre-formatted at diskfmt's checked-in golden size, and the only
	// first-boot work left is growing it to the partition's real size.
	// Either way it runs before the data mount (see
	// cmd/gosd-init/internal/dataexpand). It is passed the boot-partition
	// device the boot mount actually used, so only the disk the
	// system truly booted from is ever touched — the same reasoning that
	// makes MountBootPartition's sentinel check necessary — plus the
	// resolved data filesystem, this image's per-app data-partition label
	// (config.json's dataLabel, which decides both what a fresh format is
	// stamped with and which survivor may be adopted), and whether this
	// image was built with --data-size=expand, all of which Run derives
	// once (see resolveDataFilesystem) and passes through unchanged.
	// Nil-checked like the other optional deps. An ordinary failure is
	// logged and boot proceeds to the read-only /data fallback;
	// dataexpand.ErrDataCorrupt — an established partition whose
	// filesystem is gone, app data possibly at stake — instead records the
	// failure via FaultReport and halts the device.
	ExpandData func(bootPartitionDevice string, fs diskfmt.FS, dataLabel string, expand bool, log func(format string, args ...any)) error

	// ProvisionSnapshot, if non-nil, is called once the data partition is
	// mounted and this boot's provisioning has settled: it keeps the
	// snapshot in /data up to date and, on the first boot after a reflash,
	// restores the operator's provisioning from it (see
	// cmd/gosd-init/internal/provsnapshot and docs/design/upgrade-path.md
	// §3). Everything it does is best-effort — it returns the gosd.toml
	// config the rest of the boot should use, which is simply the one it
	// was given whenever there's nothing to restore or anything at all
	// goes wrong. Nil-checked like the other optional deps.
	ProvisionSnapshot func(in provsnapshot.Input, log func(format string, args ...any)) provsnapshot.Result

	// WriteHosts appends the device's own hostname to /etc/hosts, once
	// cfg.Hostname is as settled as it's going to get this boot — after
	// gosd.toml/cloud-init have had a chance to override it and after any
	// first-boot-after-reflash restore from the provisioning snapshot, so
	// it never has to run twice. See internal/hostsfile: the static
	// localhost/loopback lines are already baked into the initramfs by
	// gosd build, and this call regenerates the whole file from scratch
	// (hostname included) rather than patching it, which is what keeps
	// those static lines intact regardless of how many times a hostname
	// gets re-resolved earlier in Run. Nil-checked like the other optional
	// deps; a failure here is logged and never fatal, the same as
	// applyHostname's own SetHostname failures.
	WriteHosts func(hostname string) error

	// FaultReport is how a fatal failure reaches LAST_FATAL_ERROR.md at
	// the root of the boot partition, so whoever collects an unattended
	// device can read the latest fatal issue by plugging the card into any
	// computer. Every field is nil-checked — see FaultReportDeps — and a
	// zero value means gosd-init narrates failures to the serial console
	// and nowhere else.
	FaultReport FaultReportDeps

	Sleep func(time.Duration)
	Now   func() time.Time

	// After is the supervisor's stable-run timer (see
	// Supervisor.OnStableRun): it exists so tests can decide when an app
	// has been running long enough to count as recovered. Defaults to
	// time.After.
	After func(time.Duration) <-chan time.Time

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

	// DataTarget is where the data partition is mounted read-write;
	// empty skips the data mount entirely (tests that don't care about
	// it). A missing or unmountable data partition is never fatal — an
	// empty read-only tmpfs is mounted there instead, so app writes fail
	// with EROFS rather than silently vanishing from the RAM rootfs.
	DataTarget  string
	DataDevices []string
	DataTimeout time.Duration

	// Stop, if non-nil, ends app supervision when closed. Production
	// leaves this nil so supervision runs forever, as PID 1 must; tests
	// set it to bound the otherwise-infinite supervise loop.
	Stop <-chan struct{}
}

// Run executes the locked gosd-init boot sequence: early mounts, console
// logging, config/cmdline, hostname, the boot partition mount, then
// /app supervision for the rest of the process's life. It only returns if
// supervision is stopped (tests) or a fatal error triggers the
// log+sync+sleep+reboot path (step 8); in the latter case it returns the
// error that caused it, after the reboot has already been requested.
func Run(deps Deps, opts Options) error {
	log := deps.FallbackLog

	if err := mountEarly(deps.Mounter); err != nil {
		return fatal(deps, log, nil, fatalEarlyMounts, err)
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
	// Absent on any image built before gosd-acdn (config.json's Identity
	// is optional) - nothing to eyeball on those, so nothing is logged.
	if cfg.Identity != "" {
		log("image identity: %s", cfg.ShortIdentity())
	}
	// The board id config.json was baked with, captured before the
	// gosd.board= cmdline override below can replace it. cmdline.txt is a
	// hand-editable file on the FAT partition, and that override doesn't
	// touch the baked boardDisplayName, so a crash report can only pair the
	// two when they still agree — see initcfg.Config.BoardDisplayName and
	// faultreport.Context.BoardDisplayNameFor.
	bakedBoard := cfg.Board
	// The baked defaults, captured before cloud-init or gosd.toml override
	// anything: what this image would provision the device with on its own,
	// and so the yardstick the provisioning snapshot measures an operator's
	// hand-edits against (see provsnapshot).
	baked := provsnapshot.Provisioning{
		Hostname: cfg.Hostname,
		Wifi:     gosdtoml.Wifi{SSID: cfg.Wifi.SSID, Passphrase: cfg.Wifi.Passphrase},
		Env:      cfg.Env,
	}

	// Only reachable now that /proc is mounted (mountEarly above), which
	// is what makes /proc/cmdline readable in the first place.
	var bootDev string
	if cmdline, err := deps.ReadCmdline(); err != nil {
		log("reading kernel cmdline failed, no gosd.* overrides applied: %v", err)
	} else {
		if cmdline.Board != "" {
			cfg.Board = cmdline.Board
		}
		bootDev = cmdline.BootDev
		if cmdline.Debug {
			log("debug mode enabled (gosd.debug)")
		}
	}

	applyHostname(deps, log, cfg.Hostname, "")

	pathExists := deps.PathExists
	if pathExists == nil {
		pathExists = func(string) bool { return true }
	}
	bootDevices := opts.BootDevices
	if bootDev != "" {
		if filtered, matched := FilterBootDevices(bootDevices, bootDev); matched {
			bootDevices = filtered
			log("gosd.bootdev=%s: probing only its partitions for the boot partition (%s)", bootDev, strings.Join(filtered, ", "))
		} else {
			log("gosd.bootdev=%s matches no boot partition candidate; probing all of %s", bootDev, strings.Join(bootDevices, ", "))
		}
	}
	bootDevice, err := MountBootPartition(deps.Mounter, opts.BootTarget, bootDevices, opts.BootTimeout, pathExists, deps.Sleep, deps.Now)
	if err != nil {
		return fatal(deps, log, nil, fatalBootMount, err)
	}
	log("boot partition mounted at %s from %s", opts.BootTarget, bootDevice)

	// From here on a fatal can be recorded on the card itself: there is
	// somewhere to write it. Everything above this line can only ever reach
	// the serial console — see fatal.
	report := newFatalReporter(deps, log, faultreport.Context{
		AppName:             cfg.AppName,
		AppVersion:          cfg.AppVersion,
		ShortIdentity:       cfg.ShortIdentity(),
		SupportURL:          cfg.SupportURL,
		BoardID:             cfg.Board,
		BoardDisplayName:    cfg.BoardDisplayName,
		BoardDisplayNameFor: bakedBoard,
	})

	// gosd.toml and cloud-init provisioning both live on the just-mounted
	// boot partition, so neither can be read before now. Precedence
	// (locked, see docs/provisioning-formats.md) is
	// gosd.toml > cloud-init > config.json: cloud-init is read first so a
	// subsequent gosd.toml value can still override it, and either one
	// overriding the hostname applied at step 4 above means it has to be
	// re-applied here, before /app starts.
	var provisionResult provision.Result
	if deps.ReadProvisioning != nil {
		provisionResult = deps.ReadProvisioning(log)
		if provisionResult.Hostname != "" {
			if validHostname(provisionResult.Hostname) {
				cfg.Hostname = provisionResult.Hostname
				log("hostname from cloud-init user-data")
			} else {
				log("hostname %q from cloud-init user-data is invalid (must be 1-%d lowercase letters, digits, and hyphens); keeping %q", provisionResult.Hostname, naming.MaxLength, cfg.Hostname)
			}
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
				if validHostname(gosdToml.Hostname) {
					cfg.Hostname = gosdToml.Hostname
					log("hostname from gosd.toml")
				} else {
					log("hostname %q from gosd.toml is invalid (must be 1-%d lowercase letters, digits, and hyphens); keeping %q", gosdToml.Hostname, naming.MaxLength, cfg.Hostname)
				}
			}
		}
		for _, warning := range warnings {
			log("%s", warning)
		}

		applyHostname(deps, log, cfg.Hostname, "gosd.toml")
	} else if provisionResult.Hostname != "" {
		applyHostname(deps, log, cfg.Hostname, "cloud-init")
	}

	// dataFilesystem is decided once, from config.json's baked
	// --data-filesystem choice alone: unlike Hostname/Wifi/Env/DataFlush,
	// nothing on the boot partition (gosd.toml, cloud-init) can
	// override it — the filesystem a partition holds is fixed for the
	// life of the card, chosen at build time and baked into the image
	// gosd build produced. Both the dataexpand call and mountData below
	// use this one resolved value.
	dataFilesystem := resolveDataFilesystem(cfg.DataFilesystem)
	log("data partition filesystem: %s", dataFilesystem)

	// Computed now, from gosdToml as read straight off the card: /data's
	// mount below and the GOSD_DATA_FLUSH env var built further down both
	// have to agree, and a ProvisionSnapshot restore (below) can go on to
	// reset gosdToml.DataFlush to nil (plan.apply only ever carries
	// forward Hostname/Wifi/Env — DataFlush isn't part of the provisioning
	// snapshot, see gosdtoml.Config.DataFlush's doc) — so this value, not
	// a re-derived one, is what both later steps use.
	dataFlush, dataFlushSource := effectiveDataFlush(cfg.DataFlush, gosdToml.DataFlush)
	if dataFlushSource != "" {
		log("data partition flush: %t (%s)", dataFlush, dataFlushSource)
	}

	// dataexpand also needs to run for a fixed-size ext4 image (config.json's
	// dataFilesystem, not dataExpand): unlike FAT32, ext4 ships pre-formatted
	// at diskfmt's checked-in golden size and needs a one-time grow to the
	// partition's real size on its actual first boot — see
	// cmd/gosd-init/internal/dataexpand's package comment.
	if deps.ExpandData != nil && (cfg.DataExpand || dataFilesystem == diskfmt.EXT4) {
		if err := deps.ExpandData(bootDevice, dataFilesystem, cfg.DataLabel, cfg.DataExpand, log); errors.Is(err, dataexpand.ErrDataCorrupt) {
			return haltForDataCorruption(deps, log, report, cfg.DataLabel, dataFilesystem, cfg.DataExpand, err)
		} else if err != nil {
			log("expanding the data partition failed; continuing without it: %v", err)
		}
	}
	mountData(deps, opts, dataFilesystem, dataFlush, log)

	// The boot counter is durable state, so it lives on the data partition
	// and can only be reached now — which is why the data-corruption halt
	// above reports an unknown boot number rather than a stale one. A
	// read-only or absent /data reports unknown too, in preference to a
	// count that silently never advances.
	if deps.FaultReport.CountBoot != nil {
		if count, ok := deps.FaultReport.CountBoot(); ok {
			report.setBootCount(count)
		}
	}

	// Provisioning has settled, and /data — where the snapshot lives — is
	// as mounted as it's going to get, so this is the first and last moment
	// a reflash can be healed. It runs before the WiFi/env decisions below
	// so that anything it restores takes effect on this boot rather than
	// only the next one.
	if deps.ProvisionSnapshot != nil {
		snapshot := deps.ProvisionSnapshot(provsnapshot.Input{
			Identity:  cfg.Identity,
			Baked:     baked,
			CloudInit: provsnapshot.CloudInit{Hostname: provisionResult.Hostname, Wifi: provisionResult.Wifi},
			GosdToml:  gosdToml,
		}, log)
		gosdToml = snapshot.GosdToml
		if snapshot.HostnameRestored {
			cfg.Hostname = gosdToml.Hostname
			if err := deps.Hostname.SetHostname(cfg.Hostname); err != nil {
				// Best-effort, like everything else the snapshot does: the
				// device keeps the hostname it already had rather than
				// failing to boot over a self-heal.
				log("applying the restored hostname failed, continuing without it: %v", err)
			} else {
				log("hostname set to %q (restored from the provisioning snapshot)", cfg.Hostname)
			}
		}
	}

	// cfg.Hostname is now as settled as it's going to get this boot — every
	// source that can still change it (gosd.toml/cloud-init above, and a
	// provisioning-snapshot restore just above) has already had its turn —
	// so this is the one point Run writes the device's 127.0.1.1 line to
	// /etc/hosts, rather than on every earlier hostname re-resolution.
	if deps.WriteHosts != nil {
		if err := deps.WriteHosts(cfg.Hostname); err != nil {
			log("writing /etc/hosts failed, %q may not resolve to this device's own address: %v", cfg.Hostname, err)
		}
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

	// userEnv is the app's own env — never gosd-init's reserved GOSD_*
	// namespace, which mergeUserEnv already drops — and is what a crash
	// report redacts by value (see envRedactionRules): report was
	// constructed long before this point, back when the boot partition
	// mount succeeded, so its secrets are handed over now through
	// setSecrets rather than at construction.
	userEnv := mergeUserEnv(cfg.Env, gosdToml.Env, log)
	report.setSecrets(envRedactionRules(userEnv))

	env := []string{
		"GOSD_BOARD=" + cfg.Board,
		"GOSD_HOSTNAME=" + cfg.Hostname,
		"GOSD_DATA_FLUSH=" + dataFlushEnvValue(dataFlush),
	}
	env = append(env, userEnv...)

	guard := PanicGuard{Rebooter: deps.Rebooter, Sleep: deps.Sleep, Log: log}
	if deps.StartNetworking != nil {
		guard.Go("networking", func() {
			deps.StartNetworking(cfg, gosdToml, provisionResult.Wifi, log)
		})
	}

	// tail retains /app's own last DefaultCapacity bytes of stdout/stderr,
	// for a crash report's Technical detail — a panic, segfault or OOM kill
	// otherwise scrolls past on a serial cable nobody has attached and PID 1
	// never holds a copy (gosd-s9uq). appOutput tees console FIRST: tail's
	// Write never blocks or errors (see consoletail.Buffer's doc), so
	// ordering it after console can't change what reaches the console, and
	// it keeps the console's own byte stream the primary path rather than a
	// side effect of the tail's.
	tail := consoletail.New()
	appOutput := io.MultiWriter(console, tail)

	sup := &Supervisor{
		Start: func() (int, error) {
			return deps.AppStarter.Start(opts.AppPath, env, appOutput, appOutput)
		},
		Wait:        deps.Reaper.Wait,
		Sleep:       deps.Sleep,
		Now:         deps.Now,
		After:       deps.After,
		Backoff:     NewBackoff(DefaultBackoffBase, DefaultBackoffCap),
		StableAfter: StableRunThreshold,
		Log:         log,
	}
	if report != nil {
		// A device that came back must not still look broken, and the
		// next failure after a stable run deserves a report of its own:
		// both are the reporter's job (see fatalReporter). Guarded like
		// every other goroutine gosd-init starts — a panic in PID 1 is a
		// dead appliance (gosd-fkkr).
		sup.OnStableRun = func() { guard.Guard("the crash-report cleanup", report.markStableRun) }
	}
	// OnExit runs synchronously inside sup.Run, which is itself already
	// wrapped by the guard.Guard call below, so a panic here (there
	// shouldn't be one — record's own errors are all handled) is caught by
	// that outer guard rather than needing one of its own.
	//
	// Exactly one report is written per exit, and a fault the app declared
	// for itself outranks the crash tail: the app knows what its user was
	// promised and what would fix it, where the tail only knows what blew
	// up — which withConsoleTail keeps, as technical detail. Recording both
	// would spend two boot-FAT remounts to leave the less useful one on the
	// card, since fault.Fatal exits non-zero and so reads as a crash too.
	sup.OnExit = func(status ExitStatus, _ time.Duration) (stop bool) {
		if declared, ok := appFault(deps); ok {
			haltForAppFault(deps, log, report, declared, tail.String())
			return true
		}
		if isCrash(status) {
			report.record(newAppCrashReport(status, tail.String()))
		}
		return false
	}
	guard.Guard("app supervision", func() { sup.Run(opts.Stop) })
	return nil
}

// RunAndReboot runs the boot sequence and, however it ends — the fatal
// path, a panic, or a clean return — asks for a reboot before returning
// itself. PID 1 exiting is a kernel panic, so gosd-init's main calls this
// and then blocks forever: this is what makes that block a formality
// rather than the only thing between a latent bug and a bricked board.
func RunAndReboot(deps Deps, opts Options) {
	guard := PanicGuard{Rebooter: deps.Rebooter, Sleep: deps.Sleep, Log: deps.FallbackLog}
	defer guard.recoverPanic("the boot sequence")

	err := Run(deps, opts)
	guard.Reboot(fmt.Sprintf("the boot sequence returned (%v)", err))
}

// resolveDataFilesystem maps config.json's baked dataFilesystem field
// (gosd build --data-filesystem) to the diskfmt.FS token both the
// dataexpand step and the data mount need. "" (an image built before this
// field existed) and any value dataexpand doesn't build are treated as
// diskfmt.FAT32 — FAT32 is, and remains, the default — rather than
// rejected, so an old config.json keeps behaving exactly as it always did.
func resolveDataFilesystem(raw string) diskfmt.FS {
	if diskfmt.FS(raw) == diskfmt.EXT4 {
		return diskfmt.EXT4
	}
	return diskfmt.FAT32
}

// effectiveDataFlush resolves the vfat "flush" mount option's effective
// value for this boot: gosd.toml's data_flush key, when the operator set
// one (override non-nil), else config.json's baked gosd build --data-flush
// default. It also reports the source, "" when nothing overrode the baked
// value, purely so Run can log an override without logging every ordinary
// boot that just uses the baked default (bean gosd-9m1k).
func effectiveDataFlush(baked bool, override *bool) (flush bool, source string) {
	if override != nil {
		return *override, "gosd.toml"
	}
	return baked, ""
}

// dataFlushEnvValue formats the effective data-flush setting for
// GOSD_DATA_FLUSH, the reserved env var blockmount's emmc/disk vfat mounts
// read it back from (see internal/blockmount's vfatMountOption) since that
// package mounts from the app's own process, which has no access to
// config.json or gosd.toml directly.
func dataFlushEnvValue(flush bool) string {
	if flush {
		return "1"
	}
	return "0"
}

// mountData mounts the data partition (fs — FAT32 or ext4, see
// resolveDataFilesystem) read-write at opts.DataTarget when it exists, and
// otherwise mounts an empty read-only tmpfs there so that app writes fail
// loudly with EROFS instead of silently landing in the RAM-backed rootfs
// and vanishing on reboot (see MountDataReadOnlyFallback). Nothing here is
// ever fatal: a missing partition (an image built with --data-size=0, or
// from before the partition existed) or a failing mount just means no
// persistent storage this boot. flush is the effective data-flush setting
// (see effectiveDataFlush), passed through to MountDataPartition (a no-op
// for anything other than FAT32 — see dataMountOption).
func mountData(deps Deps, opts Options, fs diskfmt.FS, flush bool, log func(format string, args ...any)) {
	if opts.DataTarget == "" {
		return
	}

	if deps.EnsureDataMountpoint != nil {
		if err := deps.EnsureDataMountpoint(); err != nil {
			// Without the mountpoint we can neither mount the partition nor
			// the read-only fallback; nothing left to do but report it.
			log("creating %s failed, continuing without persistent storage: %v", opts.DataTarget, err)
			return
		}
	}

	if err := MountDataPartition(deps.Mounter, opts.DataTarget, opts.DataDevices, opts.DataTimeout, fs, flush, deps.Sleep, deps.Now); err != nil {
		if errors.Is(err, ErrDataPartitionMissing) {
			log("no data partition on this image; mounting %s read-only", opts.DataTarget)
		} else {
			log("mounting data partition failed; mounting %s read-only: %v", opts.DataTarget, err)
		}
		if err := MountDataReadOnlyFallback(deps.Mounter, opts.DataTarget); err != nil {
			log("mounting the read-only %s placeholder failed; writes there will be lost on reboot: %v", opts.DataTarget, err)
		}
		return
	}
	log("data partition mounted read-write at %s", opts.DataTarget)

	if deps.EnsureDataMarker != nil {
		if err := deps.EnsureDataMarker(); err != nil {
			// Worth surfacing (first sign of a bad card), but the mount
			// itself succeeded, so persistence is available.
			log("creating the data partition marker file failed: %v", err)
		}
	}
}

// reservedEnvPrefix is the namespace gosd-init itself owns (GOSD_BOARD,
// GOSD_HOSTNAME, and any future GOSD_* var): per gosd.toml
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

// envRedactionRules turns the app's own env — mergeUserEnv's output, which
// has already dropped gosd-init's reserved GOSD_* namespace — into the
// rules a crash report redacts its body with: each KEY=VALUE pair becomes a
// rule replacing every occurrence of VALUE with {$KEY}. Callers MUST pass
// mergeUserEnv's return value here, never the full env slice Run sends the
// app (which is this plus GOSD_BOARD/GOSD_HOSTNAME/GOSD_DATA_FLUSH
// prepended) — per gosd-m6py's locked decision, GOSD_DATA_FLUSH is "0" or
// "1", and redacting it would replace every digit in the technical detail.
// A value too short to redact safely (redact.MinNeedleLength) is not
// filtered here; redact.Redact applies that floor uniformly and reports the
// skip, so there is exactly one place that decision is made.
func envRedactionRules(env []string) []redact.Rule {
	rules := make([]redact.Rule, 0, len(env))
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		rules = append(rules, redact.Rule{Needle: value, Replacement: "{$" + key + "}"})
	}
	return rules
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

// fatalClass is one kind of gosd-init fatal: its stable error code, the
// prose a device's owner reads, and whether the device halts or reboots.
//
// The codes are namespaced GOSD-* to distinguish them from an app's own, and
// are part of the contract a support page mirrors, so they are stable —
// see docs/crash-reports.md, which lists them.
type fatalClass struct {
	// code is the report's error_code, stable and greppable.
	code string
	// action names what failed, in the gerund form the returned error and
	// the console line both use ("mounting the boot partition"). Empty
	// leaves the cause to speak for itself, for a class whose own error
	// already reads as a complete statement.
	action string
	// doing is what the device was doing for its user at the time, in the
	// terms its owner thinks in.
	doing string
	// problem is a human explanation of what went wrong.
	problem string
	// fix is a concrete instruction, or "" to send the reader to the
	// image's support URL instead.
	fix string
	// halt stops the device instead of rebooting it. Reserved for states
	// no retry can improve: a reboot loop there just grinds the card and
	// buries the report under its own repetition. Anything that might
	// succeed on a second attempt reboots, which is the default.
	halt bool
}

// The fatal classes gosd-init raises itself. Neither of the two below can
// ever be recorded on the card — they are the failures that happen before,
// or in the course of, mounting the very partition a report is written to —
// so the serial console is their only route, and docs/crash-reports.md says
// so. They still carry prose because the class table is the one place a
// reader should have to look to know what a given code means.
var (
	// fatalEarlyMounts: /proc, /sys, /dev and /run are the ground
	// everything else stands on. Rebooting is worth a try — the failure
	// may be a device that hadn't finished probing — and there is nothing
	// else left to attempt.
	fatalEarlyMounts = fatalClass{
		code:    "GOSD-EARLY-MOUNT",
		action:  "mounting early filesystems",
		doing:   "starting up",
		problem: "This device couldn't set up the basic system directories it needs before it can run anything at all.",
	}

	// fatalBootMount: the card the device booted from can't be re-read, or
	// what was found isn't a GoSD boot partition. Rebooting is right
	// because a slow SD controller is a real cause and the mount is
	// already retried before this fires.
	fatalBootMount = fatalClass{
		code:    "GOSD-BOOT-MOUNT",
		action:  "mounting boot partition",
		doing:   "starting up",
		problem: "This device couldn't read the SD card it started from.",
		fix:     "Turn the device off, re-seat the card, and turn it back on. If that doesn't help, write the image to the card again — or to a different card, since this one may be failing.",
	}
)

// haltForDataCorruption is the unattended-device version of a refusal: the
// established data partition no longer holds the filesystem a completed
// first boot left, and anything that "fixed" it would destroy whatever the
// app had stored. The failure is recorded to LAST_FATAL_ERROR.md on the boot
// partition — readable on any computer the card is plugged into — and the
// device halts rather than rebooting, because no retry can improve a
// corrupt filesystem and a reboot loop would only mask it. dataLabel and fs
// are this image's own baked choices (config.json's dataLabel and
// dataFilesystem), so the recovery instructions name the exact volume the
// next boot will accept rather than a generic one; dataExpand decides which
// second option is actually true of this image, since only a
// --data-size=expand image creates a missing data partition for itself
// (dataexpand.Run returns early otherwise, so deleting a fixed-size image's
// partition 2 leaves it gone until the card is flashed again). Like fatal,
// it returns the wrapped error for callers and tests; in production the
// machine has halted before that matters.
func haltForDataCorruption(deps Deps, log func(format string, args ...any), report *fatalReporter, dataLabel string, fs diskfmt.FS, dataExpand bool, cause error) error {
	orStartOver := "delete partition 2 and flash this image to the card again, which restores that partition empty"
	if dataExpand {
		orStartOver = "delete partition 2 entirely — the next boot will recreate it, empty"
	}

	// No action: dataexpand's own error already reads as a whole sentence
	// ("the data partition is corrupt: /dev/mmcblk0p2 holds nothing ..."),
	// and wrapping it in another gerund would only say it twice.
	return fatal(deps, log, report, fatalClass{
		code:    "GOSD-DATA-CORRUPT",
		doing:   "starting up",
		problem: "The part of the card this device keeps its data on no longer holds a filesystem it recognises. It was stopped rather than started, so that whatever is still there can be salvaged.",
		fix: fmt.Sprintf("Plug the card into a computer and save anything you need from partition 2. Then either reformat that partition as %s, labelled %s, or %s.",
			fs, dataLabel, orStartOver),
		halt: true,
	}, cause)
}

// fatal implements step 8 of the boot sequence: log, record what happened
// where the device's owner can find it, sync, and then either halt or (the
// default) sleep 5s and reboot. It returns the wrapped error so callers (and
// tests) can observe what happened; in production the machine is on its way
// down before that return ever matters.
//
// report is nil for any failure raised before the boot partition is mounted,
// which is the whole of the sequence up to and including the mount itself:
// there is nowhere to write a report, so those failures reach the serial
// console and nowhere else.
func fatal(deps Deps, log func(format string, args ...any), report *fatalReporter, class fatalClass, err error) error {
	wrapped := err
	if class.action != "" {
		wrapped = fmt.Errorf("%s failed: %w", class.action, err)
	}

	if class.halt {
		log("fatal: %v; halting", wrapped)
	} else {
		log("fatal: %v; rebooting in 5s", wrapped)
	}

	if report == nil {
		log("%s can't be written before the boot partition is mounted, so this is only on the serial console", faultreport.FileName)
	} else {
		report.record(faultreport.Report{
			Code:    class.code,
			Doing:   class.doing,
			Problem: class.problem,
			Fix:     class.fix,
			Detail:  wrapped.Error(),
		})
	}

	deps.Rebooter.Sync()
	if class.halt {
		deps.Rebooter.Halt()
		return wrapped
	}
	deps.Sleep(5 * time.Second)
	deps.Rebooter.Reboot()
	return wrapped
}

// validHostname reports whether name is safe to hand to SetHostname as-is:
// non-empty and unchanged by naming.Sanitize, meaning it already satisfies
// both of Sanitize's constraints — the [a-z0-9-] charset and the
// naming.MaxLength byte cap that sethostname(2) enforces. A gosd.toml or
// cloud-init hostname that fails this check is never silently rewritten to
// fit (see gosd-jeaw): mangling a hand-edited value would only confuse
// whoever wrote it, so it's rejected outright and the previous hostname —
// always itself a value that once passed this same check — is kept.
func validHostname(name string) bool {
	return name != "" && naming.Sanitize(name) == name
}

// applyHostname calls SetHostname and reports the outcome without ever
// failing boot. Earlier, a SetHostname failure here was treated as fatal
// (step 8, reboot); but a wrong hostname is cosmetic, while a reboot loop
// is not — see gosd-jeaw, where a hand-edited gosd.toml hostname long
// enough to make sethostname(2) return EINVAL turned into a permanent
// reboot loop with nothing recorded on the card. step names which source is
// being (re-)applied — "" for the initial config.json apply at step 4,
// "gosd.toml"/"cloud-init" for the re-apply once the boot partition is
// mounted — and is folded into both the success and failure log lines.
func applyHostname(deps Deps, log func(format string, args ...any), hostname, step string) {
	suffix := ""
	if step != "" {
		suffix = fmt.Sprintf(" (%s applied)", step)
	}
	if err := deps.Hostname.SetHostname(hostname); err != nil {
		log("setting hostname to %q failed%s, continuing without changing it: %v", hostname, suffix, err)
		return
	}
	log("hostname set to %q%s", hostname, suffix)
}
