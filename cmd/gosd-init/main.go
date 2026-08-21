// Command gosd-init is PID 1 on a gosd image: a static Go binary that runs
// as /init from the initramfs, brings up the board, and supervises the
// user's application for the life of the device. There is no shell, no
// busybox, no interactive surface of any kind — if gosd-init can't do
// something in Go, it doesn't happen.
package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/boot"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/cardconfig"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/childbackoff"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/cloudflared"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/configstore"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/dataexpand"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/durable"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/mdnsresponder"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/netup"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/statusled"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/timesync"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/tsfunnel"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/wifiup"
	"github.com/jphastings/gosd/internal/configtree"
	"github.com/jphastings/gosd/internal/devreserve"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/faultdrop"
	"github.com/jphastings/gosd/internal/faultreport"
	"github.com/jphastings/gosd/internal/hostsfile"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/provision"
)

const (
	configPath  = "/etc/gosd/config.json"
	cmdlinePath = "/proc/cmdline"
	appPath     = "/app"
	bootTarget  = "/boot"
	dataTarget  = "/data"

	// cloudflaredBinaryPath is where `gosd build --ingress cloudflared`
	// bakes the cloudflared binary into the initramfs (see
	// cmd/gosd/ingress.go's ingressCloudflaredDest and
	// initcfg.Config.IngressCloudflared's doc comment) — duplicated here
	// rather than imported, since that constant lives in the gosd CLI's own
	// internal package, a different binary from gosd-init.
	cloudflaredBinaryPath = "/bin/cloudflared"

	// tsfunnelBinaryPath is where `gosd build --ingress tailscale-funnel`
	// bakes the gosd-tsfunnel shim into the initramfs (see
	// cmd/gosd/ingress.go's ingressTailscaleFunnelDest and
	// initcfg.Config.IngressTailscaleFunnel's doc comment) — duplicated
	// here for the same reason as cloudflaredBinaryPath above: that
	// constant lives in the gosd CLI's own internal package, a different
	// binary from gosd-init.
	tsfunnelBinaryPath = "/bin/gosd-tsfunnel"

	// dataMarkerPath is an empty file created on the data partition the
	// first time it's mounted, marking it as initialized by gosd.
	dataMarkerPath = dataTarget + "/.gosd-data"

	// bootCountPath is the durable boot counter a crash report's "boot:"
	// header comes from — see countBoot for why it lives on the data
	// partition rather than the boot one.
	bootCountPath = dataTarget + "/.gosd-boot-count"

	// bootMountTimeout bounds how long gosd-init retries mounting the
	// boot partition: the MMC controller may still be probing when
	// gosd-init reaches this step, and there's no udev to wait on.
	bootMountTimeout = 10 * time.Second

	// dataMountTimeout bounds retries of the data-partition mount. It runs
	// after the boot mount has already succeeded (so the card is probed
	// and a genuinely missing partition is detected instantly, not
	// retried); the timeout only bounds transient non-ENOENT failures.
	dataMountTimeout = 10 * time.Second
)

// bootDevices are the candidate device nodes for the FAT boot partition,
// tried in order, with no udev available to discover it.
// /dev/vda1 is qemu-virt's virtio-blk SD card (see internal/boards/qemuvirt)
// - listed last since it's never present alongside the real mmcblk devices,
// checked with the exact same probe logic as those.
var bootDevices = []string{"/dev/mmcblk0p1", "/dev/mmcblk1p1", "/dev/vda1"}

// dataDevices are the candidate device nodes for the optional data
// partition: partition 2 of the same devices bootDevices covers.
var dataDevices = []string{"/dev/mmcblk0p2", "/dev/mmcblk1p2", "/dev/vda2"}

