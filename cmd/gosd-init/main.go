// Command gosd-init is PID 1 on a gosd image: a static Go binary that runs
// as /init from the initramfs, brings up the board, and supervises the
// user's application for the life of the device. There is no shell, no
// busybox, no interactive surface of any kind — if gosd-init can't do
// something in Go, it doesn't happen.
package main

import (
	"net"
	"os"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/boot"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/mdnsresponder"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/netup"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/timesync"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/wifiup"
	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/initcfg"
)

const (
	configPath   = "/etc/gosd/config.json"
	cmdlinePath  = "/proc/cmdline"
	gosdTomlPath = "/boot/gosd.toml"
	appPath      = "/app"
	bootTarget   = "/boot"
	dataTarget   = "/data"

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
var bootDevices = []string{"/dev/mmcblk0p1", "/dev/mmcblk1p1"}

// dataDevices are the candidate device nodes for the optional GOSD-DATA FAT
// partition: partition 2 of the same cards bootDevices covers.
var dataDevices = []string{"/dev/mmcblk0p2", "/dev/mmcblk1p2"}

func main() {
	platform := boot.NewPlatform()
	platform.IgnoreShutdownSignals()

	deps := boot.Deps{
		Mounter:     platform.Mounter,
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
		ReadGosdToml:         readGosdToml,
		EnsureDataMountpoint: ensureDataMountpoint,
		EnsureDataMarker:     ensureDataMarker,
		Sleep:                time.Sleep,
		Now:                  time.Now,
		StartNetworking: func(cfg initcfg.Config, gosdToml gosdtoml.Config, log func(format string, args ...any)) {
			// mdnsChanged is netup/wifiup's existing MarkNetworkUp/
			// ClearNetworkUp hooks, additionally fanned out to the mDNS
			// responder below: no change to either package, just an
			// extra notification wrapped around the closures main.go
			// already builds for them (see netupDeps/wifiupDeps).
			mdnsChanged := mdnsresponder.NewSignal()

			go netup.Run(netupDeps(log, mdnsChanged), netup.Options{})
			go timesync.Run(timesyncDeps(log), timesync.Options{
				Servers:               ntpServers(cfg),
				ResyncEvery:           timesync.DefaultResyncInterval,
				NetworkUpPollInterval: timesync.DefaultNetworkUpPollInterval,
			})
			go mdnsresponder.Run(mdnsresponderDeps(log, mdnsChanged), mdnsresponder.Options{Hostname: cfg.Hostname})

			wifiClient, err := wifiup.NewPlatform()
			if err != nil {
				// Expected on an Ethernet-only board with no WiFi
				// hardware/driver at all; not fatal to boot.
				log("WiFi unavailable, skipping: %v", err)
				return
			}
			wifiup.Run(wifiupDeps(wifiClient, cfg, gosdToml.Wifi, log, mdnsChanged), wifiup.Options{})
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

	// Run only returns once the fatal (log+sync+sleep+reboot) path has
	// already been triggered, or the machine has rebooted out from under
	// us; either way there's nothing left for main to do.
	_ = boot.Run(deps, opts)
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

// readGosdToml reads and parses /boot/gosd.toml, the hand-editable fallback
// config on the GOSD-BOOT partition. The file is entirely optional — a
// missing file is not logged as a problem at all, since most users will
// never touch it — but a present-and-unreadable-as-TOML file (a typo from
// hand-editing) is surfaced as an error for boot.Run to log as a warning;
// either way, boot never fails over it.
func readGosdToml() (gosdtoml.Config, error) {
	data, err := os.ReadFile(gosdTomlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return gosdtoml.Config{}, nil
		}
		return gosdtoml.Config{}, err
	}
	return gosdtoml.Parse(data)
}

// ensureDataMountpoint creates /data on the RAM-backed rootfs so the
// GOSD-DATA partition has somewhere to mount; the initramfs archive doesn't
// contain empty directories.
func ensureDataMountpoint() error {
	return os.MkdirAll(dataTarget, 0o755)
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
// renewed) — see mdnsresponderDeps and gosd-r796.
func netupDeps(log func(format string, args ...any), changed *mdnsresponder.Signal) netup.Deps {
	platform := netup.NewPlatform()
	return netup.Deps{
		Links:           platform.Links,
		DHCP:            platform.DHCP,
		Clock:           netup.NewRealClock(),
		NewBackoff:      func() *netup.Backoff { return netup.NewBackoff(netup.DefaultBackoffBase, netup.DefaultBackoffCap) },
		WriteResolvConf: func(dns []net.IP) error { return netup.WriteResolvConf(netup.DefaultResolvConfPath, dns) },
		MarkNetworkUp: func() error {
			err := netup.MarkNetworkUp(netup.DefaultNetworkUpPath)
			changed.Notify()
			return err
		},
		ClearNetworkUp: func() error {
			err := netup.ClearNetworkUp(netup.DefaultNetworkUpPath)
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
// wireless — and the credential source: config.json's wifi block, unless
// gosd.toml hand-edits one in, in which case that takes precedence. changed
// is wired the same way netupDeps wires it: see that function's doc.
func wifiupDeps(client wifiup.WifiClient, cfg initcfg.Config, gosdWifi gosdtoml.Wifi, log func(format string, args ...any), changed *mdnsresponder.Signal) wifiup.Deps {
	platform := netup.NewPlatform()
	return wifiup.Deps{
		Wifi:            client,
		Credentials:     wifiup.ConfigCredentials{Wifi: cfg.Wifi, GosdToml: gosdWifi},
		Links:           platform.Links,
		DHCP:            platform.DHCP,
		Clock:           netup.NewRealClock(),
		NewBackoff:      func() *netup.Backoff { return netup.NewBackoff(netup.DefaultBackoffBase, netup.DefaultBackoffCap) },
		WriteResolvConf: func(dns []net.IP) error { return netup.WriteResolvConf(netup.DefaultResolvConfPath, dns) },
		MarkNetworkUp: func() error {
			err := netup.MarkNetworkUp(netup.DefaultNetworkUpPath)
			changed.Notify()
			return err
		},
		ClearNetworkUp: func() error {
			err := netup.ClearNetworkUp(netup.DefaultNetworkUpPath)
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
