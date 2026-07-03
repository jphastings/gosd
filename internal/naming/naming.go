// Package naming sanitizes free-form strings (like a main package's
// basename) into the restricted character set gosd uses for hostnames and
// output filenames.
package naming

import (
	"regexp"
	"strings"
)

var invalidRun = regexp.MustCompile(`[^a-z0-9-]+`)
var dashRun = regexp.MustCompile(`-+`)

// Sanitize lowercases s and restricts it to [a-z0-9-], collapsing runs of
// disallowed characters into a single hyphen and trimming leading/trailing
// hyphens. If nothing usable remains, it returns "app".
func Sanitize(s string) string {
	lowered := strings.ToLower(s)
	replaced := invalidRun.ReplaceAllString(lowered, "-")
	collapsed := dashRun.ReplaceAllString(replaced, "-")
	trimmed := strings.Trim(collapsed, "-")
	if trimmed == "" {
		return "app"
	}
	return trimmed
}