func main() {
	platform := boot.NewPlatform()
	platform.IgnoreShutdownSignals()

	deps := boot.Deps{
		Mounter: platform.Mounter,
		// PathExists confirms a freshly-mounted boot-partition candidate
		// really is one (see boot.MountBootPartition and gosd-pcwl); plain
		// os.Stat is enough since gosd-init only ever calls it against an
		// already-mounted path.
		PathExists: pathExists,
		Hostname:   platform.Hostname,
		AppStarter: platform.AppStarter,
		Reaper:     platform.Reaper,
		Rebooter:   platform.Rebooter,
		// StatusLED discovers its LED lazily, on first use — see
		// statusled.Sysfs's doc — so wiring it here, before /sys is
		// mounted, is safe: it's a silent no-op on any board that turns
		// out to have none (qemu-virt, or any board with no gpio-leds LED
		// at all).
		StatusLED:   statusled.New(statusled.DefaultRoot),
		OpenConsole: platform.OpenConsole,
		FallbackLog: fallbackLog,
		ReadConfig:  readConfig,
		// ReadCmdline reads /proc/cmdline, which only exists once /proc is
		// mounted; boot.Run calls this itself, after the early mounts
		// (step 1), rather than main reading it up front.
		ReadCmdline: readCmdline,
		// ReadConfigTree reads /boot/config, the settings this device has
		// been given, which only exists once the boot partition is
		// mounted; boot.Run calls this itself, after that mount (step 5),
		// rather than main reading it up front.
		ReadConfigTree: readConfigTree,
		// ReadProvisioning reads cloud-init's user-data/network-config,
		// which — like the config tree — only exist once the boot
		// partition is mounted (step 5); boot.Run calls this itself, right
		// alongside ReadConfigTree.
		ReadProvisioning: readProvisioning,
		// EditBoot is the only way anything writes to the boot partition
		// mid-boot: it remounts it read-write for the duration of one
		// edit, flushes, and puts it back (see boot.Platform's
		// EditBootPartition).
		EditBoot: func(edit func(root string) error) error {
			return platform.EditBootPartition(bootTarget, edit)
		},
		EnsureDataMountpoint: ensureDataMountpoint,
		EnsureDataMarker:     ensureDataMarker,
		ExpandData:           expandData,
		// ReserveDevices writes down which of this board's block devices
		// belong to GoSD, for gadget.MassStorage to refuse against —
		// plain file operations, not platform ones, since /run is an
		// ordinary tmpfs by the time boot.Run reaches this.
		ReserveDevices: func(devices []devreserve.Entry) error {
			return devreserve.Write(devreserve.Path, devices)
		},
		WriteHosts: func(hostname string) error {
			return hostsfile.Write(hostsfile.Path, hostname)
		},
		FaultReport: boot.FaultReportDeps{
			Write: func(body string) error {
				return platform.WriteFatalReport(bootTarget, body)
			},
			Exists: func(name string) bool {
				return pathExists(filepath.Join(bootTarget, name))
			},
			Remove: func(names []string) error {
				return platform.RemoveBootFiles(bootTarget, names)
			},
			DeviceModel: platform.DeviceModel,
			Uptime:      platform.Uptime,
			ClockSynced: clockSynced,
			CountBoot:   countBoot,
			// AppFault consumes the drop file the public fault package
			// leaves in /run, so a fault the app declared for itself
			// reaches the card exactly once — see internal/faultdrop.
			// Plain file operations, not platform ones: /run is an
			// ordinary tmpfs by the time any app has run.
			AppFault:          func() (faultreport.Report, bool) { return faultdrop.Take(faultdrop.Path) },
			RegisteredSecrets: platform.RegisteredSecrets,
		},
		Sleep: time.Sleep,
		Now:   time.Now,
		After: time.After,
		StartNetworking: func(cfg initcfg.Config, config cardconfig.Tree, log func(format string, args ...any)) {
			// mdnsChanged is netup/wifiup's existing MarkNetworkUp/
			// ClearNetworkUp hooks, additionally fanned out to the mDNS
			// responder below: no change to either package, just an
			// extra notification wrapped around the closures main.go
			// already builds for them (see netupDeps/wifiupDeps).
			mdnsChanged := mdnsresponder.NewSignal()

			// upSet refcounts the shared /run/gosd/network-up marker
			// across netup (wired interfaces) and wifiup (WiFi): both
			// packages' MarkNetworkUp/ClearNetworkUp calls, each keyed
			// by their own interface name, route through this single
			// instance so a dual-interface board (e.g. pi-3b's Ethernet
			// + WiFi) doesn't have one medium going down clear a marker
			// the other medium still needs (bean gosd-akk4). See
			// netup.UpSet's doc.
			upSet := netup.NewUpSet(
				func() error { return netup.MarkNetworkUp(netup.DefaultNetworkUpPath) },
				func() error { return netup.ClearNetworkUp(netup.DefaultNetworkUpPath) },
			)

			// Each of these loops runs for the life of the device, and a
			// panic escaping any of them would take PID 1 with it — so
			// they run guarded, logging the stack and rebooting instead
			// (see boot.PanicGuard and gosd-fkkr).
			guard := boot.PanicGuard{Rebooter: platform.Rebooter, Sleep: time.Sleep, Log: log}

			guard.Go("netup", func() { netup.Run(netupDeps(log, mdnsChanged, upSet), netup.Options{}) })
			guard.Go("timesync", func() {
				timesync.Run(timesyncDeps(log), timesync.Options{
					Servers:               ntpServers(cfg),
					ResyncEvery:           timesync.DefaultResyncInterval,
					NetworkUpPollInterval: timesync.DefaultNetworkUpPollInterval,
					// Floor/MaxStep are gosd-0esw's guards against an
					// unauthenticated SNTP reply setting the clock to an
					// arbitrary value: Floor refuses anything before this
					// image was built, MaxStep bounds how far a resync
					// may step the clock outright (see timesync's
					// package doc). cfg.BuildTime is the zero time.Time
					// for a config.json baked before that field existed,
					// which disables the floor rather than misfiring.
					Floor:   cfg.BuildTime(),
					MaxStep: timesync.DefaultMaxStep,
				})
			})
			guard.Go("the mDNS responder", func() {
				mdnsresponder.Run(mdnsresponderDeps(log, mdnsChanged), mdnsresponder.Options{Hostname: cfg.Hostname})
			})
			// cloudflared is a gosd-SHIPPED system service, not a user
			// external — the narrow gosd-oyhi carve-out (see
			// boot/reaper.go's stash comment and docs/runtime.md's
			// "Your app owns it at runtime" bullet) lets gosd-init
			// supervise it here alongside netup/timesync/mdnsresponder,
			// guarded the same way. Placed before the WiFi block below so
			// an Ethernet-only board's early return (no WiFi hardware)
			// can never skip starting it.
			guard.Go("cloudflared", func() {
				cloudflared.Run(cloudflaredDeps(log, exitCodeOnly(platform.Reaper.Wait)), cloudflared.Options{
					BinaryPath:             cloudflaredBinaryPath,
					Baked:                  cfg.IngressCloudflared,
					Config:                 cloudflaredConfig(config),
					NetworkUpPollInterval:  cloudflared.DefaultNetworkUpPollInterval,
					TimeSyncedTimeout:      cloudflared.DefaultTimeSyncedTimeout,
					TimeSyncedPollInterval: cloudflared.DefaultTimeSyncedPollInterval,
				})
			})
			// tailscale-funnel is the epic gosd-65uy's second gosd-SHIPPED
			// system service under the same gosd-oyhi carve-out as
			// cloudflared above (see that guard.Go call's comment) — same
			// reasoning, same placement before the WiFi block below so an
			// Ethernet-only board's early return can never skip starting it
			// either.
			guard.Go("tailscale-funnel", func() {
				tsfunnel.Run(tsfunnelDeps(log, exitCodeOnly(platform.Reaper.Wait)), tsfunnel.Options{
					BinaryPath:             tsfunnelBinaryPath,
					Baked:                  cfg.IngressTailscaleFunnel,
					Config:                 tsfunnelConfig(config),
					Hostname:               cfg.Hostname,
					NetworkUpPollInterval:  tsfunnel.DefaultNetworkUpPollInterval,
					TimeSyncedTimeout:      tsfunnel.DefaultTimeSyncedTimeout,
					TimeSyncedPollInterval: tsfunnel.DefaultTimeSyncedPollInterval,
				})
			})

			wifiClient, err := wifiup.NewPlatform()
			if err != nil {
				// Expected on an Ethernet-only board with no WiFi
				// hardware/driver at all; not fatal to boot.
				log("WiFi unavailable, skipping: %v", err)
				return
			}
			guard.Guard("wifiup", func() {
				wifiup.Run(wifiupDeps(wifiClient, cfg, cardWifi(config), log, mdnsChanged, upSet), wifiup.Options{})
			})
		},
	}
	opts := boot.Options{
		AppPath:     appPath,
		BootTarget:  bootTarget,
		BootDevices: bootDevices,
		BootTimeout: bootMountTimeout,
		DataTarget:  dataTarget,
		DataDevices: dataDevices,
		DataTimeout: dataMountTimeout,
		// Where this device's own settings are kept so that re-flashing
		// the card doesn't lose them; on the data partition, because that
		// is the only part of the card a re-flash leaves alone.
		ConfigStoreDir: filepath.Join(dataTarget, configstore.Dir),
	}

	// RunAndReboot only returns once a reboot has been requested — by the
	// fatal path, by a panic guard, or by RunAndReboot itself if the boot
	// sequence ever simply returns. The machine is on its way down by
	// then, but main must not return regardless: PID 1 exiting is a kernel
	// panic, and an appliance that panics is an appliance someone has to
	// physically power-cycle (gosd-fkkr).
	boot.RunAndReboot(deps, opts)
	select {}
}

