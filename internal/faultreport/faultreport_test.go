package faultreport

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/redact"
)

var update = flag.Bool("update", false, "rewrite the testdata golden files from the current renderer")

// crashedAt is the wall clock every golden that has a trustworthy one uses.
var crashedAt = time.Date(2026, 9, 11, 11, 57, 3, 0, time.UTC)

// pi is the context a healthy, network-connected Pi Zero 2W produces: a real
// device-tree model, a synced clock, and a fully-populated image line.
func pi() Context {
	return Context{
		AppName:             "myapp",
		AppVersion:          "0.1.0",
		ShortIdentity:       "a1b2c3d4",
		SupportURL:          "https://example.com/support",
		DeviceModel:         "Raspberry Pi Zero 2 W Rev 1.0",
		BoardID:             "pi-zero-2w",
		BoardDisplayName:    "Raspberry Pi Zero 2W",
		BoardDisplayNameFor: "pi-zero-2w",
		Timestamp:           crashedAt,
		ClockSynced:         true,
		Uptime:              4*time.Minute + 12*time.Second,
		UptimeKnown:         true,
		BootCount:           37,
	}
}

func TestRender(t *testing.T) {
	corrupt := Report{
		Code:    "GOSD-DATA-CORRUPT",
		Doing:   "starting up",
		Problem: "The storage this device keeps your data on no longer holds a filesystem it recognises.",
		Fix:     "Plug the card into a computer and salvage what you need from partition 2.",
		Detail:  "expanding the data partition: data partition corrupt: /dev/mmcblk0p2 holds nothing (blank space)",
	}

	cases := []struct {
		name   string
		report Report
		ctx    Context
	}{
		{"full", corrupt, pi()},
		{
			// An app that couldn't name a fix: the report has to send its
			// reader somewhere rather than simply stopping.
			name:   "no-fix-with-support-url",
			report: Report{Code: "NO-API-KEY", Doing: "fetching today's forecast", Problem: "The weather service rejected our API key.", Detail: "401 Unauthorized"},
			ctx:    pi(),
		},
		{
			// The same, on an image built without --support-url: the
			// fallback must not trail off pointing at nothing.
			name:   "no-fix-no-support-url",
			report: Report{Code: "NO-API-KEY", Doing: "fetching today's forecast", Problem: "The weather service rejected our API key.", Detail: "401 Unauthorized"},
			ctx: func() Context {
				c := pi()
				c.SupportURL = ""
				return c
			}(),
		},
		{
			// The case a crash report most exists for: a failure before the
			// network — and so before the clock — came up, on an image old
			// enough to have baked none of the report metadata.
			name:   "unsynced-clock-and-nothing-baked",
			report: Report{Code: "GOSD-BOOT-MOUNT", Problem: "The device could not read its own SD card."},
			ctx: Context{
				BoardID:             "pi-zero-w",
				BoardDisplayName:    "Raspberry Pi Zero W",
				BoardDisplayNameFor: "pi-zero-w",
				Timestamp:           time.Unix(0, 0),
			},
		},
		{
			// A declared fault with nothing technical behind it at all.
			name:   "no-technical-detail",
			report: Report{Code: "NO-SENSOR", Doing: "reading the temperature", Problem: "The configured sensor isn't one this build supports.", Fix: "Set sensor = \"bme280\" in gosd.toml on this card."},
			ctx:    pi(),
		},
		{
			// A panic's goroutine dump: multi-KiB, verbatim, and never
			// truncated by the renderer (the tail is bounded before it ever
			// gets here — see cmd/gosd-init/internal/consoletail).
			name:   "multi-kib-technical-detail",
			report: Report{Code: "GOSD-APP-CRASH", Doing: "running", Problem: "The app stopped unexpectedly.", Detail: goroutineDump(40)},
			ctx:    pi(),
		},
		{
			// qemu-virt's device tree describes itself with a compatible
			// string, which tells an owner nothing: the baked display name
			// is used instead.
			name:   "unusable-device-tree-model",
			report: corrupt,
			ctx: func() Context {
				c := pi()
				c.DeviceModel = "linux,dummy-virt"
				c.BoardID = "qemu-virt"
				c.BoardDisplayName = "QEMU virt"
				c.BoardDisplayNameFor = "qemu-virt"
				return c
			}(),
		},
		{
			// gosd.board= on the hand-editable cmdline.txt changed which
			// board this image runs as, so the baked display name may now
			// describe different hardware: only the bare id is trustworthy.
			name:   "board-overridden-on-the-cmdline",
			report: corrupt,
			ctx: func() Context {
				c := pi()
				c.DeviceModel = ""
				c.BoardID = "pi-3b"
				return c
			}(),
		},
		{
			// Secrets are scrubbed wherever they landed — an app is
			// perfectly capable of interpolating one into its own prose.
			name: "secrets-redacted",
			report: Report{
				Code:    "GOSD-APP-CRASH",
				Doing:   "publishing a reading",
				Problem: "The broker rejected the token sk-live-9f3c2b71.",
				Detail:  "connect: auth failed for sk-live-9f3c2b71\nsession=deadbeefcafe",
			},
			ctx: func() Context {
				c := pi()
				c.Secrets = []redact.Rule{
					{Needle: "sk-live-9f3c2b71", Replacement: "{$BROKER_TOKEN}"},
					{Needle: "deadbeefcafe", Replacement: "{secret: session-token}"},
				}
				return c
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkGolden(t, tc.name, Render(tc.report, tc.ctx).Markdown)
		})
	}
}

