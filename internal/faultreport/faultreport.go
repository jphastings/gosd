// Package faultreport renders LAST_FATAL_ERROR.md: the human-readable crash
// report a GoSD device writes onto the root of its own boot partition so
// that whoever collects an unattended device can read the latest fatal issue
// by plugging the card into any computer (epic gosd-47z3).
//
// Every producer of a report formats through Render — gosd-init's own fatal
// paths, the app-crash console tail, and the public fault package an app
// calls itself — so the file reads identically whatever raised it. That is
// the whole reason this package exists rather than each caller building its
// own string, and it is why the redaction pass lives here too (see
// Context.Secrets): a producer cannot forget to scrub what it never
// assembles.
//
// The output is Markdown with YAML frontmatter, because it has to render as
// prose in a text editor, a Finder/Explorer preview and a GitHub issue, all
// of which a non-technical device owner might reach for, while still being
// greppable by whoever supports them.
package faultreport

import (
	"strconv"
	"strings"
	"time"

	"github.com/jphastings/gosd/internal/redact"
)

// FileName is the report's name at the root of the boot partition. It is
// deliberately loud and sorts near the top of a FAT root otherwise full of
// kernel8.img / config.txt / gosd.toml.
const FileName = "LAST_FATAL_ERROR.md"

// LegacyFileName is what this file was called before gosd-pun9, when it was
// a plain log written only by the data-corruption halt. A card flashed by an
// older release can still carry one, so gosd-init deletes it rather than
// leaving two contradictory files on the same card.
const LegacyFileName = "boot-failure.log"

// unspecifiedCode is the error_code emitted for a report whose raiser didn't
// set one. Deliberately not in the GOSD-* namespace: that namespace is
// gosd-init's own, and a missing code is far more likely to be an app's
// omission than one of ours.
const unspecifiedCode = "UNSPECIFIED"

// unknown is what every header field renders as when its value genuinely
// isn't knowable, rather than guessing. A wrong timestamp in a crash report
// is worse than an absent one.
const unknown = "unknown"

// Report is the content of one crash report: what the device was doing, what
// went wrong, and what its owner can do about it. Doing, Problem and Fix are
// written for the device's owner — who is usually not a developer — and
// Detail is the only field aimed at whoever they forward the file to.
type Report struct {
	// Code is a stable, greppable identifier for this class of failure:
	// gosd-init's own are namespaced GOSD-*, an app's are whatever it
	// passes. Empty renders as UNSPECIFIED.
	Code string
	// Doing is what the device was doing for its user, in human terms
	// ("fetching today's forecast"). Empty drops the clause entirely
	// rather than rendering an awkward half-sentence.
	Doing string
	// Problem is a short human explanation of what went wrong.
	Problem string
	// Fix is a concrete instruction the owner can act on ("add
	// WEATHER_API_KEY to gosd.toml on this card"). Empty is a legitimate
	// answer — the report then points at Context.SupportURL, or says
	// plainly that there is nowhere to point.
	Fix string
	// Detail is the panic dump, error chain or console tail, reproduced
	// verbatim in an indented code block. Empty renders as a sentence
	// saying nothing was captured, never an empty section.
	Detail string
}

