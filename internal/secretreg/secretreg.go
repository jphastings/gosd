// Package secretreg defines the format of the registration file
// fault.RegisterSecretString (bean gosd-aa1p) writes to /run, and reads it
// back: on a panic the app never gets to hand anything over, so a secret
// registered only in the app's own memory is exactly the one still in the
// crash report gosd-init is about to write. Writing it through to tmpfs
// immediately, on every call, is what lets gosd-init redact a crash it
// never saw coming.
//
// # File format
//
// [Path] is /run/gosd/secrets.json — /run is tmpfs, already mounted by
// gosd-init's early mounts. The file holds the WHOLE current set of
// registrations, replaced (never appended to) on every
// fault.RegisterSecretString call: a JSON array of objects, each one that
// call's two arguments, unchanged:
//
//	[
//	  {"secret": "sk_live_51H...", "replacement": "stripe-api-key"},
//	  {"secret": "eyJhbGciOi...", "replacement": "session-token"}
//	]
//
// Mode 0600 — this is plaintext secrets on root-owned tmpfs, and no other
// process on the device has a reason to read it. The writer must write to
// Path+".tmp" and rename it into place, the same write-then-rename
// discipline as every other file in this codebase that must never be read
// half-written (e.g. cmd/gosd-init/internal/provsnapshot.WriteFileDurably):
// gosd-init only ever reads Path itself, so a reader can never observe a
// partial write, only the complete old file or the complete new one.
//
// # What a reader must assume
//
// [Parse] drops the file wholesale — treats it as no registrations at all —
// rather than partially trust anything from a file that is empty, larger
// than [MaxTotalBytes], or doesn't parse as this exact shape. This is the
// gosd-6cf2 self-heal lesson generalised: a state file that can wedge a
// reader on one bad byte is worse than one that's occasionally ignored, and
// there is no safe way to guess at meaning from bytes that don't parse.
//
// A file that DOES parse but names more than [MaxRegistrations] secrets is
// different: the data is trustworthy, just larger than policy allows, so
// Parse keeps the file's first MaxRegistrations entries rather than
// discarding all of them — dropping every registration because of one
// registered too many would turn a resource bound into a redaction outage,
// which is the opposite of what this package is for.
package secretreg

import (
	"encoding/json"

	"github.com/jphastings/gosd/internal/redact"
)

// Path is where fault.RegisterSecretString (bean gosd-aa1p) writes the
// registration file, and where gosd-init reads it back at crash-report
// time — fresh on every report, not once at boot, so a registration made
// moments before a crash still counts. See the package doc for the file's
// exact format and the write-then-rename discipline the writer must follow.
const Path = "/run/gosd/secrets.json"

// MaxRegistrations bounds how many entries [Parse] will trust from one
// file. It's generous for any realistic set of hand-registered secrets (API
// keys, tokens, credentials an app happens to hold) while still being a
// real bound: an app that registers in a loop must not be able to grow this
// file without limit.
const MaxRegistrations = 64

// MaxTotalBytes bounds the registration file's size that [Parse] will
// trust. /run is tmpfs — memory, shared with everything else gosd-init and
// the app keep there — so this exists to keep a runaway registration loop
// from pressuring it meaningfully, not because legitimate use ever
// approaches it: 64 entries of realistic secret/label lengths land nowhere
// close to this.
const MaxTotalBytes = 64 * 1024

// registration is one entry in the file: RegisterSecretString's two
// arguments, unchanged.
type registration struct {
	Secret      string `json:"secret"`
	Replacement string `json:"replacement"`
}

// Label formats a registered secret's human replacement exactly the way
// every crash report renders it, so gosd-init's on-crash read and the fault
// package's own off-device rendering (go test, a developer's Mac) produce
// byte-identical output. Exported so RegisterSecretString's off-device path
// calls this too, rather than a second copy of "{secret: ...}" drifting
// from this one.
func Label(replacement string) string {
	return "{secret: " + replacement + "}"
}

// Parse turns the raw bytes of the registration file into redaction rules.
// It drops the whole file — never partially trusting it — when the bytes
// are empty, larger than [MaxTotalBytes], or don't parse as the documented
// shape at all; see the package doc for why. A file that parses but names
// more than [MaxRegistrations] secrets is truncated to the first
// MaxRegistrations entries in file order rather than dropped outright, on
// the same reasoning. A secret shorter than redact.MinNeedleLength is still
// passed through as a rule: redact.Redact applies that floor uniformly to
// every caller, this one included, and reports the skip itself.
func Parse(data []byte) []redact.Rule {
	if len(data) == 0 || len(data) > MaxTotalBytes {
		return nil
	}

	var entries []registration
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	if len(entries) > MaxRegistrations {
		entries = entries[:MaxRegistrations]
	}

	rules := make([]redact.Rule, 0, len(entries))
	for _, e := range entries {
		rules = append(rules, redact.Rule{Needle: e.Secret, Replacement: Label(e.Replacement)})
	}
	return rules
}
