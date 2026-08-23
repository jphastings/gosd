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
			// The same, on an image built without --app-support-url: the
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
			report: Report{Code: "NO-SENSOR", Doing: "reading the temperature", Problem: "The configured sensor isn't one this build supports.", Fix: "Write bme280 into config/env/SENSOR on this card."},
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
			// Secrets are scrubbed wherever the report carried them — an
			// app is perfectly capable of interpolating one into its own
			// prose, or into the error code the header renders. The third
			// rule is the other half of the same property: "computer" is
			// a word in gosd's own explanation of what this file is, and
			// that sentence must come through untouched however ordinary
			// an app's env value turns out to be.
			name: "secrets-redacted",
			report: Report{
				Code:    "AUTH-FAIL-sk-live-9f3c2b71",
				Doing:   "publishing a reading",
				Problem: "The broker rejected the token sk-live-9f3c2b71.",
				Detail:  "connect: auth failed for sk-live-9f3c2b71\nsession=deadbeefcafe",
			},
			ctx: func() Context {
				c := pi()
				c.Secrets = []redact.Rule{
					{Needle: "sk-live-9f3c2b71", Replacement: "{$BROKER_TOKEN}"},
					{Needle: "deadbeefcafe", Replacement: "{secret: session-token}"},
					{Needle: "computer", Replacement: "{$WORKSTATION}"},
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

func TestRenderScrubsSecretsFromEveryFieldTheReportCarries(t *testing.T) {
	const secret = "sk-live-9f3c2b71"
	ctx := pi()
	ctx.Secrets = []redact.Rule{{Needle: secret, Replacement: "{$BROKER_TOKEN}"}}

	got := Render(Report{
		Code:    "AUTH-FAIL-" + secret,
		Doing:   "publishing with " + secret,
		Problem: "the broker rejected " + secret,
		Fix:     "replace " + secret + " in config/env/API_TOKEN",
		Detail:  "auth failed for " + secret,
	}, ctx)

	if strings.Contains(got.Markdown, secret) {
		t.Errorf("the rendered report still contains the secret:\n%s", got.Markdown)
	}
	if n := strings.Count(got.Markdown, "{$BROKER_TOKEN}"); n != 5 {
		t.Errorf("replaced the secret %d times, want 5 (Code, Doing, Problem, Fix and Detail)", n)
	}
}

// The error code is app-supplied text, not one of the header fields gosd
// generates for itself, and it skipped redaction entirely for as long as a
// comment claiming otherwise stood (bean gosd-ywsv).
func TestRenderScrubsSecretsFromTheErrorCodeInTheHeader(t *testing.T) {
	const secret = "sk_live_ABCDEFGHIJKLMNOP"
	ctx := pi()
	ctx.Secrets = []redact.Rule{{Needle: secret, Replacement: "{secret: stripe}"}}

	header, _, _ := strings.Cut(Render(Report{Code: "AUTH-FAIL-" + secret}, ctx).Markdown, "\n---\n")

	if strings.Contains(header, secret) {
		t.Errorf("the frontmatter still contains the secret:\n%s", header)
	}
	if !strings.Contains(header, "{secret: stripe}") {
		t.Errorf("the frontmatter is missing the redaction placeholder:\n%s", header)
	}
}

// gosd's own prose can't hold a secret, so redacting it protects nothing
// and damages the one part of the file a non-technical device owner reads
// (bean gosd-fu1z). "computer" is exactly redact.MinNeedleLength bytes, so
// an env value of it clears the floor and used to rewrite the sentence
// explaining what this file is.
func TestRenderLeavesGosdsOwnWordsAlone(t *testing.T) {
	ctx := pi()
	ctx.Secrets = []redact.Rule{{Needle: "computer", Replacement: "{$WORKSTATION}"}}

	got := Render(Report{Code: "X", Problem: "The device could not reach the computer."}, ctx).Markdown

	if !strings.Contains(got, "read it on any computer.") {
		t.Errorf("redaction rewrote gosd's own explanation of the file:\n%s", got)
	}
	if !strings.Contains(got, "reach the {$WORKSTATION}.") {
		t.Errorf("the same value in the app's own words wasn't redacted:\n%s", got)
	}
}

// The image line's fields are the developer's strings rather than gosd's,
// so they stay in the redacted set even though everything around them left
// it: an app can bake a value it also holds as a secret.
func TestRenderScrubsSecretsFromTheBakedImageFields(t *testing.T) {
	const secret = "sk-live-9f3c2b71"
	ctx := pi()
	ctx.AppName = secret
	ctx.AppVersion = secret
	ctx.SupportURL = "https://example.com/" + secret
	ctx.Secrets = []redact.Rule{{Needle: secret, Replacement: "{$BROKER_TOKEN}"}}

	got := Render(Report{Code: "X", Problem: "the key was rejected"}, ctx).Markdown

	if strings.Contains(got, secret) {
		t.Errorf("the rendered report still contains the secret:\n%s", got)
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

func TestFoldConsoleTailKeepsTheAppsOwnWordsAndTheTailBoth(t *testing.T) {
	// A fault call on one goroutine and a panic on another can genuinely
	// coincide. The app knows what its user was promised; the tail knows
	// what actually blew up.
	declared := Report{
		Code:    "NO-API-KEY",
		Doing:   "fetching the forecast",
		Problem: "the weather service rejected our API key",
		Fix:     "add WEATHER_API_KEY to config/env/ on this card",
		Detail:  "401 unauthorized",
	}

	got := FoldConsoleTail(declared, "panic: nil map write\ngoroutine 7 [running]:")

	if got.Code != declared.Code || got.Doing != declared.Doing || got.Problem != declared.Problem || got.Fix != declared.Fix {
		t.Errorf("the human sections became %+v, want the app's own", got)
	}
	if !strings.Contains(got.Detail, "401 unauthorized") || !strings.Contains(got.Detail, "panic: nil map write") {
		t.Errorf("technical detail = %q, want both the app's error and the console tail", got.Detail)
	}
}

func TestFoldConsoleTailBecomesTheOnlyDetailWhenThereIsNone(t *testing.T) {
	got := FoldConsoleTail(Report{Code: "NO-API-KEY"}, "listening on :80\npanic: nil map write")

	if got.Detail != "listening on :80\npanic: nil map write" {
		t.Errorf("technical detail = %q, want the console tail alone", got.Detail)
	}
}

func TestFoldConsoleTailLeavesAnEmptyTailAlone(t *testing.T) {
	declared := Report{Code: "NO-API-KEY", Detail: "401 unauthorized"}

	got := FoldConsoleTail(declared, "")

	if got.Detail != "401 unauthorized" {
		t.Errorf("technical detail = %q, want it unchanged when there is no tail to fold in", got.Detail)
	}
}

// TestRenderOmitsUnhonestUnknownsInAPreview pins the "worth fixing while
// here" half of gosd-72ga: the off-device developer preview never had a
// device tree, a boot counter or an uptime clock to read in the first
// place, so printing "unknown" for each reads as a broken report rather
// than a genuine preview of a complete one. The same fields on a real
// device (Preview unset) still print "unknown" honestly — see the
// "full" golden case, which keeps them.
func TestRenderOmitsUnhonestUnknownsInAPreview(t *testing.T) {
	got := Render(Report{Code: "NO-API-KEY", Problem: "the key was rejected"}, Context{
		AppName:     "myapp",
		Timestamp:   time.Now(),
		ClockSynced: true,
		Preview:     true,
	}).Markdown

	for _, absent := range []string{"uptime:", "boot:", "device:"} {
		if strings.Contains(got, absent) {
			t.Errorf("preview report contains %q, want it omitted entirely rather than printed as unknown:\n%s", absent, got)
		}
	}
	if !strings.Contains(got, "error_code: NO-API-KEY") || !strings.Contains(got, "image: myapp") {
		t.Errorf("preview report is missing fields it CAN honestly claim:\n%s", got)
	}
}

// TestRenderStillSaysUnknownOnADeviceWhenAFieldGenuinelyFailsToRead proves
// Preview is the only thing that changes: an on-device report — this image
// simply has no writable /data, no readable device tree, and hasn't
// measured uptime — still says so plainly, because there "unknown" is
// diagnostic information, not a placeholder for something a preview could
// never have known anyway.
func TestRenderStillSaysUnknownOnADeviceWhenAFieldGenuinelyFailsToRead(t *testing.T) {
	got := Render(Report{Code: "GOSD-APP-CRASH"}, Context{BoardID: "pi-zero-2w"}).Markdown

	for _, want := range []string{"uptime: unknown", "boot: unknown"} {
		if !strings.Contains(got, want) {
			t.Errorf("on-device report is missing %q, want it printed honestly:\n%s", want, got)
		}
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
