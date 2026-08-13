package boot

import (
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jphastings/gosd/internal/faultreport"
	"github.com/jphastings/gosd/internal/redact"
)

// parseDeviceModel extracts the model string from the raw contents of the
// device tree's model property. Device-tree strings are NUL-terminated, and
// the kernel exposes the property's bytes verbatim, so the trailing NUL is
// part of what's read — left in place it would be written straight into the
// report's header.
func parseDeviceModel(raw []byte) string {
	return strings.TrimSpace(strings.ReplaceAll(string(raw), "\x00", ""))
}

// parseUptime reads /proc/uptime's first field: seconds since boot, with two
// decimal places, followed by the idle-time field this doesn't care about.
func parseUptime(raw string) (time.Duration, bool) {
	seconds, _, _ := strings.Cut(strings.TrimSpace(raw), " ")
	value, err := strconv.ParseFloat(seconds, 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return time.Duration(value * float64(time.Second)), true
}

// FaultReportDeps bundles everything the crash-report mechanism needs from
// the platform: LAST_FATAL_ERROR.md is written to the boot partition, but
// the facts in its header come from the device tree, /proc and the data
// partition. Every field is nil-checked — a Deps with none of them set
// simply never writes a report, which is what the pure-logic tests and any
// caller that doesn't care about crash reporting get.
//
// Only Write is load-bearing: with a nil Write there is nowhere to put a
// report, so gosd-init doesn't assemble one at all and the serial console
// stays the only record.
type FaultReportDeps struct {
	// Write records a rendered report as LAST_FATAL_ERROR.md at the root
	// of the (normally read-only) boot partition, briefly remounting it
	// read-write. The file is overwritten, never appended: it always
	// describes the latest fatal issue, which is the one whoever collects
	// the device needs.
	Write func(body string) error

	// Exists reports whether name is present at the root of the boot
	// partition — a plain read against the read-only mount. It is what
	// keeps the "delete a stale report" rule free: a healthy device that
	// has never crashed answers false and never remounts read-write at
	// all. A remount-rw is the one window in which a power cut can damage
	// the boot FAT, so this check is not an optimisation.
	Exists func(name string) bool

	// Remove deletes names from the root of the boot partition, briefly
	// remounting it read-write. Only ever called once Exists has confirmed
	// there is something to delete.
	Remove func(names []string) error

	// DeviceModel returns the hardware's own self-description from the
	// device tree, or "" when it can't be read. Called at report time
	// rather than at boot: /sys is mounted for the life of the device, so
	// a boot that never crashes never pays for the read.
	DeviceModel func() string

	// Uptime reports how long the machine has been up, and whether that
	// could be measured at all. Unlike the wall clock this is always
	// true when present — see faultreport.Context.
	Uptime func() (time.Duration, bool)

	// ClockSynced reports whether the clock has been set from a time
	// server this boot, deciding whether the report's timestamp is
	// trustworthy enough to print.
	ClockSynced func() bool

	// CountBoot records this boot in the durable counter on the data
	// partition and returns its number, reporting false when there is
	// nowhere to keep it (a read-only or absent /data). Unlike the other
	// fields here it is called exactly once per boot, since it is the one
	// that writes.
	CountBoot func() (int, bool)

	// AppFault reads and removes the /run drop file a fault.Fatal call
	// leaves behind (see internal/faultdrop), reporting false — the
	// common case, since most exits declare nothing — when the app named
	// no fault of its own. It is called after every exit of /app, so it
	// must consume the file rather than merely read it: a report is
	// delivered once, to the exit that raised it. A nil AppFault means
	// declared faults are never picked up, which is what the pure-logic
	// tests get.
	AppFault func() (faultreport.Report, bool)

	// RegisteredSecrets reads the /run registration file
	// fault.RegisterSecretString writes (see internal/secretreg), fresh at
	// the moment of every report rather than once at boot: a registration
	// made moments before a panic must still redact. Like DeviceModel and
	// Uptime, a nil result (nothing registered, or a registration file
	// secretreg.Parse won't trust — see its doc) simply means this report
	// carries no rules from this source, never an error.
	RegisteredSecrets func() []redact.Rule
}

// maxReportsPerBoot bounds how many times fatalReporter.record ever actually
// writes LAST_FATAL_ERROR.md in a single boot, regardless of how many
// stable-run cycles occur. armed alone only bounds writes to one per crash
// LOOP, not one per BOOT — and gosd-s9uq's adversarial pass found the gap
// that leaves open: an app that dies just after StableRunThreshold produces
// a delete-then-write pair of boot-FAT remounts every cycle
// (markStableRun's cleanup, followed by the next record()), roughly 200
// remounts an hour at gosd-init's ~35s crash/recover cadence — for as long
// as the device stays up, which for a crash-looping /app under a live PID 1
// can be indefinite. Every fatal gosd-init raises for ITSELF halts or
// reboots before a second cycle can happen, so in practice this only ever
// binds the app-crash path (gosd-s9uq), which is the first thing that makes
// a fatal recoverable and therefore repeatable.
//
// The cap gates only NEW writes, not cleanup: once it's reached, record
// simply stops refreshing the card, but markStableRun still deletes
// whatever report is left the next time the app proves stable (that delete
// isn't gated by armed either — see cleanUp). So a device that genuinely
// recovers after its last recorded crash still ends up looking recovered;
// it just can't be told about any FURTHER crash until the next reboot resets
// the counter. That caps this boot's total fault-reporter remounts at
// 2*maxReportsPerBoot+1 (each write, plus at most one trailing cleanup)
// however long the crash loop runs — a hard ceiling on the total, not merely
// a lower rate that still accumulates without bound over a long enough
// uptime.
const maxReportsPerBoot = 10

// fatalReporter owns LAST_FATAL_ERROR.md for the life of one boot: it holds
// the parts of the report header that don't change (which app, which board,
// which image), fills in the parts that do at the moment of a failure, and
// enforces the rules that keep the file honest and its writes rare.
//
// The first rule is one report per stable-run cycle. Writing means
// remounting the boot partition read-write, which is the one moment a power
// cut can damage the boot FAT; that risk was accepted on the basis that
// writes are tiny and rare, so a crash loop must not turn it into a write
// per restart. The first failure after the app last ran stably is recorded;
// the rest only narrate to the console.
//
// The second is that a recovered device must not look broken: once the app
// has run stably, any report left on the card is deleted.
//
// The third — added by gosd-s9uq, see maxReportsPerBoot — is that a boot
// only ever writes so many reports in total, however many times the first
// two rules cycle: the first two alone bound the rate of an indefinite crash
// loop, not its total cost over an indefinitely long uptime.
//
// All three are enforced here rather than at each call site so that every
// producer — the fatal paths below, the app-crash tail, and later the public
// fault package — inherits them.
//
// A nil *fatalReporter is usable and does nothing, which is the state before
// the boot partition is mounted; see fatal's enumeration of the failures
// that can never be recorded.
type fatalReporter struct {
	deps FaultReportDeps
	log  func(format string, args ...any)
	now  func() time.Time

	mu sync.Mutex
	// ctx is every header fact known before a failure happens. The rest
	// (the clock, the uptime, the device tree) is gathered at record time.
	ctx faultreport.Context
	// armed is the one-report-per-stable-run-cycle gate.
	armed bool
	// writes counts how many reports this boot has actually written, so
	// record can enforce maxReportsPerBoot regardless of how many times
	// armed cycles between crash and recovery.
	writes int
}

// newFatalReporter returns a reporter for the mounted boot partition, or nil
// when there is nowhere to write a report.
func newFatalReporter(deps Deps, log func(format string, args ...any), ctx faultreport.Context) *fatalReporter {
	if deps.FaultReport.Write == nil {
		return nil
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &fatalReporter{deps: deps.FaultReport, log: log, now: now, ctx: ctx, armed: true}
}

// setBootCount records this boot's number for every report written from here
// on. It is separate from construction because the counter lives on the data
// partition, which is mounted well after the boot partition a report is
// written to — so a failure between the two (the data-corruption halt, most
// of all) honestly reports an unknown boot number rather than a stale one.
func (r *fatalReporter) setBootCount(count int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx.BootCount = count
}

// setSecrets records the redaction rules discovered from the app's own
// environment (sequence.go's envRedactionRules) for every report written
// from here on. Like setBootCount, this exists because the reporter is
// constructed well before the app env is assembled — mergeUserEnv only
// runs once the card's settings have been read — so there is no
// single moment before both are ready. Rules registered through
// fault.RegisterSecretString are NOT set this way: those are read fresh at
// every record() call (see headerNow), since a registration made moments
// before a crash has to count.
func (r *fatalReporter) setSecrets(rules []redact.Rule) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx.Secrets = rules
}

// record renders report and writes it to the boot partition, and reports
// whether it got there. A second failure in the same crash loop is narrated
// to the console and not written (see the type's doc); so is every failure
// once the write itself has failed.
func (r *fatalReporter) record(report faultreport.Report) bool {
	if r == nil {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.armed {
		r.log("not recording %s: %s already describes this run's first failure", report.Code, faultreport.FileName)
		return false
	}
	if r.writes >= maxReportsPerBoot {
		r.log("not recording %s: this boot has already written the most reports it ever will (%d); %s still shows the last one recorded", report.Code, maxReportsPerBoot, faultreport.FileName)
		return false
	}

	rendered := faultreport.Render(report, r.headerNow())
	for _, replacement := range rendered.SkippedSecrets {
		r.log("%s is too short to redact safely, so it was left as-is in %s", replacement, faultreport.FileName)
	}

	if err := r.deps.Write(rendered.Markdown); err != nil {
		r.log("recording this failure to %s on the boot partition failed: %v", faultreport.FileName, err)
		return false
	}
	r.armed = false
	r.writes++
	r.log("recorded this failure to %s at the root of the boot partition; the report follows on this console", faultreport.FileName)
	// gosd-init's own console line is the one copy of this report that can
	// never end up nested inside itself: consoletail only ever captures
	// /app's own stdout/stderr (sequence.go's appOutput tee), never what r.log
	// writes directly to the console, so this can't be folded into a future
	// report's Detail the way a printed-to-stderr copy from the app's own
	// process could (gosd-72ga). It is also strictly more complete than
	// anything an app could print for itself: the device model, uptime and
	// boot count below are only ever known here.
	r.log("%s", rendered.Markdown)

	// An upgraded card can still carry the file this one replaced. Deleting
	// it here as well as at markStableRun covers the device that crashes
	// before it ever runs stably, which is exactly the device whose owner
	// is about to go looking.
	r.cleanUp(faultreport.LegacyFileName)
	return true
}

// markStableRun is what makes a recovered device stop looking broken: the
// app has run long enough to count as working, so any report describing an
// earlier failure is deleted and the next failure is allowed to write a
// fresh one.
func (r *fatalReporter) markStableRun() {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.armed = true
	r.cleanUp(faultreport.FileName, faultreport.LegacyFileName)
}

// cleanUp deletes whichever of names are actually on the card, and touches
// nothing at all when none of them are. Callers must hold r.mu.
func (r *fatalReporter) cleanUp(names ...string) {
	if r.deps.Exists == nil || r.deps.Remove == nil {
		return
	}

	var present []string
	for _, name := range names {
		if r.deps.Exists(name) {
			present = append(present, name)
		}
	}
	if len(present) == 0 {
		return
	}

	if err := r.deps.Remove(present); err != nil {
		r.log("deleting %v from the boot partition failed; the card may still look like it crashed: %v", present, err)
		return
	}
	r.log("deleted %v from the boot partition: the app is running", present)
}

// headerNow completes the report header with everything that can only be
// known at the moment of the failure. Callers must hold r.mu.
func (r *fatalReporter) headerNow() faultreport.Context {
	ctx := r.ctx
	ctx.Timestamp = r.now()
	if r.deps.ClockSynced != nil {
		ctx.ClockSynced = r.deps.ClockSynced()
	}
	if r.deps.Uptime != nil {
		ctx.Uptime, ctx.UptimeKnown = r.deps.Uptime()
	}
	if r.deps.DeviceModel != nil {
		ctx.DeviceModel = r.deps.DeviceModel()
	}
	if r.deps.RegisteredSecrets != nil {
		// slices.Clone first: ctx.Secrets still shares r.ctx.Secrets's
		// backing array at this point, and appending in place could
		// clobber it under a concurrent record() call.
		ctx.Secrets = append(slices.Clone(ctx.Secrets), r.deps.RegisteredSecrets()...)
	}
	return ctx
}
