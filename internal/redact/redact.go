// Package redact replaces secret values in text with safe placeholders. It
// is the pure core behind LAST_FATAL_ERROR.md's scrub pass (bean gosd-m6py,
// epic gosd-47z3): gosd-init and the public fault package each feed it
// (needle, replacement) pairs — the app's env var values, the ingress
// credentials the card carries, and fault.RegisterSecretString's
// registrations — and it returns text with every occurrence of every needle
// replaced.
//
// This package only does the replacement, and it does it one string at a
// time: [New] prepares a rule set once and [Redactor.Apply] runs it over as
// many strings as the caller has. That shape is deliberate. Its caller,
// internal/faultreport, redacts the individual values a report carries in
// from outside — never the assembled document, which is mostly gosd's own
// compile-time prose that cannot hold a secret and can only be damaged by
// being rewritten (bean gosd-fu1z).
//
// Discovering which values count as secrets is the callers' job: see
// cmd/gosd-init/internal/boot's envRedactionRules and ingressRedactionRules,
// and internal/secretreg for the /run registration file.
package redact

import (
	"sort"
	"strings"
)

// MinNeedleLength is the shortest needle, measured in bytes, that a
// Redactor will act on. A needle shorter than this is skipped rather than
// applied.
//
// Over-redaction, not a missed match, is the failure mode that matters here:
// gosd-init redacts every value in the app's environment, and short,
// ordinary config values collide constantly with substrings that appear
// throughout an otherwise-innocent report — DEBUG=1 would blank every "1"
// in a stack trace's line numbers, PORT=80 every "80" in a byte count,
// turning a readable crash report into confetti. Genuine secrets (API keys,
// tokens, passwords worth protecting) are conventionally well above this
// length; short, deliberately-chosen values generally are not the kind of
// thing this mechanism exists to protect.
//
// This cuts both ways: a genuinely short secret is not redacted below this
// floor. [Redactor.Skipped] exists so a caller can log that a value was
// left alone, without logging the value itself.
//
// The floor is unconditional across callers: a secret deliberately
// registered through fault.RegisterSecretString is skipped too if it's
// shorter than this. That's intended, not an oversight — a caller that
// needs to know uses [Redactor.Skipped].
//
// The floor is a blunt instrument against collisions, and it is a little
// blunter than it needs to be now that gosd's own boilerplate is never
// redacted at all (bean gosd-fu1z): the worst collisions it was chosen to
// prevent were with fixed prose that redaction no longer touches. Lowering
// it is still its own decision, with its own evidence, and nobody has made
// it — a short value colliding with a stack trace's own text is untouched
// by that argument.
const MinNeedleLength = 8

// Rule pairs a secret to find with the text that replaces it.
//
// Replacement must be safe to appear in a document its reader might forward
// to a stranger — it names what was removed without being a secret itself
// ("{$STRIPE_KEY}", "{secret: stripe-api-key}"), never a second value that
// itself needs protecting. This package relies on that: [Redactor.Skipped]
// reports Replacement, never Needle, for exactly this reason.
type Rule struct {
	Needle      string
	Replacement string
}

// Redactor is a rule set prepared for use, holding the rules long enough to
// redact any number of strings with them. The zero Redactor is valid and
// changes nothing.
type Redactor struct {
	applied []Rule
	skipped []string
}

// New prepares rules for application, applying [MinNeedleLength] and
// ordering the survivors longest needle first.
//
// Needles are applied longest first, regardless of their position in rules.
// This is what keeps a secret that contains a shorter secret as a substring
// from being partially redacted: replacing the longer needle first removes
// the embedded occurrence of the shorter one along with it, rather than
// leaving a mangled fragment of the longer secret exposed around the
// shorter one's replacement. Once a needle is matched and replaced, that
// span of the text is gone — a shorter, overlapping needle that only
// matched inside it will no longer be found there, which is correct: the
// secret it was part of has already been removed.
//
// Deciding all of this once, here, is also what lets a caller redact many
// strings and still get one answer about what was skipped: [Redactor.Skipped]
// describes the rules, not any particular string they were applied to.
//
// The one case where the position of a rule in rules still matters is two
// rules sharing the exact same Needle with different Replacements — the
// first one in rules is the Replacement that gets used, because by the time
// the second is applied nothing is left to match.
func New(rules []Rule) Redactor {
	r := Redactor{applied: make([]Rule, 0, len(rules))}
	for _, rule := range rules {
		if len(rule.Needle) < MinNeedleLength {
			r.skipped = append(r.skipped, rule.Replacement)
			continue
		}
		r.applied = append(r.applied, rule)
	}

	sort.SliceStable(r.applied, func(i, j int) bool {
		return len(r.applied[i].Needle) > len(r.applied[j].Needle)
	})
	return r
}

// Apply returns text with every occurrence of every applied rule's Needle
// replaced by its Replacement. It is pure: the same text always yields the
// same result, and applying a Redactor to one string never changes what it
// does to the next.
func (r Redactor) Apply(text string) string {
	for _, rule := range r.applied {
		text = strings.ReplaceAll(text, rule.Needle, rule.Replacement)
	}
	return text
}

// Skipped carries the Replacement — never the Needle — of every rule whose
// Needle was shorter than [MinNeedleLength], in the order the rules were
// supplied. Its shape makes logging it safe by construction: there is no
// value returned here that a caller could log that would leak a secret.
func (r Redactor) Skipped() []string {
	return r.skipped
}
