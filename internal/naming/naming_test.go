package naming

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"hello":            "hello",
		"Hello_World":      "hello-world",
		"./examples/hello": "examples-hello",
		"My App v2!!":      "my-app-v2",
		"---":              "app",
		"":                 "app",
	}

	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeCapsLength(t *testing.T) {
	long := strings.Repeat("a", 100)
	if got := Sanitize(long); len(got) != MaxLength {
		t.Errorf("Sanitize(100 a's) has length %d, want %d", len(got), MaxLength)
	}

	// A hyphen sitting exactly at the truncation point must not survive as
	// a trailing hyphen once the tail past it is cut off.
	hyphenAtBoundary := strings.Repeat("a", MaxLength) + "-more"
	if got := Sanitize(hyphenAtBoundary); strings.HasSuffix(got, "-") {
		t.Errorf("Sanitize(%q) = %q, want no trailing hyphen after truncation", hyphenAtBoundary, got)
	}
}