// Context is everything about the device and the image that a report's
// header needs and that its raiser has no way to know. gosd-init assembles
// it once per boot; the public fault package's off-device rendering fills in
// only what a developer's own machine can honestly claim.
type Context struct {
	// AppName is config.json's baked appName. Empty (an image built before
	// that field existed) is rendered as "unknown" and never substituted
	// with the device's hostname, which a user may have renamed.
	AppName string
	// AppVersion is config.json's baked appVersion (gosd build
	// --app-version). Empty omits it from the image line.
	AppVersion string
	// ShortIdentity is initcfg.Config.ShortIdentity: the truncated,
	// content-derived digest of this build's boot payload. Empty omits it.
	ShortIdentity string
	// SupportURL is config.json's baked supportURL, used only when a
	// report declares no Fix.
	SupportURL string

	// DeviceModel is the hardware's own self-description, read verbatim
	// from the device tree (/sys/firmware/devicetree/base/model). It is
	// preferred over BoardDisplayName because it names the hardware that
	// actually booted, distinguishing boards a single GoSD image
	// deliberately conflates — pi-3b covers both the 3B and the 3B+, and
	// only the device tree says which one this is. Pass it raw: Render
	// discards a value that is a device-tree compatible string rather than
	// a human-readable name (qemu-virt's "linux,dummy-virt"), and falls
	// back to BoardDisplayName.
	DeviceModel string
	// BoardID is the board this image is running as, after any
	// gosd.board= kernel cmdline override.
	BoardID string
	// BoardDisplayName is config.json's baked boardDisplayName.
	BoardDisplayName string
	// BoardDisplayNameFor is the board id BoardDisplayName was baked for —
	// the value of config.json's board field as parsed, before any
	// gosd.board= override. Render only pairs BoardDisplayName with
	// BoardID when the two agree: cmdline.txt is a hand-editable file on
	// the FAT partition, and a gosd.board= override changes the effective
	// board without touching the baked display name, so trusting the
	// display name unconditionally would name the wrong hardware (see
	// initcfg.Config.BoardDisplayName's own caution).
	BoardDisplayNameFor string

	// Timestamp is the wall clock at the moment of the failure, and is
	// only rendered when ClockSynced reports it can be trusted.
	Timestamp time.Time
	// ClockSynced reports whether the clock has been set from a time
	// server this boot. No board in the fleet has a working RTC, so before
	// the first successful sync the clock reads ~1970 — and a crash before
	// networking comes up is exactly the case a report exists for. False
	// renders "timestamp: unknown" rather than a confidently wrong date.
	ClockSynced bool

	// Uptime is how long the device had been up, and UptimeKnown whether
	// it could be measured at all. Unlike the wall clock this is always
	// true when present, and it answers the question that actually
	// matters: did it die instantly or after four days?
	Uptime      time.Duration
	UptimeKnown bool

	// BootCount is how many times this device has booted, from the durable
	// counter on the data partition. Zero — no counter, or a read-only or
	// absent /data — renders as "unknown".
	BootCount int

	// Secrets are the (needle, replacement) pairs scrubbed from the
	// rendered body before it is returned. The report tells its reader to
	// forward the whole file to a support site, so this is applied to
	// every producer's output by construction rather than at each call
	// site. The frontmatter is generated here from known-safe fields and
	// is deliberately left alone. See internal/redact, whose
	// MinNeedleLength floor means a short value is skipped rather than
	// applied — Result.SkippedSecrets reports which, without exposing any
	// of them.
	Secrets []redact.Rule
}

// Result is a rendered report.
type Result struct {
	// Markdown is the complete file contents, frontmatter included.
	Markdown string
	// SkippedSecrets carries the Replacement — never the needle — of every
	// Context.Secrets rule that was too short for redact to act on safely,
	// so a caller can log the omission without leaking what it protected.
	SkippedSecrets []string
}

// Render turns a report and its device context into the complete
// LAST_FATAL_ERROR.md file contents, with every Context.Secrets rule applied
// to the body.
func Render(r Report, c Context) Result {
	redacted := redact.Redact(body(r, c), c.Secrets)
	return Result{
		Markdown:       frontmatter(r, c) + redacted.Body,
		SkippedSecrets: redacted.Skipped,
	}
}

// frontmatter renders the machine-readable header, in the locked field
// order. Every value is emitted through yamlScalar so that a device model or
// app version containing YAML's own punctuation can't silently truncate the
// field it's in.
func frontmatter(r Report, c Context) string {
	var b strings.Builder
	b.WriteString("---\n")
	writeField(&b, "error_code", or(r.Code, unspecifiedCode))
	writeField(&b, "timestamp", timestampField(c))
	writeField(&b, "clock", clockField(c))
	writeField(&b, "uptime", uptimeField(c))
	writeField(&b, "boot", bootField(c))
	writeField(&b, "device", deviceField(c))
	writeField(&b, "image", imageField(c))
	b.WriteString("---\n\n")
	return b.String()
}

func writeField(b *strings.Builder, name, value string) {
	b.WriteString(name)
	b.WriteString(": ")
	b.WriteString(yamlScalar(value))
	b.WriteString("\n")
}

func timestampField(c Context) string {
	if !c.ClockSynced || c.Timestamp.IsZero() {
		return unknown
	}
	return c.Timestamp.UTC().Format(time.RFC3339)
}

func clockField(c Context) string {
	if c.ClockSynced {
		return "ntp-synced"
	}
	return "unsynced — timestamp is not trustworthy"
}

func uptimeField(c Context) string {
	if !c.UptimeKnown {
		return unknown
	}
	return c.Uptime.Round(time.Second).String()
}

func bootField(c Context) string {
	if c.BootCount <= 0 {
		return unknown
	}
	return strconv.Itoa(c.BootCount)
}

// deviceField names the hardware for the owner and the board id for whoever
// debugs it: the two answer different questions ("what is this?" versus
// "which kernel and artifacts shipped?"), so both are emitted.
func deviceField(c Context) string {
	name := hardwareName(c.DeviceModel)
	if name == "" && c.BoardDisplayNameFor == c.BoardID {
		name = strings.TrimSpace(c.BoardDisplayName)
	}
	switch {
	case name != "" && c.BoardID != "":
		return name + " (" + c.BoardID + ")"
	case name != "":
		return name
	case c.BoardID != "":
		return c.BoardID
	default:
		return unknown
	}
}

