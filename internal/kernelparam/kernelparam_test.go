package kernelparam_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/kernelparam"
)

func TestParseAcceptsAnyWellShapedParameter(t *testing.T) {
	// Deliberately a mix gosd knows nothing about: a module parameter, a
	// bare switch, a value with punctuation, and one no board here has a
	// kernel for. Accepting the unknown ones is the point - validation is
	// on shape, not vocabulary.
	in := []string{"snd_bcm2835.enable_hdmi=1", "nomodeset", "loglevel=8", "cma=64M@256M", "totally.made.up=yes"}

	got, err := kernelparam.Parse(in)
	if err != nil {
		t.Fatalf("Parse(%v) failed: %v", in, err)
	}
	if strings.Join(got, " ") != strings.Join(in, " ") {
		t.Errorf("Parse(%v) = %v, want the same parameters in the same order", in, got)
	}
}

func TestParseWithNoParametersReturnsNothing(t *testing.T) {
	got, err := kernelparam.Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil) failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Parse(nil) = %v, want nothing", got)
	}
}

func TestParseRejectsValuesThatWouldBreakTheBootConfig(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{"two parameters in one flag", "quiet loglevel=8"},
		{"tab", "loglevel=8\tquiet"},
		{"newline", "loglevel=8\nquiet"},
		{"carriage return", "loglevel=8\r"},
		{"NUL", "loglevel=8\x00"},
		{"control character", "loglevel=\x07"},
		{"no name before the =", "=8"},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := kernelparam.Parse([]string{tc.value})
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want an error", tc.value)
			}
			if !strings.Contains(err.Error(), "--kernel-param") {
				t.Errorf("error = %q, want it to name the flag it is about", err)
			}
			// Quoted, so a value whose problem is an invisible
			// character (a tab, a NUL) is legible in the message
			// rather than corrupting the terminal that prints it.
			if tc.value != "" && !strings.Contains(err.Error(), strconv.Quote(tc.value)) {
				t.Errorf("error = %q, want it to quote the offending value %q", err, tc.value)
			}
		})
	}
}

func TestParseRejectsTheWholeSetWhenOneValueIsBad(t *testing.T) {
	if _, err := kernelparam.Parse([]string{"quiet", "bad param", "loglevel=8"}); err == nil {
		t.Fatal("Parse succeeded with one malformed value among good ones, want an error")
	}
}