func TestRenderScrubsSecretsFromEveryPartOfTheBody(t *testing.T) {
	const secret = "sk-live-9f3c2b71"
	ctx := pi()
	ctx.Secrets = []redact.Rule{{Needle: secret, Replacement: "{$BROKER_TOKEN}"}}

	got := Render(Report{
		Code:    "GOSD-APP-CRASH",
		Doing:   "publishing with " + secret,
		Problem: "the broker rejected " + secret,
		Fix:     "replace " + secret + " in gosd.toml",
		Detail:  "auth failed for " + secret,
	}, ctx)

	if strings.Contains(got.Markdown, secret) {
		t.Errorf("the rendered report still contains the secret:\n%s", got.Markdown)
	}
	if n := strings.Count(got.Markdown, "{$BROKER_TOKEN}"); n != 4 {
		t.Errorf("replaced the secret %d times, want 4 (Doing, Problem, Fix and Detail)", n)
	}
}

func TestRenderReportsASecretTooShortToRedact(t *testing.T) {
	ctx := pi()
	ctx.Secrets = []redact.Rule{{Needle: "hunter2", Replacement: "{secret: pin}"}}

	got := Render(Report{Code: "X", Detail: "tried hunter2"}, ctx)

	if want := []string{"{secret: pin}"}; len(got.SkippedSecrets) != 1 || got.SkippedSecrets[0] != want[0] {
		t.Errorf("SkippedSecrets = %v, want %v", got.SkippedSecrets, want)
	}
}

// TestRenderKeepsTheHeaderParseable pins the reason yamlScalar exists: the
// build identity sits after a "#", which unquoted is a YAML comment.
func TestRenderKeepsTheHeaderParseable(t *testing.T) {
	got := Render(Report{Code: "X"}, pi()).Markdown

	if !strings.Contains(got, `image: "myapp 0.1.0 #a1b2c3d4"`) {
		t.Errorf("the image header line isn't quoted, so a parser drops the build identity:\n%s", got)
	}
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name+".md")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s (re-run with -update to create it): %v", path, err)
	}
	if got != string(want) {
		t.Errorf("rendered report doesn't match %s:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

// goroutineDump fabricates a panic dump of a realistic shape and size, so
// the multi-KiB golden exercises a real tail rather than one long line.
func goroutineDump(goroutines int) string {
	var b strings.Builder
	b.WriteString("panic: runtime error: invalid memory address or nil pointer dereference\n")
	b.WriteString("[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x5f2a4]\n\n")
	for i := range goroutines {
		fmt.Fprintf(&b, "goroutine %d [running]:\n", i+1)
		fmt.Fprintf(&b, "main.(*sampler).read(0x400012c%03x, {0x4000180000, 0x40, 0x40})\n", i)
		fmt.Fprintf(&b, "\t/home/dev/myapp/sampler.go:%d +0x%x\n", 100+i, 0x1c+i)
	}
	return b.String()
}
