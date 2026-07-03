// Command gosd-init is PID 1 on a gosd image: a static Go binary that runs
// as /init from the initramfs, brings up the board, and supervises the
// user's application for the life of the device. There is no shell, no
// busybox, no interactive surface of any kind — if gosd-init can't do
// something in Go, it doesn't happen.
package main

import (
	"os"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/boot"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/initcfg"
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
