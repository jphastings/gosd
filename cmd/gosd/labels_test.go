package main

import (
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/naming"
)

func TestResolveLabelsDefaultsToTheAppName(t *testing.T) {
	cases := map[string]struct{ boot, data string }{
		// Short enough to use whole.
		"hello": {"hello-boot", "hello-data"},
		// Exactly at the cap once truncated.
		"usbwebsite": {"usbweb-boot", "usbweb-data"},
		// The truncation lands on a hyphen, which must not survive.
		"abcde-fgh": {"abcde-boot", "abcde-data"},
		"a":         {"a-boot", "a-data"},
		// deriveAppName sanitizes before this ever sees it, but a name that
		// sanitizes to nothing still has to produce usable labels.
		"":       {"app-boot", "app-data"},
		"My App": {"my-app-boot", "my-app-data"},
	}
	for appName, want := range cases {
		labels, err := resolveLabels("", false, appName)
		if err != nil {
			t.Errorf("resolveLabels(app %q) = %v, want nil", appName, err)
			continue
		}
		if labels.Boot != want.boot || labels.Data != want.data {
			t.Errorf("resolveLabels(app %q) = %+v, want {%s %s}", appName, labels, want.boot, want.data)
		}
	}
}

func TestResolveLabelsUsesAnExplicitPrefixVerbatim(t *testing.T) {
	// Case included: an explicit prefix is never lowercased or otherwise
	// rewritten, so what a person typed is what ends up on the card.
	for _, prefix := range []string{"web", "Web", "abcdef", "a1", "my_app", "a-b"} {
		labels, err := resolveLabels(prefix, true, "ignored")
		if err != nil {
			t.Errorf("resolveLabels(--label-prefix=%q) = %v, want nil", prefix, err)
			continue
		}
		if labels.Boot != prefix+"-boot" || labels.Data != prefix+"-data" {
			t.Errorf("resolveLabels(--label-prefix=%q) = %+v, want the prefix used verbatim", prefix, labels)
		}
	}
}

// charsetHint is the part of the refusal every disallowed character shares.
const charsetHint = "letters, digits, hyphens and underscores"

func TestResolveLabelsRefusesAnUnusablePrefix(t *testing.T) {
	cases := map[string]struct {
		prefix   string
		wantHint string
	}{
		// One character too long for FAT's 11-byte label; the error offers
		// a prefix that does fit.
		"seven characters": {"appname", "--label-prefix=appnam"},
		"empty":            {"", "omit the flag"},
		// Everything below is the same charset rule: a volume label is
		// stored as a FAT short-name entry, so only [A-Za-z0-9_-] is
		// allowed through.
		"an inner space":     {"my app", charsetHint},
		"a trailing space":   {"web ", charsetHint},
		"non-ASCII":          {"café", charsetHint},
		"a non-printable":    {"we\x00b", charsetHint},
		"a path separator":   {"a/b", charsetHint},
		"a backslash":        {`a\b`, charsetHint},
		"a dot":              {"a.b", charsetHint},
		"a colon":            {"a:b", charsetHint},
		"a wildcard":         {"a*b", charsetHint},
		"a redirect bracket": {"a<b", charsetHint},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := resolveLabels(c.prefix, true, "hello")
			if err == nil {
				t.Fatalf("resolveLabels(--label-prefix=%q) = nil, want a refusal", c.prefix)
			}
			if !strings.Contains(err.Error(), c.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err, c.wantHint)
			}
		})
	}
}

// Whatever resolveLabels accepts, both labels must be ones every formatter
// and every mount decision in the stack will handle — blockmount.ValidateLabel
// is the authority on that, and this pins that resolveLabels never lets a
// pair past it.
func TestResolveLabelsOnlyEverProducesValidLabels(t *testing.T) {
	accepted := []struct {
		prefix   string
		explicit bool
		appName  string
	}{
		{appName: "hello"},
		{appName: "usbwebsite"},
		{appName: ""},
		{appName: "My App v2!!"},
		{prefix: "web", explicit: true},
		{prefix: "Web", explicit: true},
		{prefix: "abcdef", explicit: true},
		{prefix: "my_app", explicit: true},
	}
	for _, c := range accepted {
		labels, err := resolveLabels(c.prefix, c.explicit, c.appName)
		if err != nil {
			t.Errorf("resolveLabels(%q, %v, %q) = %v, want nil", c.prefix, c.explicit, c.appName, err)
			continue
		}
		for _, label := range []string{labels.Boot, labels.Data} {
			if err := blockmount.ValidateLabel("test", diskfmt.FAT32, label); err != nil {
				t.Errorf("resolveLabels(%q, %v, %q) produced %q, which blockmount rejects: %v", c.prefix, c.explicit, c.appName, label, err)
			}
			if len(label) > naming.LabelPrefixMaxLength+len(naming.BootLabelSuffix) {
				t.Errorf("label %q is %d bytes, longer than a FAT volume label", label, len(label))
			}
		}
	}
}