// readConfig reads and parses config.json, which is baked into the
// initramfs itself and so is readable immediately, before any mounts.
func readConfig() (initcfg.Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return initcfg.Config{}, err
	}
	return initcfg.ParseConfig(data)
}

// readCmdline reads and parses the kernel command line. Unlike config.json,
// /proc/cmdline requires /proc to be mounted first.
func readCmdline() (initcfg.CmdlineArgs, error) {
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return initcfg.CmdlineArgs{}, err
	}
	return initcfg.ParseCmdline(string(data)), nil
}

// pathExists reports whether path exists, used by boot.MountBootPartition
// to check for the boot-partition sentinel file on a freshly-mounted
// candidate
// (see gosd-pcwl).
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readConfigTree reads the config/ tree at the root of the boot partition —
// every setting this device has been given, one per file (see
// cmd/gosd-init/internal/cardconfig). Only readable once that partition is
// mounted, which is why boot.Run calls it at step 5 rather than main
// reading it up front. It never fails: a tree that isn't there, or can't be
// read, leaves every setting to the value baked into config.json.
func readConfigTree(log func(format string, args ...any)) cardconfig.Tree {
	return cardconfig.Read(filepath.Join(bootTarget, configtree.Dir), log)
}

// readProvisioning reads cloud-init's user-data/network-config (and checks
// for firstrun.sh) on the boot partition — see internal/provision.
// Like readConfigTree, this only becomes readable once that partition is
// mounted; boot.Run calls it right alongside readConfigTree (step 5).
// provision.Read is itself best-effort (a missing/malformed file is logged
// through log and skipped), so there's no error for this wrapper to
// propagate.
func readProvisioning(log func(format string, args ...any)) provision.Result {
	return provision.Read(bootTarget, log)
}

