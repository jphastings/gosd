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
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/boot"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/cloudflared"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/dataexpand"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/mdnsresponder"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/netup"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/provsnapshot"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/timesync"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/wifiup"
	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/hostsfile"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/provision"
)

const (
	configPath   = "/etc/gosd/config.json"
	cmdlinePath  = "/proc/cmdline"
	gosdTomlPath = "/boot/gosd.toml"
	appPath      = "/app"
	bootTarget   = "/boot"
	dataTarget   = "/data"

	// cloudflaredBinaryPath is where `gosd build --ingress cloudflared`
	// bakes the cloudflared binary into the initramfs (see
	// cmd/gosd/ingress.go's ingressCloudflaredDest and
	// initcfg.Config.IngressCloudflared's doc comment) — duplicated here
	// rather than imported, since that constant lives in the gosd CLI's own
	// internal package, a different binary from gosd-init.
	cloudflaredBinaryPath = "/bin/cloudflared"

	// dataMarkerPath is an empty file created on the GOSD-DATA partition
	// the first time it's mounted, marking it as initialized by gosd.
	dataMarkerPath = dataTarget + "/.gosd-data"

	// bootMountTimeout bounds how long gosd-init retries mounting the
	// GOSD-BOOT partition: the MMC controller may still be probing when
	// gosd-init reaches this step, and there's no udev to wait on.
	bootMountTimeout = 10 * time.Second

	// dataMountTimeout bounds retries of the GOSD-DATA mount. It runs
	// after the boot mount has already succeeded (so the card is probed
	// and a genuinely missing partition is detected instantly, not
	// retried); the timeout only bounds transient non-ENOENT failures.
	dataMountTimeout = 10 * time.Second
)

// bootDevices are the candidate device nodes for the GOSD-BOOT FAT
// partition, tried in order, with no udev available to discover it.
// /dev/vda1 is qemu-virt's virtio-blk SD card (see internal/boards/qemuvirt)
// - listed last since it's never present alongside the real mmcblk devices,
// checked with the exact same probe logic as those.
var bootDevices = []string{"/dev/mmcblk0p1", "/dev/mmcblk1p1", "/dev/vda1"}

// dataDevices are the candidate device nodes for the optional GOSD-DATA FAT
// partition: partition 2 of the same devices bootDevices covers.
var dataDevices = []string{"/dev/mmcblk0p2", "/dev/mmcblk1p2", "/dev/vda2"}

