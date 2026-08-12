package secretreg

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseReturnsRulesForEachRegistration(t *testing.T) {
	data := []byte(`[
		{"secret": "sk_live_51H_abcdefg", "replacement": "stripe-api-key"},
		{"secret": "another-long-secret-value", "replacement": "session-token"}
	]`)

	rules := Parse(data)

	if len(rules) != 2 {
		t.Fatalf("Parse() returned %d rules, want 2", len(rules))
	}
	if rules[0].Needle != "sk_live_51H_abcdefg" || rules[0].Replacement != "{secret: stripe-api-key}" {
		t.Errorf("rules[0] = %+v, want Needle sk_live_51H_abcdefg / Replacement {secret: stripe-api-key}", rules[0])
	}
	if rules[1].Needle != "another-long-secret-value" || rules[1].Replacement != "{secret: session-token}" {
		t.Errorf("rules[1] = %+v, want Needle another-long-secret-value / Replacement {secret: session-token}", rules[1])
	}
}

func TestParseDropsTheWholeFileRatherThanTrustPartially(t *testing.T) {
	// The self-heal lesson from gosd-6cf2: an untrustworthy state file is
	// dropped entirely, never partially believed.
	cases := map[string]string{
		"empty":           "",
		"malformed JSON":  `[{"secret": "abc"`,
		"wrong shape":     `{"secret": "abc", "replacement": "x"}`,
		"not JSON at all": "definitely not json",
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if rules := Parse([]byte(data)); rules != nil {
				t.Errorf("Parse(%q) = %v, want nil", data, rules)
			}
		})
	}
}

func TestParseTruncatesRatherThanDropsTooManyRegistrations(t *testing.T) {
	// Unlike an unparseable/empty/oversized file, a file that names one
	// registration too many is still fully trustworthy data: dropping
	// every entry over one policy limit would turn a resource bound into
	// a redaction outage, so Parse keeps as many as the bound allows
	// rather than none.
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i <= MaxRegistrations; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"secret": "secret-value-number-%02d", "replacement": "label-%02d"}`, i, i)
	}
	b.WriteString("]")

	rules := Parse([]byte(b.String()))
	if len(rules) != MaxRegistrations {
		t.Fatalf("Parse() with %d registrations returned %d rules, want exactly MaxRegistrations=%d", MaxRegistrations+1, len(rules), MaxRegistrations)
	}
	if rules[0].Needle != "secret-value-number-00" {
		t.Errorf("Parse() kept the wrong entries: rules[0] = %+v, want the first-registered secret", rules[0])
	}
}

func TestParseAcceptsExactlyMaxRegistrations(t *testing.T) {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < MaxRegistrations; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"secret": "some-secret-value", "replacement": "label"}`)
	}
	b.WriteString("]")

	if rules := Parse([]byte(b.String())); len(rules) != MaxRegistrations {
		t.Errorf("Parse() with exactly MaxRegistrations returned %d rules, want %d", len(rules), MaxRegistrations)
	}
}

func TestParseDropsOversizedFile(t *testing.T) {
	oversized := append([]byte(`[{"secret": "x", "replacement": "`), make([]byte, MaxTotalBytes+1)...)
	oversized = append(oversized, []byte(`"}]`)...)

	if rules := Parse(oversized); rules != nil {
		t.Errorf("Parse() on an oversized file = %v, want nil", rules)
	}
}

func TestEncodeWritesWhatParseReadsBack(t *testing.T) {
	entries := []Entry{{Secret: "sk_live_51H_abcdefg", Replacement: "stripe-api-key"}}

	data, err := Encode(entries)
	if err != nil {
		t.Fatal(err)
	}

	rules := Parse(data)
	if len(rules) != 1 || rules[0].Needle != entries[0].Secret || rules[0].Replacement != Label(entries[0].Replacement) {
		t.Errorf("Parse(Encode(entries)) = %+v, want the entry back under its label", rules)
	}
}

func TestEncodeRefusesWhatParseWouldNotReadBackWhole(t *testing.T) {
	// Both bounds matter to the writer, for different reasons: past
	// MaxRegistrations Parse silently drops the tail, and past
	// MaxTotalBytes it drops every entry in the file — so a writer that
	// let either through would cause a redaction outage rather than lose
	// one registration.
	tooMany := make([]Entry, MaxRegistrations+1)
	for i := range tooMany {
		tooMany[i] = Entry{Secret: fmt.Sprintf("secret-value-number-%02d", i), Replacement: "label"}
	}
	if _, err := Encode(tooMany); err == nil {
		t.Errorf("Encode() accepted %d registrations, want a refusal past %d", len(tooMany), MaxRegistrations)
	}

	if _, err := Encode([]Entry{{Secret: strings.Repeat("s", MaxTotalBytes), Replacement: "enormous"}}); err == nil {
		t.Errorf("Encode() accepted an entry larger than the %d bytes Parse will read", MaxTotalBytes)
	}
}

func TestLabelNamesTheReplacementNotTheSecret(t *testing.T) {
	if got, want := Label("stripe-api-key"), "{secret: stripe-api-key}"; got != want {
		t.Errorf("Label(%q) = %q, want %q", "stripe-api-key", got, want)
	}
}
