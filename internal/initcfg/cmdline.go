package initcfg

import "strings"

// CmdlineArgs holds the gosd-specific kernel command-line parameters
// gosd-init understands. The kernel command line is a space-separated list
// of "key" or "key=value" tokens; every other token (console=, root=, ...)
// is ignored.
type CmdlineArgs struct {
	// Board overrides Config.Board when non-empty, from gosd.board=<id>.
	Board string
	// Debug enables verbose logging, from a bare gosd.debug or
	// gosd.debug=<truthy value>.
	Debug bool
}

// ParseCmdline parses the contents of /proc/cmdline (or an equivalent
// string) for the gosd.board and gosd.debug parameters.
func ParseCmdline(cmdline string) CmdlineArgs {
	var args CmdlineArgs
	for _, tok := range strings.Fields(cmdline) {
		key, value, hasValue := strings.Cut(tok, "=")
		switch key {
		case "gosd.board":
			if hasValue {
				args.Board = value
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