// ensureDataMountpoint creates /data on the RAM-backed rootfs so the
// data partition has somewhere to mount; the initramfs archive doesn't
// contain empty directories.
func ensureDataMountpoint() error {
	return os.MkdirAll(dataTarget, 0o755)
}

// dataNodeTimeout bounds how long expandData waits for the freshly-created
// data partition's device node to appear: devtmpfs creates it almost
// immediately, but there's no udev to synchronize on.
const dataNodeTimeout = 5 * time.Second

// dataexpandEXT4Mountpoint is where dataexpand mounts the data partition
// briefly while establishing or checking an ext4 filesystem (see
// dataexpand.NewDeps's doc) — unused for a FAT32 image. /run is already a
// tmpfs by the time expandData ever runs (mountEarly, boot sequence step 1).
const dataexpandEXT4Mountpoint = "/run/gosd/dataexpand"

// expandData wires dataexpand's first-boot data-partition work — creating
// the partition for images built with --data-size=expand, and/or growing a
// fixed-size ext4 image's golden filesystem to its partition's real size —
// against the real block-device syscalls, deriving the whole disk and its
// partition-2 node from the partition the boot mount actually used. fs,
// dataLabel and expand are boot.Run's resolved config.json values
// (dataFilesystem, dataLabel, dataExpand), passed straight through to
// dataexpand.Options.
func expandData(bootPartition string, fs diskfmt.FS, dataLabel string, expand bool, log func(format string, args ...any)) error {
	device, partition2, ok := dataexpand.DataPartitionFor(bootPartition)
	if !ok {
		return fmt.Errorf("cannot derive the disk behind boot partition %s", bootPartition)
	}
	return dataexpand.Run(dataexpand.NewDeps(log, dataexpandEXT4Mountpoint), dataexpand.Options{
		Device:          device,
		PartitionDevice: partition2,
		NodeTimeout:     dataNodeTimeout,
		Filesystem:      fs,
		DataLabel:       dataLabel,
		Expand:          expand,
	})
}

