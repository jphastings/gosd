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
