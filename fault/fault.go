// Package fault lets a GoSD app declare a fatal error in terms its user can
// act on, and have the device write that explanation onto its own SD card.
//
// A GoSD device is unattended and its owner is usually not a developer:
// there is no screen, no shell, no SSH, and — without a soldering iron — no
// console. When something goes permanently wrong, the card is the only
// channel back to a human. [Fatal] uses it: the device writes
// LAST_FATAL_ERROR.md to the root of its boot partition, which the owner can
// read on any computer, fix themselves, or forward to you whole. Nothing is
// sent anywhere; there is no telemetry in GoSD and this is not it.
//
// The report gosd-init writes for a crash your app never got to describe
// carries a stack trace and little else. This package is for the failures
// your app understands and a stack trace never could — a rejected API key,
// a config naming a sensor this build doesn't support, a required variable
// nobody set. Those have a real fix, and a report that states it is the
// difference between a returned device and a two-minute edit on the card.
//
// # Fatal means fatal
//
// [Fatal] halts the board. It does not return, and the device stays down
// until someone power-cycles it — so use it only for a condition no restart
// can improve. Anything that might succeed on a second attempt should be an
// ordinary returned error, left to gosd-init's supervisor and its backoff.
//
// # Off a device, it shows you the report instead
//
// On your Mac, under go test, or in anything else not built by gosd build,
// [Fatal] prints the very same Markdown document to stderr and exits,
// rather than looking for a boot partition that isn't there. That is the
// point of it: you can read exactly what your user would read, and check
// your own wording, without flashing a card.
//
// The printed copy carries only what your app's own process can know. On a
// device, the copy on the card additionally names the hardware from its
// device tree, the image's identity, how long the board had been up, how
// many times it has booted, and every value in your app's environment is
// scrubbed from it — see [RegisterSecretString] for the secrets your app
// holds that no environment variable names.
//
// On a device, [Fatal] prints only a short line naming the error code and
// pointing at LAST_FATAL_ERROR.md — never the full report. gosd-init keeps a
// tail of your app's own console output for the crash report it writes when
// your app dies unexpectedly, so printing the whole report here would hand
// gosd-init a copy of the report as your app's own "technical detail",
// nested inside the very report gosd-init is about to write. gosd-init logs
// the complete report to the serial console itself once it commits one — a
// strictly better copy, since it knows the device model, uptime and boot
// count your app's own process never can.
//
// The full guide, including what gosd-init reports for itself, is
// docs/crash-reports.md.
package fault

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jphastings/gosd/internal/faultdrop"
	"github.com/jphastings/gosd/internal/faultreport"
	"github.com/jphastings/gosd/internal/redact"
	"github.com/jphastings/gosd/internal/secretreg"
)

// Report is a fatal condition described in terms its raiser understands and
// its reader doesn't.
//
// Write Doing, Problem and Fix for the person holding the device, who is
// usually not a developer and did not write your app. Detail is the only
// field aimed at whoever they forward the file to.
type Report struct {
	// Code is a stable, greppable identifier for this class of failure,
	// yours to choose: "NO-API-KEY", "SENSOR-UNSUPPORTED". It heads the
	// report's machine-readable frontmatter, so a support page can list
	// your codes and a user can quote one. Keep it stable across
	// releases. Empty renders as UNSPECIFIED.
	Code string
	// Doing is what the device was doing for its user, in their terms:
	// "fetching today's forecast". It completes the sentence "Your device
	// stopped while …", so phrase it to fit. Empty drops the clause.
	Doing string
	// Problem is a short human explanation of what went wrong: "the
	// weather service rejected our API key".
	Problem string
	// Fix is a concrete instruction its reader can act on: "add
	// WEATHER_API_KEY to config/env/ on this card". Leave it empty when
	// there genuinely isn't one — the report then points at the
	// --app-support-url baked into the image instead of inventing advice.
	Fix string
	// Detail is the technical half: the error that led here, reproduced
	// verbatim for whoever gets the file forwarded to them. Nil is fine
	// and renders as a sentence saying nothing was captured.
	//
	// Its text is scrubbed of registered secrets and, on a device, of
	// every value in your app's environment — but it is not scrubbed of
	// anything else, so an error chain that quotes a credential your app
	// never registered will appear in a file the report asks its reader
	// to forward.
	Detail error
}

// unnamedSecret labels a registration whose replacement was unusable — see
// [RegisterSecretString]. Protecting the secret matters more than naming
// it, so a bad label is replaced rather than the registration refused.
const unnamedSecret = "unnamed"

