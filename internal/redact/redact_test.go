package redact

import (
	"strings"
	"testing"
)

func TestRedact_ReplacesNeedles(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		rules []Rule
		want  string
	}{
		{
			name:  "needle appearing many times",
			body:  "key=sk_live_abcdef01 seen twice: sk_live_abcdef01",
			rules: []Rule{{Needle: "sk_live_abcdef01", Replacement: "{$API_KEY}"}},
			want:  "key={$API_KEY} seen twice: {$API_KEY}",
		},
		{
			name: "needles shorter than the threshold are left in place",
			body: "PORT=80 DEBUG=1",
			rules: []Rule{
				{Needle: "80", Replacement: "{$PORT}"},
				{Needle: "1", Replacement: "{$DEBUG}"},
			},
			want: "PORT=80 DEBUG=1",
		},
		{
			name:  "empty needle is skipped, not applied everywhere",
			body:  "unchanged",
			rules: []Rule{{Needle: "", Replacement: "{$EMPTY}"}},
			want:  "unchanged",
		},
		{
			name:  "empty body",
			body:  "",
			rules: []Rule{{Needle: "longenoughsecret", Replacement: "{$S}"}},
			want:  "",
		},
		{
			name:  "multi-byte UTF-8 content",
			body:  "clé secrète : pässwörd12345 ✅",
			rules: []Rule{{Needle: "pässwörd12345", Replacement: "{$MOTDEPASSE}"}},
			want:  "clé secrète : {$MOTDEPASSE} ✅",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Redact(tc.body, tc.rules).Body; got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// The threshold is a boundary, not a vibe: exactly MinNeedleLength bytes is
// applied, one byte short is skipped.
func TestRedact_MinNeedleLengthBoundary(t *testing.T) {
	atThreshold := strings.Repeat("x", MinNeedleLength)
	belowThreshold := strings.Repeat("y", MinNeedleLength-1)
	body := atThreshold + " " + belowThreshold

	res := Redact(body, []Rule{
		{Needle: atThreshold, Replacement: "{$AT}"},
		{Needle: belowThreshold, Replacement: "{$BELOW}"},
	})

	if want := "{$AT} " + belowThreshold; res.Body != want {
		t.Errorf("Redact() = %q, want %q", res.Body, want)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "{$BELOW}" {
		t.Errorf("Skipped = %v, want [{$BELOW}]", res.Skipped)
	}
}

// The correctness property most worth testing: a secret that contains
// another secret as a substring must be redacted whole, never leaving a
// fragment of the longer one exposed around the shorter one's replacement.
func TestRedact_LongestNeedleFirst(t *testing.T) {
	long := "sk_live_abcdef0123456789"
	short := "abcdef0123456789" // a substring of long

	got := Redact("token: "+long, []Rule{
		{Needle: short, Replacement: "{$SHORT}"},
		{Needle: long, Replacement: "{$LONG}"},
	}).Body

	if want := "token: {$LONG}"; got != want {
		t.Errorf("Redact() = %q, want %q", got, want)
	}
	if strings.Contains(got, "sk_live_") || strings.Contains(got, short) {
		t.Errorf("Redact() left a fragment of the longer secret: %q", got)
	}
}

// Two same-length secrets whose occurrences overlap in the body: redacting
// the first consumes the shared span, so the second's occurrence there is
// gone rather than partially replaced into a leftover fragment.
func TestRedact_OverlappingNeedles(t *testing.T) {
	first := "AAAABBBB"
	second := "BBBBCCCC"
	body := "prefix AAAABBBBCCCC suffix"

	res := Redact(body, []Rule{
		{Needle: first, Replacement: "{$FIRST}"},
		{Needle: second, Replacement: "{$SECOND}"},
	})

	if want := "prefix {$FIRST}CCCC suffix"; res.Body != want {
		t.Errorf("Redact() = %q, want %q", res.Body, want)
	}
	if strings.Contains(res.Body, first) || strings.Contains(res.Body, second) {
		t.Errorf("Redact() left an overlapping secret intact: %q", res.Body)
	}
}

func TestRedact_SkippedNeverCarriesTheNeedle(t *testing.T) {
	res := Redact("DEBUG=1 PORT=80", []Rule{
		{Needle: "1", Replacement: "{$DEBUG}"},
		{Needle: "80", Replacement: "{$PORT}"},
	})

	want := []string{"{$DEBUG}", "{$PORT}"}
	if len(res.Skipped) != len(want) {
		t.Fatalf("Skipped = %v, want %v", res.Skipped, want)
	}
	for i, r := range want {
		if res.Skipped[i] != r {
			t.Errorf("Skipped[%d] = %q, want %q", i, res.Skipped[i], r)
		}
	}
	// Result.Skipped's type ([]string of Replacement) makes this
	// structurally true, not just true for this input — there is no
	// field on Result a caller could log to reach a Needle value.
}

// The negative claim this package exists to make: once a needle clears the
// threshold and is applied, it does not survive redaction anywhere in the
// output bytes.
func TestRedact_SecretDoesNotSurvive(t *testing.T) {
	secret := "sk_live_51H8xyzABCDEFGHIJKLMNOP"
	body := "starting up\n" +
		"API key " + secret + " rejected\n" +
		"retrying with " + secret + " again\n"

	res := Redact(body, []Rule{{Needle: secret, Replacement: "{$STRIPE_KEY}"}})

	if strings.Contains(res.Body, secret) {
		t.Fatalf("secret survived redaction: %q", res.Body)
	}
}

// Two rules sharing the exact same Needle but different Replacements is the
// one case where a rule's position in the list still decides the outcome —
// documented on Redact, and pinned here so a future change can't silently
// flip which Replacement wins without a test noticing.
func TestRedact_DuplicateNeedleUsesFirstRule(t *testing.T) {
	secret := "duplicatedsecret789"

	res := Redact(secret, []Rule{
		{Needle: secret, Replacement: "{$FIRST}"},
		{Needle: secret, Replacement: "{$SECOND}"},
	})

	if want := "{$FIRST}"; res.Body != want {
		t.Errorf("Redact() = %q, want %q", res.Body, want)
	}
}

func TestRedact_Deterministic(t *testing.T) {
	rules := []Rule{
		{Needle: "aaaaaaaa", Replacement: "{$A}"},
		{Needle: "bbbbbbbb", Replacement: "{$B}"},
	}
	body := "aaaaaaaa and bbbbbbbb"

	want := Redact(body, rules).Body
	for i := 0; i < 5; i++ {
		if got := Redact(body, rules).Body; got != want {
			t.Fatalf("Redact() is non-deterministic: %q vs %q", got, want)
		}
	}
}