// clockSynced reports whether time has been set from a time server this
// boot, which is what decides whether a crash report may print a timestamp
// at all: no board in the fleet has a working RTC, so before the first sync
// the clock reads ~1970 (see faultreport.Context.ClockSynced). A failed
// check reads as unsynced — the report says "unknown" rather than risking a
// confidently wrong date.
func clockSynced() bool {
	synced, err := timeSyncedMarkerExists()
	return err == nil && synced
}

// countBoot records this boot in the durable counter on the data partition
// and returns its number, so a crash report can answer "did this device die
// instantly or after four days?" even when the clock can't.
//
// The counter lives on /data rather than the boot partition deliberately:
// counting on the boot partition would mean remounting it read-write on
// every single boot, and that remount is the one window in which a power cut
// can damage the boot FAT — the very thing the crash report's write-rate
// rule exists to keep rare. A read-only or absent /data (see
// boot.MountDataReadOnlyFallback) therefore reports no count at all, which
// the report renders as "unknown".
//
// The write is durable.WriteFile's write-to-temp-then-rename, so a power
// cut leaves either the old count or the new one, never a truncated file. A
// file that doesn't parse is treated as no counter at all and replaced,
// rather than wedging every future boot on one bad byte.
func countBoot() (int, bool) {
	count := 1
	if data, err := os.ReadFile(bootCountPath); err == nil {
		if previous, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && previous > 0 {
			count = previous + 1
		}
	}

	if err := durable.WriteFile(bootCountPath, []byte(strconv.Itoa(count)+"\n")); err != nil {
		return 0, false
	}
	return count, true
}