// Fatal records r for gosd-init to write to LAST_FATAL_ERROR.md at the root
// of the boot partition, prints a short pointer to that hand-off on stderr
// (the full report, off a device — see the package doc), and stops the
// device. It does not return.
//
// The device stays down until someone power-cycles it. Nothing restarts,
// nothing retries: an app calling Fatal is asserting that a restart cannot
// help, and restarting anyway would only grind the card and bury the report
// under a crash loop. Anything that might succeed on a retry should be an
// ordinary returned error instead, left to the supervisor's backoff — which
// already restarts your app with a widening delay and writes a report of
// its own if it keeps failing.
//
// Because Fatal ends the process where it stands, deferred functions do not
// run and neither does anything queued behind them. Flush whatever must be
// flushed before calling it.
//
// Off a GoSD device — your Mac, go test, any binary gosd build didn't
// produce — there is no card to write to and no board to halt, so Fatal
// prints the report it would have written and exits non-zero.
func Fatal(r Report) {
	std.deliver(r)
	os.Exit(faultdrop.ExitCode)
}

// RegisterSecretString ensures secret never appears in a crash report,
// replaced wherever it occurs by "{secret: replacement}".
//
// replacement is a LABEL, not a second secret: it is printed in a file the
// report asks its reader to forward to a stranger, so name what was removed
// ("stripe-api-key", "session-token") rather than describing its value.
//
// Call it as soon as your app holds the secret, not when something goes
// wrong. The registration is written through to /run/gosd/secrets.json
// immediately, on this call — /run is a RAM filesystem, so the plaintext
// secret is in memory and never touches the card — because the crash that
// most needs redacting is the one your app never sees coming. On a panic
// your code gets no chance to hand anything over, and the secret still
// sitting in the console output gosd-init is about to record is exactly the
// one nobody registered in time.
//
// Registering is additive and idempotent: registering the same secret twice
// is not an error, and the first label given for a value is the one that
// sticks. An empty secret registers nothing.
//
// Two limits are worth knowing:
//
// A secret shorter than eight bytes is deliberately NOT redacted, and is
// left in the report as it stands. Short values collide constantly with
// innocent text — a two-character secret would blank pieces of every stack
// trace — so gosd applies a length floor to every source of redaction
// equally, this one included. The omission is logged, by name, wherever the
// report is produced.
//
// gosd redacts at most 64 registered secrets, and reads back at most 64KiB
// of registrations. A registration past either bound is refused with a
// warning on stderr and does not disturb the ones already made — the whole
// set has to be readable back for any of it to be applied, so dropping the
// new one is what keeps the rest working.
func RegisterSecretString(secret, replacement string) {
	std.register(secret, replacement)
}

// std is the reporter behind the exported functions. Tests build their own
// against a temp directory and a buffer, which is what lets both the
// on-device handoff and the off-device preview be tested as behaviour on
// any machine.
var std = &reporter{dir: runDir, out: os.Stderr}

// reporter is this package's state: where reports and registrations are
// handed over (empty off a device), where their console copy goes, and
// which secrets have been registered so far.
type reporter struct {
	mu  sync.Mutex
	dir string
	out io.Writer

	// entries is the whole registration set, in registration order, and
	// is what the file is rewritten from on every call. registered keys
	// the same set by secret, so a repeat call is cheap and idempotent;
	// rules is the same set again in the shape the renderer wants.
	entries    []secretreg.Entry
	registered map[string]bool
	rules      []redact.Rule
}

// deliver is Fatal without the exit: it hands the report to gosd-init when
// there is a gosd-init to hand it to, and says on this console what
// happened — the report itself when there's no gosd-init to read it, or
// only a short pointer to it when there is.
//
// The short pointer, not the full report, is the load-bearing half of this
// function (gosd-72ga). gosd-init's console tail is a verbatim copy of this
// process's own stdout/stderr — that's the whole point of it, for a panic
// that has no report of its own to declare — so anything printed here when
// handed is true is what a FUTURE crash report's Detail could contain. A
// full report printed here would, the moment gosd-init folds this run's own
// tail into the very report it's about to write, put a complete second copy
// of the report inside itself: thinner, since this process can't know the
// device model, uptime or boot count, and therefore contradicting the real
// header a few lines above it. gosd-init writes and logs the genuine
// article once it has the report in hand (see fatalReporter.record) — this
// process saying anything more here would only be a worse rendering of it.
func (r *reporter) deliver(rep Report) {
	r.mu.Lock()
	defer r.mu.Unlock()

	report := faultreport.Report{
		Code:    rep.Code,
		Doing:   rep.Doing,
		Problem: rep.Problem,
		Fix:     rep.Fix,
		Detail:  detailText(rep.Detail),
	}

	handed, handoffErr := false, error(nil)
	if r.dir != "" {
		handoffErr = r.handOver(report)
		handed = handoffErr == nil
	}

	if handed {
		code := report.Code
		if code == "" {
			code = faultreport.UnspecifiedCode
		}
		r.warn("%s — handed to gosd-init; see %s on the boot partition; this device now stays down until someone power-cycles it", code, faultreport.FileName)
		return
	}

	rendered := faultreport.Render(report, r.context())
	_, _ = fmt.Fprintf(r.out, "%s\n", rendered.Markdown)
	for _, replacement := range rendered.SkippedSecrets {
		r.warn("%s is shorter than gosd redacts, so its value is printed above as it stands", replacement)
	}

	if handoffErr != nil {
		r.warn("handing this report to gosd-init failed (%v), so it is on this console and nowhere else; nothing will be written to the card", handoffErr)
	} else {
		r.warn("this isn't a GoSD device, so the report above was printed rather than written to %s on a card; on a device the board would stop here and stay down until it was power-cycled", faultreport.FileName)
	}
}