// hardwareName returns model if it is a human-readable hardware name, and ""
// if it is anything else — an empty file, or a device-tree compatible string
// like qemu-virt's "linux,dummy-virt", which would tell an owner nothing and
// isn't worth printing when the baked board display name is right there. The
// comma-without-space test is what separates the two: a compatible string is
// "vendor,board" by construction, while a real model string ("Raspberry Pi
// Zero 2 W Rev 1.0", "Radxa ROCK 4SE") is words with spaces.
func hardwareName(model string) string {
	name := strings.TrimSpace(strings.ReplaceAll(model, "\x00", ""))
	if name == "" {
		return ""
	}
	if strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return ""
	}
	if strings.Contains(name, ",") && !strings.Contains(name, " ") {
		return ""
	}
	return name
}

// imageField identifies the build: the app's name and version as the
// developer set them, plus the content-derived identity that tells two
// builds of the same version apart.
func imageField(c Context) string {
	parts := []string{or(c.AppName, unknown)}
	if c.AppVersion != "" {
		parts = append(parts, c.AppVersion)
	}
	if c.ShortIdentity != "" {
		parts = append(parts, "#"+c.ShortIdentity)
	}
	return strings.Join(parts, " ")
}

func body(r Report, c Context) string {
	var b strings.Builder

	if c.AppName != "" {
		b.WriteString("# " + c.AppName + " crash report\n\n")
		b.WriteString("Your " + c.AppName + " device stopped")
	} else {
		b.WriteString("# Crash report\n\n")
		b.WriteString("Your device stopped")
	}
	if r.Doing != "" {
		b.WriteString(" while " + r.Doing)
	}
	b.WriteString(".\n\n")

	b.WriteString("This file was written by the device itself, onto its own SD card, so you can\n")
	b.WriteString("read it on any computer. Nothing was sent anywhere.\n\n")

	b.WriteString("## The problem\n\n")
	b.WriteString(paragraph(or(r.Problem, "The device didn't record anything beyond the error code above.")))

	b.WriteString("## The fix\n\n")
	b.WriteString(paragraph(fixText(r, c)))

	b.WriteString("## What to send\n\n")
	b.WriteString("If you ask anyone for help, send them **this whole file** rather than a\n")
	b.WriteString("summary — the section below is the part they need.\n\n")

	b.WriteString("## Technical detail\n\n")
	b.WriteString(detailText(r.Detail))

	return b.String()
}

// fixText is the report's advice, and the reason SupportURL is baked into
// every image: a report with no fix and nowhere to send its reader is a dead
// end, so when there's no URL either the file says so plainly rather than
// trailing off into a sentence with a hole in it.
func fixText(r Report, c Context) string {
	switch {
	case r.Fix != "":
		return r.Fix
	case c.SupportURL != "":
		return "We don't have a specific fix for this one. Visit " + c.SupportURL + " and\nquote the error code above."
	default:
		return "We don't have a specific fix for this one, and this image doesn't name a\nsupport page. Quote the error code above to whoever supplied this device."
	}
}

// detailText renders Detail as an indented code block, which — unlike a
// fenced one — no content can break out of, however many backticks a panic
// dump happens to contain.
func detailText(detail string) string {
	detail = strings.TrimRight(detail, "\n")
	if strings.TrimSpace(detail) == "" {
		return "Nothing was captured for this failure.\n"
	}

	var b strings.Builder
	for _, line := range strings.Split(detail, "\n") {
		if strings.TrimSpace(line) == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("    " + line + "\n")
	}
	return b.String()
}

// paragraph terminates a variable-length section with exactly one blank line
// after it, however the value it was handed was punctuated.
func paragraph(text string) string {
	return strings.TrimRight(text, "\n") + "\n\n"
}

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// yamlScalar quotes a header value when emitting it bare would change what a
// YAML parser reads back. The image line is the case that forces this to
// exist: "myapp 0.1.0 #a1b2c3d4" has a comment marker in the middle of it,
// so unquoted it parses as "myapp 0.1.0" and silently loses the build
// identity — the one field whoever debugs the report most needs.
func yamlScalar(value string) string {
	if value == "" {
		return `""`
	}
	if needsQuoting(value) {
		return strconv.Quote(value)
	}
	return value
}

func needsQuoting(value string) bool {
	switch {
	case strings.Contains(value, ": "),
		strings.HasSuffix(value, ":"),
		strings.Contains(value, " #"),
		strings.HasPrefix(value, " "),
		strings.HasSuffix(value, " "),
		strings.ContainsAny(value, "\n\r\t"):
		return true
	}
	// A leading YAML indicator makes the whole scalar something else — a
	// sequence entry, a flow mapping, an anchor, a block scalar.
	return strings.ContainsAny(value[:1], `-?:,[]{}#&*!|>'"%@`+"`")
}
