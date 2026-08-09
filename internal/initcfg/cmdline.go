package initcfg

import "strings"

// CmdlineArgs holds the gosd-specific kernel command-line parameters
// gosd-init understands. The kernel command line is a space-separated list
// of "key" or "key=value" tokens; every other token (console=, root=, ...)
// is ignored.
type CmdlineArgs struct {
	// Board overrides Config.Board when non-empty, from gosd.board=<id>.
	Board string
	// BootDev names the disk the bootloader actually loaded the kernel
	// from, from gosd.bootdev=<name>: a kernel block-device name with an
	// optional /dev/ prefix (e.g. vda, mmcblk1, /dev/mmcblk1). gosd-init
	// uses it to probe only that disk's partitions for the boot partition, so a
	// stale GoSD image on another medium (eMMC vs SD) can't win by
	// device-name order. Empty when the bootloader can't supply it — all
	// current real-hardware images — in which case the probe walks every
	// candidate as before.
	BootDev string
	// Debug enables verbose logging, from a bare gosd.debug or
	// gosd.debug=<truthy value>.
	Debug bool
}

// ParseCmdline parses the contents of /proc/cmdline (or an equivalent
// string) for the gosd.board, gosd.bootdev and gosd.debug parameters.
func ParseCmdline(cmdline string) CmdlineArgs {
	var args CmdlineArgs
	for _, tok := range strings.Fields(cmdline) {
		key, value, hasValue := strings.Cut(tok, "=")
		switch key {
		case "gosd.board":
			if hasValue {
				args.Board = value
			}
		case "gosd.bootdev":
			if hasValue {
				args.BootDev = value
			}
		case "gosd.debug":
			args.Debug = !hasValue || isTruthy(value)
		}
	}
	return args
}

// isTruthy treats gosd.debug=0/false/no/off as disabling debug mode, and any
// other value (including gosd.debug=1 or gosd.debug=yes) as enabling it.
func isTruthy(v string) bool {
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