// ensureDataMarker creates the .gosd-data marker file on the mounted data
// partition the first time it's seen; on every later boot the file already
// exists and this is a no-op.
func ensureDataMarker() error {
	if _, err := os.Stat(dataMarkerPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(dataMarkerPath, nil, 0o644)
}

// fallbackLog is used before /dev/console is open (or if opening it fails).
func fallbackLog(format string, args ...any) {
	boot.NewLogger(os.Stderr).Printf(format, args...)
}

// netupDeps wires the real, netlink/DHCP-backed networking implementation,
// logging through log (boot's console logger, once available). changed is
// notified alongside every real MarkNetworkUp/ClearNetworkUp call so the
// mDNS responder restarts on link-down and on every lease (initial or
// renewed) — see mdnsresponderDeps and gosd-r796; that notification fires
// on every call regardless of upSet's own refcount decision, matching the
// pre-existing "restart on every lease/link event" mDNS behavior. upSet is
// the same instance wifiupDeps wires WiFi through — see its construction
// in StartNetworking and netup.UpSet's doc (bean gosd-akk4).
func netupDeps(log func(format string, args ...any), changed *mdnsresponder.Signal, upSet *netup.UpSet) netup.Deps {
	platform := netup.NewPlatform()
	return netup.Deps{
		Links:           platform.Links,
		DHCP:            platform.DHCP,
		Clock:           netup.NewRealClock(),
		NewBackoff:      func() *netup.Backoff { return netup.NewBackoff(netup.DefaultBackoffBase, netup.DefaultBackoffCap) },
		WriteResolvConf: func(dns []net.IP) error { return netup.WriteResolvConf(netup.DefaultResolvConfPath, dns) },
		MarkNetworkUp: func(iface string) error {
			err := upSet.Up(iface)
			changed.Notify()
			return err
		},
		ClearNetworkUp: func(iface string) error {
			err := upSet.Down(iface)
			changed.Notify()
			return err
		},
		Log: log,
	}
}

// timesyncDeps wires the real, settimeofday/NTP-backed time-sync
// implementation, logging through log (boot's console logger, once
// available).
func timesyncDeps(log func(format string, args ...any)) timesync.Deps {
	platform := timesync.NewPlatform()
	return timesync.Deps{
		NTP:    platform.NTP,
		System: platform.System,
		RTC:    platform.RTC,
		Clock:  timesync.NewRealClock(),
		NewBackoff: func() *timesync.Backoff {
			return timesync.NewBackoff(timesync.DefaultBackoffBase, timesync.DefaultBackoffCap)
		},
		NetworkUp: func() (bool, error) { return timesync.NetworkUpMarkerExists(netup.DefaultNetworkUpPath) },
		MarkTimeSynced: func() error {
			return timesync.WriteTimeSynced(timesync.DefaultTimeSyncedPath)
		},
		Log: log,
	}
}

// ntpServers returns cfg.NTPServers, falling back to timesync.DefaultServers
// when config.json doesn't specify one (including every config.json baked
// before this field existed) — the bean requires this field stay optional.
func ntpServers(cfg initcfg.Config) []string {
	if len(cfg.NTPServers) > 0 {
		return cfg.NTPServers
	}
	return timesync.DefaultServers
}

// The settings gosd-init's networking modules read, by their paths in the
// card's config tree. They live here, where those modules are wired,
// rather than inside each module: a module is handed its own settings,
// already read, and never the tree itself.
const (
	wifiSSIDPath       = "wifi/ssid"
	wifiPassphrasePath = "wifi/passphrase"
	cloudflaredDir     = "ingress/cloudflared"
	tsfunnelDir        = "ingress/tailscale-funnel"
)

// cardWifi is the wireless network the card names, unset (both fields
// empty) when nobody has named one — in which case wifiup falls back to
// whatever config.json was built with (see wifiup.ConfigCredentials).
func cardWifi(config cardconfig.Tree) wifiup.Wifi {
	return wifiup.Wifi{
		SSID:       config.Get(wifiSSIDPath),
		Passphrase: config.Get(wifiPassphrasePath),
	}
}

// cloudflaredConfig is the Cloudflare Tunnel the card declares, as text:
// every value is whatever somebody typed into the file, and cloudflared's
// own resolveMode is what decides whether that's usable (see
// cloudflared.Config).
func cloudflaredConfig(config cardconfig.Tree) cloudflared.Config {
	return cloudflared.Config{
		Token:    config.Get(cloudflaredDir + "/token"),
		Hostname: config.Get(cloudflaredDir + "/hostname"),
		Port:     config.Get(cloudflaredDir + "/port"),
	}
}

// tsfunnelConfig is the Tailscale Funnel the card declares, on the same
// terms as cloudflaredConfig above.
func tsfunnelConfig(config cardconfig.Tree) tsfunnel.Config {
	return tsfunnel.Config{
		Authkey:    config.Get(tsfunnelDir + "/authkey"),
		Hostname:   config.Get(tsfunnelDir + "/hostname"),
		Port:       config.Get(tsfunnelDir + "/port"),
		FunnelPort: config.Get(tsfunnelDir + "/funnel_port"),
	}
}

// wifiupDeps wires the real, nl80211-backed WiFi implementation (client)
// together with the same netlink/DHCP building blocks netupDeps uses —
// DHCP itself doesn't care whether the underlying medium is wired or
// wireless — and the credential source: the network named on the card
// (which is where an Imager wizard's answers have already been written, see
// boot's consumeCloudInit), else config.json's baked-in wifi block. changed
// and upSet are wired the same way netupDeps wires them (the same
// *mdnsresponder.Signal and *netup.UpSet instances): see that function's
// doc.
func wifiupDeps(client wifiup.WifiClient, cfg initcfg.Config, card wifiup.Wifi, log func(format string, args ...any), changed *mdnsresponder.Signal, upSet *netup.UpSet) wifiup.Deps {
	platform := netup.NewPlatform()
	return wifiup.Deps{
		Wifi:            client,
		Credentials:     wifiup.ConfigCredentials{Wifi: cfg.Wifi, Card: card},
		Links:           platform.Links,
		DHCP:            platform.DHCP,
		Clock:           netup.NewRealClock(),
		NewBackoff:      func() *netup.Backoff { return netup.NewBackoff(netup.DefaultBackoffBase, netup.DefaultBackoffCap) },
		WriteResolvConf: func(dns []net.IP) error { return netup.WriteResolvConf(netup.DefaultResolvConfPath, dns) },
		MarkNetworkUp: func(iface string) error {
			err := upSet.Up(iface)
			changed.Notify()
			return err
		},
		ClearNetworkUp: func(iface string) error {
			err := upSet.Down(iface)
			changed.Notify()
			return err
		},
		Log: log,
	}
}

// mdnsresponderDeps wires the real, pion/mdns-backed responder
// implementation, logging through log and restarting whenever changed
// fires (see netupDeps/wifiupDeps: both notify the same *Signal from their
// MarkNetworkUp/ClearNetworkUp closures).
func mdnsresponderDeps(log func(format string, args ...any), changed *mdnsresponder.Signal) mdnsresponder.Deps {
	return mdnsresponder.Deps{
		NewServer: func(hostname string) (mdnsresponder.Server, error) { return mdnsresponder.NewServer(hostname, log) },
		Changed:   changed.C(),
		Log:       log,
	}
}

// exitCodeOnly adapts the reaper's wider boot.ExitStatus (see that type's
// doc — gosd-s9uq widened it so /app's own supervision can name a signal
// death in human terms) down to the bare exit code cloudflared and
// tsfunnel have always logged: neither package supervises anything that
// needs signal detail, only /app's own supervision does, and ExitCode
// already matches what a plain Reaper.Wait returned before this type
// existed, so this preserves their behavior exactly.
func exitCodeOnly(wait func(pid int) (boot.ExitStatus, error)) func(pid int) (int, error) {
	return func(pid int) (int, error) {
		status, err := wait(pid)
		return status.ExitCode, err
	}
}

// cloudflaredDeps wires the real cloudflared supervision implementation:
// StartProcess is that package's own os/exec-backed starter (platform.go),
// and wait is platform.Reaper.Wait — NEVER exec.Cmd.Wait, for the same
// reason boot.AppStarter's doc comment already gives for /app: as PID 1,
// gosd-init reaps every child through one central wait4(-1, ...) loop, and
// a second, independent wait on the same pid would race it. NetworkUp
// reuses the exact stat check timesyncDeps already wires netup's own
// NetworkUp field through (timesync.NetworkUpMarkerExists); TimeSynced is
// timeSyncedMarkerExists's twin of that same check for timesync's own
// marker, which nothing outside timesync needed to read before cloudflared.
func cloudflaredDeps(log func(format string, args ...any), wait func(pid int) (int, error)) cloudflared.Deps {
	return cloudflared.Deps{
		StartProcess: cloudflared.StartProcess,
		Wait:         wait,
		NetworkUp:    func() (bool, error) { return timesync.NetworkUpMarkerExists(netup.DefaultNetworkUpPath) },
		TimeSynced:   timeSyncedMarkerExists,
		MkdirAll:     os.MkdirAll,
		WriteFile:    os.WriteFile,
		Clock:        cloudflared.NewRealClock(),
		NewBackoff: func() *childbackoff.Backoff {
			return childbackoff.NewBackoff(cloudflared.DefaultBackoffBase, cloudflared.DefaultBackoffCap)
		},
		Log: log,
	}
}

// tsfunnelDeps wires the real tailscale-funnel supervision implementation:
// StartProcess is that package's own os/exec-backed starter (platform.go),
// and wait is platform.Reaper.Wait — NEVER exec.Cmd.Wait, for the exact same
// reaper-race reason cloudflaredDeps's doc comment gives. NetworkUp and
// TimeSynced reuse the same two marker checks cloudflaredDeps wires through,
// since both packages gate on the same network-up/time-synced markers; there
// is no WriteFile here because, unlike cloudflared (which writes a
// config.yml), tsfunnel's per-boot values travel entirely through argv/env
// (see tsfunnel.runArgs/runEnv).
func tsfunnelDeps(log func(format string, args ...any), wait func(pid int) (int, error)) tsfunnel.Deps {
	return tsfunnel.Deps{
		StartProcess: tsfunnel.StartProcess,
		Wait:         wait,
		NetworkUp:    func() (bool, error) { return timesync.NetworkUpMarkerExists(netup.DefaultNetworkUpPath) },
		TimeSynced:   timeSyncedMarkerExists,
		MkdirAll:     os.MkdirAll,
		Clock:        tsfunnel.NewRealClock(),
		NewBackoff: func() *childbackoff.Backoff {
			return childbackoff.NewBackoff(tsfunnel.DefaultBackoffBase, tsfunnel.DefaultBackoffCap)
		},
		Log: log,
	}
}

// timeSyncedMarkerExists reports whether the time-synced marker file
// exists, mirroring timesync.NetworkUpMarkerExists's shape (an existence
// check, "not found" isn't an error). timesync itself has no exported
// helper for its own marker — nothing outside that package needed to check
// it before cloudflared did.
func timeSyncedMarkerExists() (bool, error) {
	_, err := os.Stat(timesync.DefaultTimeSyncedPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("checking %s: %w", timesync.DefaultTimeSyncedPath, err)
}