func main() {
	platform := boot.NewPlatform()
	platform.IgnoreShutdownSignals()

	deps := boot.Deps{
		Mounter: platform.Mounter,
		// PathExists confirms a freshly-mounted GOSD-BOOT candidate really
		// is GOSD-BOOT (see boot.MountBootPartition and gosd-pcwl); plain
		// os.Stat is enough since gosd-init only ever calls it against an
		// already-mounted path.
		PathExists:  pathExists,
		Hostname:    platform.Hostname,
		AppStarter:  platform.AppStarter,
		Reaper:      platform.Reaper,
		Rebooter:    platform.Rebooter,
		OpenConsole: platform.OpenConsole,
		FallbackLog: fallbackLog,
		ReadConfig:  readConfig,
		// ReadCmdline reads /proc/cmdline, which only exists once /proc is
		// mounted; boot.Run calls this itself, after the early mounts
		// (step 1), rather than main reading it up front.
		ReadCmdline: readCmdline,
		// ReadGosdToml reads /boot/gosd.toml, which only exists once the
		// GOSD-BOOT partition is mounted; boot.Run calls this itself,
		// after that mount (step 5), rather than main reading it up front.
		ReadGosdToml: readGosdToml,
		// ReadProvisioning reads cloud-init's user-data/network-config,
		// which — like gosd.toml — only exist once the GOSD-BOOT
		// partition is mounted (step 5); boot.Run calls this itself,
		// right alongside ReadGosdToml.
		ReadProvisioning:     readProvisioning,
		EnsureDataMountpoint: ensureDataMountpoint,
		EnsureDataMarker:     ensureDataMarker,
		ExpandData:           expandData,
		// ProvisionSnapshot needs both partitions mounted — the snapshot
		// lives on GOSD-DATA and a restore is written back to GOSD-BOOT —
		// so boot.Run calls it only once the data mount has been attempted.
		ProvisionSnapshot: func(in provsnapshot.Input, log func(format string, args ...any)) provsnapshot.Result {
			deps := provsnapshot.NewDeps(
				filepath.Join(dataTarget, provsnapshot.Dir),
				func(name string, data []byte) error { return platform.WriteBootFile(bootTarget, name, data) },
				log,
			)
			return provsnapshot.Run(deps, in)
		},
		WriteHosts: func(hostname string) error {
			return hostsfile.Write(hostsfile.Path, hostname)
		},
		WriteBootFailure: func(msg string) error {
			return platform.WriteBootFailure(bootTarget, msg)
		},
		Sleep: time.Sleep,
		Now:   time.Now,
		StartNetworking: func(cfg initcfg.Config, gosdToml gosdtoml.Config, provisionWifi []provision.WifiNetwork, log func(format string, args ...any)) {
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
				cloudflared.Run(cloudflaredDeps(log, platform.Reaper.Wait), cloudflared.Options{
					BinaryPath:             cloudflaredBinaryPath,
					Baked:                  cfg.IngressCloudflared,
					Config:                 gosdToml.Ingress.Cloudflared,
					NetworkUpPollInterval:  cloudflared.DefaultNetworkUpPollInterval,
					TimeSyncedTimeout:      cloudflared.DefaultTimeSyncedTimeout,
					TimeSyncedPollInterval: cloudflared.DefaultTimeSyncedPollInterval,
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
				wifiup.Run(wifiupDeps(wifiClient, cfg, gosdToml.Wifi, provisionWifi, log, mdnsChanged, upSet), wifiup.Options{})
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
// to check for the GOSD-BOOT sentinel file on a freshly-mounted candidate
// (see gosd-pcwl).
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readGosdToml reads and parses /boot/gosd.toml, the hand-editable fallback
// config on the GOSD-BOOT partition. The file is entirely optional — a
// missing file is not logged as a problem at all, since most users will
// never touch it — but a present-and-unreadable-as-TOML file (a typo from
// hand-editing) is surfaced as an error for boot.Run to log as a warning;
// either way, boot never fails over it. gosdtoml.Parse's own warnings
// (bare-scalar [env] coercions, dropped non-scalar entries) are passed
// straight through for boot.Run to log.
func readGosdToml() (gosdtoml.Config, []string, error) {
	data, err := os.ReadFile(gosdTomlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return gosdtoml.Config{}, nil, nil
		}
		return gosdtoml.Config{}, nil, err
	}
	return gosdtoml.Parse(data)
}

// readProvisioning reads cloud-init's user-data/network-config (and checks
// for firstrun.sh) on the GOSD-BOOT partition — see internal/provision.
// Like readGosdToml, this only becomes readable once that partition is
// mounted; boot.Run calls it right alongside readGosdToml (step 5).
// provision.Read is itself best-effort (a missing/malformed file is logged
// through log and skipped), so there's no error for this wrapper to
// propagate.
func readProvisioning(log func(format string, args ...any)) provision.Result {
	return provision.Read(bootTarget, log)
}

// ensureDataMountpoint creates /data on the RAM-backed rootfs so the
// GOSD-DATA partition has somewhere to mount; the initramfs archive doesn't
// contain empty directories.
func ensureDataMountpoint() error {
	return os.MkdirAll(dataTarget, 0o755)
}

// dataNodeTimeout bounds how long expandData waits for the freshly-created
// data partition's device node to appear: devtmpfs creates it almost
// immediately, but there's no udev to synchronize on.
const dataNodeTimeout = 5 * time.Second

// expandData wires dataexpand's first-boot GOSD-DATA creation (images built
// with --data-size=expand) against the real block-device syscalls, deriving
// the whole disk and its partition-2 node from the partition the boot mount
// actually used.
func expandData(bootPartition string, log func(format string, args ...any)) error {
	device, partition2, ok := dataexpand.DataPartitionFor(bootPartition)
	if !ok {
		return fmt.Errorf("cannot derive the disk behind boot partition %s", bootPartition)
	}
	return dataexpand.Run(dataexpand.NewDeps(log), dataexpand.Options{
		Device:          device,
		PartitionDevice: partition2,
		NodeTimeout:     dataNodeTimeout,
	})
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

// wifiupDeps wires the real, nl80211-backed WiFi implementation (client)
// together with the same netlink/DHCP building blocks netupDeps uses —
// DHCP itself doesn't care whether the underlying medium is wired or
// wireless — and the credential source, in locked precedence order:
// gosd.toml's hand-edited network, else the first network cloud-init's
// network-config named (provisionWifi), else config.json's baked-in wifi
// block. changed and upSet are wired the same way netupDeps wires them
// (the same *mdnsresponder.Signal and *netup.UpSet instances): see that
// function's doc.
func wifiupDeps(client wifiup.WifiClient, cfg initcfg.Config, gosdWifi gosdtoml.Wifi, provisionWifi []provision.WifiNetwork, log func(format string, args ...any), changed *mdnsresponder.Signal, upSet *netup.UpSet) wifiup.Deps {
	platform := netup.NewPlatform()
	return wifiup.Deps{
		Wifi:            client,
		Credentials:     wifiup.ConfigCredentials{Wifi: cfg.Wifi, GosdToml: gosdWifi, Provision: provisionWifi},
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
		NewBackoff: func() *cloudflared.Backoff {
			return cloudflared.NewBackoff(cloudflared.DefaultBackoffBase, cloudflared.DefaultBackoffCap)
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
