// Package naming sanitizes free-form strings (like a main package's
// basename) into the restricted character set gosd uses for hostnames,
// output filenames, and the per-app partition volume labels an image is
// built with (see LabelPrefix and LabelsFor).
package naming

import (
	"regexp"
	"strings"
)

// MaxLength is the longest string Sanitize will ever return. It matches
// sethostname(2)'s 64-byte limit (63 usable bytes plus the kernel's NUL
// terminator) — the tightest constraint among Sanitize's consumers — so a
// single cap keeps hostnames, FAT label components, and output filenames
// all safely under it.
const MaxLength = 63

var invalidRun = regexp.MustCompile(`[^a-z0-9-]+`)
var dashRun = regexp.MustCompile(`-+`)

// Sanitize lowercases s and restricts it to [a-z0-9-], collapsing runs of
// disallowed characters into a single hyphen, trimming leading/trailing
// hyphens, and capping the result at MaxLength bytes (re-trimming any
// hyphen the cap exposes at the new end). If nothing usable remains, it
// returns "app".
func Sanitize(s string) string {
	lowered := strings.ToLower(s)
	replaced := invalidRun.ReplaceAllString(lowered, "-")
	collapsed := dashRun.ReplaceAllString(replaced, "-")
	trimmed := strings.Trim(collapsed, "-")
	if trimmed == "" {
		return "app"
	}
	if len(trimmed) > MaxLength {
		trimmed = strings.Trim(trimmed[:MaxLength], "-")
		if trimmed == "" {
			return "app"
		}
	}
	return trimmed
}

// ValidHostname reports whether name can be used as a device's hostname
// exactly as written: non-empty, and left unchanged by Sanitize — which
// means it already satisfies both of Sanitize's constraints, the [a-z0-9-]
// charset and the MaxLength byte cap that sethostname(2) enforces.
//
// It lives here, beside Sanitize, so that every place a hostname crosses
// from something somebody typed into something the device acts on shares
// one definition of "valid": gosd-init's config-tree gate and
// internal/hostsfile's /etc/hosts renderer both call it. Two copies of this
// rule is how a hostname carrying a newline reached /etc/hosts unescaped
// (bean gosd-39da); one copy is how it stays out.
//
// A name that fails is never silently rewritten to fit (see gosd-jeaw):
// mangling what somebody typed only confuses them, so callers refuse the
// value and say which file to fix.
func ValidHostname(name string) bool {
	return name != "" && Sanitize(name) == name
}
