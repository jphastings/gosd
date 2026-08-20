package boot

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/cardconfig"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/childbackoff"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/configstore"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/consoletail"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/dataexpand"
	"github.com/jphastings/gosd/internal/configtree"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/faultreport"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/naming"
	"github.com/jphastings/gosd/internal/provision"
	"github.com/jphastings/gosd/internal/redact"
)

// The settings gosd-init itself acts on, named by their path in the card's
// config tree. Everything under envPath belongs to the app rather than to
// gosd-init, and each ingress agent reads its own group directly (see
// main.go's StartNetworking wiring) — the two ingress credentials below are
// named here as well only because a crash report has to redact them, never
// to act on them (see ingressRedactionRules).
const (
	hostnamePath         = "hostname"
	wifiSSIDPath         = "wifi/ssid"
	wifiPassphrasePath   = "wifi/passphrase"
	dataFlushPath        = "data_flush"
	envPath              = "env"
	cloudflaredTokenPath = "ingress/cloudflared/token"
	tsfunnelAuthkeyPath  = "ingress/tailscale-funnel/authkey"
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

	// StatusLED drives the board's onboard status LED, if it has one — see
	// the StatusLED interface's own doc. Nil is a valid, silent no-op.
	StatusLED StatusLED

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

	// ReadConfigTree reads the config/ tree at the root of the boot
	// partition: every setting this device has been given, one per file
	// (see cmd/gosd-init/internal/cardconfig). It is the single source of
	// truth for those settings — config.json's baked values are only the
	// fallback for the ones the card leaves unset — and, unlike
	// ReadConfig, it can only be called once the boot partition is mounted
	// (step 5), which is why Run calls it right after MountBootPartition
	// succeeds, and why the hostname it may name has to be re-applied
	// there even though step 4 already applied config.json's. It never
	// fails: a tree that can't be read reads as an empty one. Nil-checked
	// (like StartNetworking) so a test that only cares about baked values
	// can leave it unset.
	ReadConfigTree func(log func(format string, args ...any)) cardconfig.Tree

	// ReadProvisioning reads cloud-init's user-data/network-config (and
	// checks for firstrun.sh) on the just-mounted boot partition —
	// see internal/provision. Nil-checked like ReadConfigTree. A seed is
	// consumed rather than consulted: Run deletes it and writes what it
	// asked for into the config tree (see consumeCloudInit), so a wizard's
	// answers become ordinary settings instead of a second, competing
	// source of truth. log is passed through so provision.Read can report
	// per-file problems (missing, unreadable, malformed) at the point
	// they're found, the same as every other package in gosd-init that
	// owns multi-step diagnostics.
	ReadProvisioning func(log func(format string, args ...any)) provision.Result

	// EditBoot runs edit against the root of the normally read-only boot
	// partition, with it briefly remounted read-write and everything edit
	// wrote made durable before the read-only mount is restored. Deleting
	// a consumed cloud-init seed and writing its values into the config
	// tree are two separate calls, deliberately: the deletion is durable
	// before the first value is written (see consumeCloudInit).
	// Nil-checked like the other optional deps — nil means this device
	// can't write to its own card, which costs it the seed consumption and
	// nothing else, since a seed's values still apply to the boot that
	// found them.
	EditBoot func(edit func(root string) error) error

	// EnsureDataMountpoint creates the data mount target directory on the
	// RAM-backed rootfs (the initramfs archive carries no empty
	// directories, so /data doesn't exist until something makes it).
	// Nil-checked, like ReadConfigTree: tests that don't exercise the data
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

	// WriteHosts appends the device's own hostname to /etc/hosts, once
	// cfg.Hostname is as settled as it's going to get this boot — after
	// the card, and any cloud-init seed consumed into it, have had their
	// say — so it never has to run twice. See internal/hostsfile: the static
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
	// card's config tree — with any cloud-init seed already consumed into
	// it, so wifiup and each ingress agent read one settled source — and
	// Run's current logger (the console, if opening it succeeded) so its
	// output goes to the same place as the rest of gosd-init's. Networking
	// (link up, DHCP, DNS, WiFi) must never block or delay /app's start,
	// so Run doesn't wait for it and doesn't know or care what it does
	// beyond that; production wires this to start both netup.Run (wired)
	// and wifiup.Run (WiFi), tests leave it nil.
	StartNetworking func(cfg initcfg.Config, config cardconfig.Tree, log func(format string, args ...any))
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

	// ConfigStoreDir is where this device's own settings are kept on the
	// mounted data partition, so that re-flashing the card doesn't lose
	// them (see cmd/gosd-init/internal/configstore). Empty skips the store
	// entirely, exactly as an empty DataTarget skips the data mount: an
	// image with no data partition keeps nothing across a re-flash, and a
	// test that doesn't care about the store doesn't have to wire one up.
	ConfigStoreDir string

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
	setStatusLED(deps, log, "booting", StatusLED.Booting)
	describeStatusLED(deps, log)

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

	// The config tree and cloud-init's seed both live on the just-mounted
	// boot partition, so neither can be read before now. The tree is the
	// single source of truth for what this device has been told to do;
	// config.json's baked values are the fallback for each setting the
	// card leaves unset. A seed is not a second source: it is consumed
	// into the tree (see consumeCloudInit) before anything below reads a
	// setting.
	config := cardconfig.Tree{}
	if deps.ReadConfigTree != nil {
		config = deps.ReadConfigTree(log)
	}
	if deps.ReadProvisioning != nil {
		consumeCloudInit(deps, config, deps.ReadProvisioning(log), log)
	}

	// A hostname on the card supersedes the one applied at step 4 above,
	// so it has to be re-applied here, before /app starts.
	if hostname, ok := cardHostname(config, cfg.Hostname, log); ok {
		cfg.Hostname = hostname
		applyHostname(deps, log, cfg.Hostname, cardconfig.OnCard(hostnamePath))
	}

	// dataFilesystem is decided once, from config.json's baked
	// --data-filesystem choice alone: unlike the hostname, WiFi, env and
	// data_flush settings, nothing on the card can override it — the
	// filesystem a partition holds is fixed for the life of the card,
	// chosen at build time and baked into the image gosd build produced.
	// Both the dataexpand call and mountData below use this one resolved
	// value.
	dataFilesystem := resolveDataFilesystem(cfg.DataFilesystem)
	log("data partition filesystem: %s", dataFilesystem)

	// Computed once, here: /data's mount below and the GOSD_DATA_FLUSH env
	// var built further down have to agree with each other.
	dataFlush, dataFlushSource := effectiveDataFlush(cfg.DataFlush, config.Get(dataFlushPath))
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

	// The settings that survive a re-flash are kept on the data partition,
	// so this is the earliest point they can be reached — and the last
	// point that is any use, since everything below acts on a setting.
	//
	// The one setting already acted on above is data_flush, which decides
	// how /data is mounted and so cannot wait for /data to be mounted. A
	// restored data_flush therefore takes effect from the boot after the
	// re-flash rather than during it, which costs that boot ordinary
	// writeback behaviour and nothing else: durability comes from the
	// fsync sequence in docs/runtime.md, never from that mount option
	// (locked, bean gosd-9m1k).
	if restored := reconcileConfigStore(deps, opts, config, cfg, log); slices.Contains(restored, hostnamePath) {
		// The hostname applied above was read off a card that hadn't yet
		// had its kept settings put back on it; this is the one the device
		// is actually called, and /app hasn't started yet.
		if hostname, ok := cardHostname(config, cfg.Hostname, log); ok {
			cfg.Hostname = hostname
			applyHostname(deps, log, cfg.Hostname, cardconfig.OnCard(hostnamePath))
		}
	}

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

	// cfg.Hostname is as settled as it's going to get this boot — the card
	// and any cloud-init seed have both had their turn above — so this is
	// the one point Run writes the device's 127.0.1.1 line to /etc/hosts,
	// rather than on every earlier hostname re-resolution.
	if deps.WriteHosts != nil {
		if err := deps.WriteHosts(cfg.Hostname); err != nil {
			log("writing /etc/hosts failed, %q may not resolve to this device's own address: %v", cfg.Hostname, err)
		}
	}

	switch {
	case config.Get(wifiSSIDPath) != "":
		log("wifi from %s", cardconfig.OnCard(wifiSSIDPath))
	case cfg.Wifi.SSID != "":
		log("wifi from config.json")
	}

	// userEnv is the app's own env — never gosd-init's reserved GOSD_*
	// namespace, which mergeUserEnv already drops — and is what a crash
	// report redacts by value (see envRedactionRules): report was
	// constructed long before this point, back when the boot partition
	// mount succeeded, so its secrets are handed over now through
	// setSecrets rather than at construction.
	userEnv := mergeUserEnv(cfg.Env, config.Group(envPath), log)
	report.setSecrets(append(envRedactionRules(userEnv), ingressRedactionRules(config)...))

	env := []string{
		"GOSD_BOARD=" + cfg.Board,
		"GOSD_HOSTNAME=" + cfg.Hostname,
		"GOSD_DATA_FLUSH=" + dataFlushEnvValue(dataFlush),
	}
	env = append(env, userEnv...)

	guard := PanicGuard{Rebooter: deps.Rebooter, Sleep: deps.Sleep, Log: log}
	if deps.StartNetworking != nil {
		guard.Go("networking", func() {
			deps.StartNetworking(cfg, config, log)
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

	// appHandedOver guards the one-time "Running" transition: only the
	// first successful start hands control to the app, so a later restart
	// after a transient crash must not blink the LED back to "booting" —
	// there are only three states, and a mid-boot-shaped flicker on every
	// ordinary restart isn't one of them.
	appHandedOver := false
	sup := &Supervisor{
		Start: func() (int, error) {
			pid, err := deps.AppStarter.Start(opts.AppPath, env, appOutput, appOutput)
			if err == nil && !appHandedOver {
				appHandedOver = true
				setStatusLED(deps, log, "running", StatusLED.Running)
			}
			return pid, err
		},
		Wait:        deps.Reaper.Wait,
		Sleep:       deps.Sleep,
		Now:         deps.Now,
		After:       deps.After,
		Backoff:     childbackoff.NewBackoff(DefaultBackoffBase, DefaultBackoffCap),
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
	// up — which faultreport.FoldConsoleTail keeps, as technical detail.
	// Recording both would spend two boot-FAT remounts to leave the less
	// useful one on the card, since fault.Fatal exits non-zero and so reads
	// as a crash too.
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
// value for this boot: the card's data_flush setting when it holds anything
// at all — "set" means non-empty, so any word a person types into that file
// turns it on, which is what its explanation on the card tells them to do —
// else config.json's baked gosd build --data-flush default. It also reports
// the source, "" when the card had no opinion, purely so Run can log an
// override without logging every ordinary boot that just uses the baked
// default (bean gosd-9m1k).
func effectiveDataFlush(baked bool, card string) (flush bool, source string) {
	if card != "" {
		return true, cardconfig.OnCard(dataFlushPath)
	}
	return baked, ""
}

// cardHostname returns the hostname the card's config/hostname names, and
// whether it named a usable one at all — false leaves the sanitized default
// config.json baked in (an app's own name) standing, which is what makes an
// unset hostname file the ordinary case rather than a failure.
//
// A value that isn't a valid hostname is never silently rewritten to fit
// (see validHostname and gosd-jeaw): mangling what somebody typed would
// only confuse them, so it's refused with a log line naming the file to
// fix, and current — itself always a value that once passed this check —
// is kept.
func cardHostname(config cardconfig.Tree, current string, log func(format string, args ...any)) (string, bool) {
	name := config.Get(hostnamePath)
	switch {
	case name == "":
		return "", false
	case !validHostname(name):
		log("the name in %s (%q) can't be used as a hostname: it must be 1-%d lowercase letters, digits, and hyphens; keeping %q", cardconfig.OnCard(hostnamePath), name, naming.MaxLength, current)
		return "", false
	}
	return name, true
}

// consumeCloudInit turns the answers somebody gave a flashing tool's wizard
// into ordinary settings on the card. The seed is DELETED first, durably,
// and only then are its values written into the config tree (locked, epic
// gosd-rw6n): a power cut in that gap loses the answers, which can be given
// again by flashing again, where the reverse order would leave a seed on
// the card that silently overwrote every later hand-edit, on every boot,
// for the life of the device.
//
// Whatever the seed asked for applies to this boot either way — a card that
// has gone read-only, or a device with no way to write to its own card at
// all, shouldn't also lose the network it was just told to join — it simply
// doesn't survive to the next one.
func consumeCloudInit(deps Deps, config cardconfig.Tree, result provision.Result, log func(format string, args ...any)) {
	if len(result.SeedFiles) == 0 {
		return
	}
	values := cloudInitValues(result, log)

	deleted := false
	if deps.EditBoot == nil {
		log("cloud-init provisioning found, but this device can't write to its own card; using it for this boot only")
	} else if err := deps.EditBoot(func(root string) error { return provision.DeleteSeed(root, result.SeedFiles) }); err != nil {
		log("deleting cloud-init's provisioning from the boot partition failed, so it can't become settings on the card; using it for this boot only: %v", err)
	} else {
		deleted = true
	}

	if !deleted {
		for path, value := range values {
			config.Set(path, value)
		}
		return
	}
	if len(values) == 0 {
		return
	}

	if err := deps.EditBoot(func(root string) error {
		return config.Write(filepath.Join(root, configtree.Dir), values)
	}); err != nil {
		log("writing cloud-init's answers into %s/ failed; they still apply to this boot: %v", configtree.Dir, err)
	}
	for _, path := range sortedPaths(values) {
		log("%s set from cloud-init provisioning", cardconfig.OnCard(path))
	}
}

// reconcileConfigStore keeps the copy of this device's own settings on the
// data partition up to date and, on the first boot under an image that copy
// wasn't written under — the boot after a re-flash — puts them back onto the
// card. It returns the settings it restored, so the caller can resolve again
// anything it has already acted on.
//
// It is deliberately called after the cloud-init seed has been consumed into
// the tree (locked, epic gosd-rw6n): a wizard's answers are ordinary card
// edits by then, which is what makes them survive the re-flash after this
// one rather than being a second, competing source of truth.
func reconcileConfigStore(deps Deps, opts Options, config cardconfig.Tree, cfg initcfg.Config, log func(format string, args ...any)) []string {
	if opts.ConfigStoreDir == "" {
		return nil
	}
	// cfg.Identity, never cfg.Board: the identity is the one field a
	// hand-edited cmdline.txt can't overwrite (see initcfg.Config.Board),
	// and mistaking one image for another here would either resurrect a
	// setting somebody deleted or forget one they set.
	return configstore.Reconcile(configstore.Deps{
		Dir:      opts.ConfigStoreDir,
		EditBoot: deps.EditBoot,
		Log:      log,
	}, config, configstore.Options{Identity: cfg.Identity, Baked: cfg.ConfigDigests}).Restored
}

// cloudInitValues maps what a cloud-init seed asked for onto the settings
// that say it. Only the first WiFi network becomes a setting: the tree
// holds one network, as gosd-init has always joined one.
//
// A hostname is written exactly as the wizard gave it, even one cardHostname
// will go on to refuse: the card is the record of what a device was told,
// and a value visible in a file somebody can open and correct is worth more
// than one silently dropped on its way there.
func cloudInitValues(result provision.Result, log func(format string, args ...any)) map[string]string {
	values := make(map[string]string, 3)
	if result.Hostname != "" {
		values[hostnamePath] = result.Hostname
	}
	if len(result.Wifi) > 0 {
		values[wifiSSIDPath] = result.Wifi[0].SSID
		values[wifiPassphrasePath] = result.Wifi[0].Password
		if len(result.Wifi) > 1 {
			log("cloud-init's network-config named %d WiFi networks; only the first (%q) becomes a setting", len(result.Wifi), result.Wifi[0].SSID)
		}
	}
	return values
}

// sortedPaths orders a set of settings by path, so what gets logged about
// them is in the same order every boot.
func sortedPaths(values map[string]string) []string {
	paths := make([]string, 0, len(values))
	for path := range values {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// dataFlushEnvValue formats the effective data-flush setting for
// GOSD_DATA_FLUSH, the reserved env var blockmount's emmc/disk vfat mounts
// read it back from (see internal/blockmount's vfatMountOption) since that
// package mounts from the app's own process, which can read neither
// config.json nor the card's settings directly.
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
// GOSD_HOSTNAME, and any future GOSD_* var): neither baked config.json env
// nor a file somebody puts in config/env/ may override it. The build
// refuses such a file outright (see internal/configtree); this is what
// happens when one is created on the card afterwards.
const reservedEnvPrefix = "GOSD_"

// mergeUserEnv merges the app's environment variables: a value set in the
// card's config/env/ directory overrides the same name baked into
// config.json, per name rather than as a whole-map replace. Any name in
// gosd-init's reserved GOSD_* namespace is dropped, with a log line each,
// so a file created on the card can't silently fail to override GOSD_BOARD
// and leave its author wondering. The survivors are returned as sorted
// NAME=VALUE strings, for deterministic env ordering. Only names and where
// they came from are ever logged, never values: they may be secrets.
func mergeUserEnv(baked, card map[string]string, log func(format string, args ...any)) []string {
	fromCard := make(map[string]bool, len(card))
	merged := make(map[string]string, len(baked)+len(card))
	for key, value := range baked {
		merged[key] = value
	}
	for key, value := range card {
		fromCard[key] = true
		merged[key] = value
	}

	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var env []string
	var cardNames, bakedNames []string
	for _, key := range keys {
		if strings.HasPrefix(key, reservedEnvPrefix) {
			log("ignoring %s: gosd-init owns the %s namespace, so a setting can't be named that", envSourceOf(key, fromCard[key]), reservedEnvPrefix)
			continue
		}
		env = append(env, key+"="+merged[key])
		if fromCard[key] {
			cardNames = append(cardNames, key)
		} else {
			bakedNames = append(bakedNames, key)
		}
	}

	if len(cardNames) > 0 || len(bakedNames) > 0 {
		log("app env: %s", describeEnvSources(cardNames, bakedNames))
	}

	return env
}

// envSourceOf names one rejected environment variable where its author can
// find it: the file on the card, or the image it was built into.
func envSourceOf(key string, fromCard bool) string {
	if fromCard {
		return cardconfig.OnCard(envPath + "/" + key)
	}
	return "the baked-in env value " + key
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

// ingressRedactionRules turns the credentials the card's ingress settings
// carry — a Cloudflare tunnel token, a Tailscale auth key — into the rules
// a crash report is redacted with, alongside envRedactionRules' app env
// (bean gosd-tzd1). They are the one class of secret gosd-init holds
// ITSELF, and until this they were the only one the redaction system knew
// nothing about: an app env value in the same position was scrubbed
// automatically, a tunnel token was not.
//
// No gosd-init code path puts either value in a log line today, so this is
// a safety net rather than a fix for a live leak — which is exactly the
// point. The redaction system exists so that nobody has to audit each new
// log line, or each upstream library's, for whether it prints a credential
// that then reaches a file whose own text tells its reader to forward it.
//
// The rules are built here, at the point Run has the settled config tree,
// rather than inside the ingress agents: an agent that never starts (no
// binary baked, network never up, the child dead on arrival) must not be
// the reason its token stays in a report. A setting nobody set is "" and
// contributes no rule.
func ingressRedactionRules(config cardconfig.Tree) []redact.Rule {
	credentials := []struct {
		path  string
		label string
	}{
		{cloudflaredTokenPath, "{ingress: cloudflared-token}"},
		{tsfunnelAuthkeyPath, "{ingress: tailscale-funnel-authkey}"},
	}

	rules := make([]redact.Rule, 0, len(credentials))
	for _, c := range credentials {
		if value := config.Get(c.path); value != "" {
			rules = append(rules, redact.Rule{Needle: value, Replacement: c.label})
		}
	}
	return rules
}

// describeEnvSources formats the "app env: ..." summary line, e.g.
// "app env: API_URL, LOG_LEVEL (config/env); PORT (baked)" — which of the
// app's environment variables somebody set on the card, and which are the
// image's own. It names the directory rather than each file so the line
// stays one line however many settings there are. Either slice may be empty
// (but not both, since the caller only invokes this when there's something
// to report).
func describeEnvSources(fromCard, fromBaked []string) string {
	var parts []string
	if len(fromCard) > 0 {
		parts = append(parts, strings.Join(fromCard, ", ")+" ("+cardconfig.OnCard(envPath)+")")
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

// setStatusLED calls one of Deps.StatusLED's three state transitions —
// State expected to be a method expression like StatusLED.Booting — and
// logs rather than propagates a failure: an LED is a courtesy for whoever
// is looking at the device, never something worth failing or delaying boot
// over. A nil StatusLED (no LED wired at all — qemu-virt, or a test that
// doesn't care) is a silent no-op.
func setStatusLED(deps Deps, log func(format string, args ...any), state string, call func(StatusLED) error) {
	if deps.StatusLED == nil {
		return
	}
	if err := call(deps.StatusLED); err != nil {
		log("status LED: setting the %s state failed: %v", state, err)
	}
}

// describeStatusLED logs which LED the board resolved to, once, immediately
// after the first state is set — see StatusLEDDescriber for why that line is
// the only diagnostic available on a device with no shell. Silent for a nil
// StatusLED, and for any implementation that doesn't describe itself.
func describeStatusLED(deps Deps, log func(format string, args ...any)) {
	if d, ok := deps.StatusLED.(StatusLEDDescriber); ok {
		log("status LED: %s", d.Describe())
	}
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
		// Only the halting branch gets the fast fatal blink: a rebooting
		// fatal is back to a fresh boot (and its own "booting" blink) within
		// 5s, and — for both of gosd-init's own reboot-only classes — never
		// even has a report to point at, since neither can happen once the
		// boot partition (and so LAST_FATAL_ERROR.md) exists to record one.
		setStatusLED(deps, log, "fatal", StatusLED.Fatal)
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
	// The console copy just logged above (and, when report != nil, the
	// full rendered report record() logged inside it) must survive the
	// Halt/Reboot that follows: a write(2) to a slow serial console
	// returns once the kernel has queued the bytes, not once they've
	// actually gone out over the wire, and unix.Reboot discards whatever
	// is still queued (gosd-fs34). This used to be true only by accident
	// on the reboot branch, which happened to have 5s of Sleep between the
	// last console write and Reboot(); the halt branch had no such
	// cushion, and FlushConsole replaces both with an explicit guarantee
	// that doesn't depend on report size or baud rate.
	deps.Rebooter.FlushConsole()
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
// naming.MaxLength byte cap that sethostname(2) enforces. A hostname from
// the card that fails this check is never silently rewritten to fit (see
// gosd-jeaw): mangling a value somebody typed would only confuse them, so it's rejected outright and the previous hostname —
// always itself a value that once passed this same check — is kept.
func validHostname(name string) bool {
	return name != "" && naming.Sanitize(name) == name
}

// applyHostname calls SetHostname and reports the outcome without ever
// failing boot. Earlier, a SetHostname failure here was treated as fatal
// (step 8, reboot); but a wrong hostname is cosmetic, while a reboot loop
// is not — see gosd-jeaw, where a hand-edited hostname long enough to make
// sethostname(2) return EINVAL turned into a permanent reboot loop with
// nothing recorded on the card. step names which source is being
// (re-)applied — "" for the initial config.json apply at step 4, the
// setting's own path on the card for the re-apply once the boot partition
// is mounted — and is folded into both the success and failure log lines.
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