// context is everything about the report's header this process can honestly
// claim for itself, used only when a report is actually going to be printed
// to this console: the off-device developer preview, and the fallback print
// when a handoff to gosd-init has failed and nothing will reach the card
// either way. It never fills in the device model, uptime or boot count —
// this process can never know them, unlike gosd-init, which reads them
// fresh at record time (see faultreport.Context) — and marks Preview only
// for the genuinely off-device case, so faultreport omits an "unknown" line
// there instead of printing one for something this render was never going
// to know regardless (a report actually on a device still says "unknown"
// honestly: there it's diagnostic, not a stand-in for "off device").
// Callers must hold r.mu.
func (r *reporter) context() faultreport.Context {
	ctx := faultreport.Context{Secrets: r.rules}
	if r.dir != "" {
		return ctx
	}
	ctx.Preview = true
	// This binary's own name IS the app's name off a device: gosd derives
	// the name it bakes from the same main package. On a device the
	// binary is always /app, which would name nothing useful.
	ctx.AppName = appName()
	ctx.Timestamp = time.Now()
	ctx.ClockSynced = true
	return ctx
}

// handOver writes the drop file gosd-init picks up when the app exits.
// Callers must hold r.mu.
func (r *reporter) handOver(report faultreport.Report) error {
	data, err := faultdrop.Marshal(report)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(r.dir, filepath.Base(faultdrop.Path)), data, 0o600)
}

// register adds one secret to the set and rewrites the registration file
// from the whole set, refusing anything the reader wouldn't read back
// (see [secretreg.Encode]) rather than writing a file that would be
// dropped entirely.
func (r *reporter) register(secret, replacement string) {
	if secret == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.registered[secret] {
		return
	}

	label := replacement
	// A label quoting the secret would write the secret into every report
	// that redacted it — the exact leak this call exists to prevent.
	if label == "" || strings.Contains(label, secret) {
		label = unnamedSecret
	}

	entries := append(slices.Clone(r.entries), secretreg.Entry{Secret: secret, Replacement: label})
	data, err := secretreg.Encode(entries)
	if err != nil {
		r.warn("%s was not registered: %v; everything registered before it is unaffected", secretreg.Label(label), err)
		return
	}

	r.entries = entries
	r.rules = append(r.rules, redact.Rule{Needle: secret, Replacement: secretreg.Label(label)})
	if r.registered == nil {
		r.registered = make(map[string]bool)
	}
	r.registered[secret] = true

	if r.dir == "" {
		return
	}
	if err := writeAtomic(filepath.Join(r.dir, filepath.Base(secretreg.Path)), data, 0o600); err != nil {
		r.warn("telling gosd-init about %s failed (%v); a crash report written by the device may show its value", secretreg.Label(label), err)
	}
}

// warn prints one prefixed line to the same stream the report goes to:
// what happened to the report just printed, or what this package couldn't
// do. Every call site passes a replacement label, never a needle — these
// lines must be as safe to read as the reports they accompany.
func (r *reporter) warn(format string, args ...any) {
	_, _ = fmt.Fprintf(r.out, "gosd/fault: "+format+"\n", args...)
}

// writeAtomic writes data at path so that a reader watching for that exact
// path — gosd-init, in production — sees either the whole previous file or
// the whole new one: write a .tmp beside it, then rename, which is atomic
// within a directory. There is no fsync in that sequence and there should
// not be, since both files live on tmpfs: there is no durability to buy,
// and a power cut takes the whole filesystem with it either way.
//
// A stale .tmp is removed rather than written over, so one left behind by
// an interrupted write can't keep permissions this call meant to set.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// detailText renders a Report's Detail for the report body. A nil error is
// no detail at all, which the renderer says plainly rather than printing an
// empty section.
func detailText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// appName names the running binary, or "" when argv[0] says nothing useful.
func appName() string {
	name := filepath.Base(os.Args[0])
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	return name
}
