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
	"github.com/jphastings/gosd/cmd/gosd-init/internal/netup"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/wifiup"
	"github.com/jphastings/gosd/internal/initcfg"
)

const (
	configPath  = "/etc/gosd/config.json"
	cmdlinePath = "/proc/cmdline"
	appPath     = "/app"
	bootTarget  = "/boot"

	// bootMountTimeout bounds how long gosd-init retries mounting the
	// GOSD-BOOT partition: the MMC controller may still be probing when
	// gosd-init reaches this step, and there's no udev to wait on.
	bootMountTimeout = 10 * time.Second
)

// bootDevices are the candidate device nodes for the GOSD-BOOT FAT
// partition, tried in order, with no udev available to discover it.
var bootDevices = []string{"/dev/mmcblk0p1", "/dev/mmcblk1p1"}

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
		Sleep:       time.Sleep,
		Now:         time.Now,
		StartNetworking: func(cfg initcfg.Config, log func(format string, args ...any)) {
			go netup.Run(netupDeps(log), netup.Options{})

			wifiClient, err := wifiup.NewPlatform()
			if err != nil {
				// Expected on an Ethernet-only board with no WiFi
				// hardware/driver at all; not fatal to boot.
				log("WiFi unavailable, skipping: %v", err)
				return
			}
			wifiup.Run(wifiupDeps(wifiClient, cfg, log), wifiup.Options{})
		},
	}
	opts := boot.Options{
		AppPath:     appPath,
		BootTarget:  bootTarget,
		BootDevices: bootDevices,
		BootTimeout: bootMountTimeout,
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

// fallbackLog is used before /dev/console is open (or if opening it fails).
func fallbackLog(format string, args ...any) {
	boot.NewLogger(os.Stderr).Printf(format, args...)
}

// netupDeps wires the real, netlink/DHCP-backed networking implementation,
// logging through log (boot's console logger, once available).
func netupDeps(log func(format string, args ...any)) netup.Deps {
	platform := netup.NewPlatform()
	return netup.Deps{
		Links:           platform.Links,
		DHCP:            platform.DHCP,
		Clock:           netup.NewRealClock(),
		NewBackoff:      func() *netup.Backoff { return netup.NewBackoff(netup.DefaultBackoffBase, netup.DefaultBackoffCap) },
		WriteResolvConf: func(dns []net.IP) error { return netup.WriteResolvConf(netup.DefaultResolvConfPath, dns) },
		MarkNetworkUp:   func() error { return netup.MarkNetworkUp(netup.DefaultNetworkUpPath) },
		ClearNetworkUp:  func() error { return netup.ClearNetworkUp(netup.DefaultNetworkUpPath) },
		Log:             log,
	}
}

// wifiupDeps wires the real, nl80211-backed WiFi implementation (client)
// together with the same netlink/DHCP building blocks netupDeps uses —
// DHCP itself doesn't care whether the underlying medium is wired or
// wireless — and config.json's wifi block as the (v0.1) credential
// source.
func wifiupDeps(client wifiup.WifiClient, cfg initcfg.Config, log func(format string, args ...any)) wifiup.Deps {
	platform := netup.NewPlatform()
	return wifiup.Deps{
		Wifi:            client,
		Credentials:     wifiup.ConfigCredentials{Wifi: cfg.Wifi},
		Links:           platform.Links,
		DHCP:            platform.DHCP,
		Clock:           netup.NewRealClock(),
		NewBackoff:      func() *netup.Backoff { return netup.NewBackoff(netup.DefaultBackoffBase, netup.DefaultBackoffCap) },
		WriteResolvConf: func(dns []net.IP) error { return netup.WriteResolvConf(netup.DefaultResolvConfPath, dns) },
		MarkNetworkUp:   func() error { return netup.MarkNetworkUp(netup.DefaultNetworkUpPath) },
		ClearNetworkUp:  func() error { return netup.ClearNetworkUp(netup.DefaultNetworkUpPath) },
		Log:             log,
	}
}
