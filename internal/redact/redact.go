// Package redact replaces secret values in a body of text with safe
// placeholders. It is the pure core behind LAST_FATAL_ERROR.md's scrub pass
// (bean gosd-m6py, epic gosd-47z3): gosd-init and the public fault package
// will each feed it (needle, replacement) pairs — the app's env var values
// and fault.RegisterSecretString registrations, respectively — and it
// returns the body with every occurrence of every needle replaced.
//
// This package only does the replacement. Discovering which values count as
// secrets (scanning the app's environment, reading fault's /run registration
// file) and wiring the result into the crash-report renderer are separate
// work, not yet implemented here — see gosd-m6py's remaining todos.
package redact

import (
	"sort"
	"strings"
)

// MinNeedleLength is the shortest needle, measured in bytes, that Redact
// will act on. A needle shorter than this is skipped rather than applied.
//
// Over-redaction, not a missed match, is the failure mode that matters here:
// gosd-init's plan is to redact every value in the app's environment, and
// short, ordinary config values collide constantly with substrings that
// appear throughout an otherwise-innocent report — DEBUG=1 would blank
// every "1" in a stack trace's line numbers, PORT=80 every "80" in a byte
// count, turning a readable crash report into confetti. Genuine secrets
// (API keys, tokens, passwords worth protecting) are conventionally well
// above this length; short, deliberately-chosen values generally are not
// the kind of thing this mechanism exists to protect.
//
// This cuts both ways: a genuinely short secret is not redacted below this
// floor. Result.Skipped exists so a caller can log that a value was left
// alone, without logging the value itself.
const MinNeedleLength = 8

// Rule pairs a secret to find with the text that replaces it.
//
// Replacement must be safe to appear in a document its reader might forward
// to a stranger — it names what was removed without being a secret itself
// ("{$STRIPE_KEY}", "{secret: stripe-api-key}"), never a second value that
// itself needs protecting. Redact relies on this: Result.Skipped reports
// Replacement, never Needle, for exactly this reason.
type Rule struct {
	Needle      string
	Replacement string
}

// Result is the outcome of a Redact call.
type Result struct {
	// Body is the input with every applied rule's Needle replaced by its
	// Replacement.
	Body string

	// Skipped carries the Replacement — never the Needle — of every Rule
	// whose Needle was shorter than MinNeedleLength, in the order the
	// rules were supplied. Its shape makes logging it safe by
	// construction: there is no field here a caller could log that would
	// leak a secret value.
	Skipped []string
}

// Redact replaces every occurrence of each rule's Needle in body with its
// Replacement, and reports which rules were skipped for being too short to
// safely act on (see MinNeedleLength).
//
// Needles are applied longest first, regardless of their position in rules.
// This is what keeps a secret that contains a shorter secret as a substring
// from being partially redacted: replacing the longer needle first removes
// the embedded occurrence of the shorter one along with it, rather than
// leaving a mangled fragment of the longer secret exposed around the
// shorter one's replacement. Once a needle is matched and replaced, that
// span of the body is gone — a shorter, overlapping needle that only
// matched inside it will no longer be found there, which is correct: the
// secret it was part of has already been removed.
//
// Redact is pure: calling it twice with the same body and rules always
// produces the same Result. The one case where the position of a rule in
// rules still matters is two rules sharing the exact same Needle with
// different Replacements — the first one in rules is the Replacement that
// gets used, because by the time the second is applied nothing is left to
// match.
func Redact(body string, rules []Rule) Result {
	applied := make([]Rule, 0, len(rules))
	var skipped []string
	for _, rule := range rules {
		if len(rule.Needle) < MinNeedleLength {
			skipped = append(skipped, rule.Replacement)
			continue
		}
		applied = append(applied, rule)
	}

	sort.SliceStable(applied, func(i, j int) bool {
		return len(applied[i].Needle) > len(applied[j].Needle)
	})

	for _, rule := range applied {
		body = strings.ReplaceAll(body, rule.Needle, rule.Replacement)
	}

	return Result{Body: body, Skipped: skipped}
}
