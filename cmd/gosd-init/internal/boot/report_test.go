package boot

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/faultreport"
	"github.com/jphastings/gosd/internal/redact"
)

func testReporter(f *fakeFaultReport) *fatalReporter {
	return newFatalReporter(
		Deps{FaultReport: f.deps(), Now: func() time.Time { return time.Unix(0, 0) }},
		func(string, ...any) {},
		faultreport.Context{AppName: "myapp", BoardID: "pi-zero-2w"},
	)
}

func TestFatalReporterWritesOneReportPerStableRunCycle(t *testing.T) {
	// A remount-rw is the one moment a power cut can damage the boot FAT,
	// so a crash loop must not turn one write into one per restart. The
	// first failure is recorded; the next is only narrated — until the app
	// proves it can run, which starts a fresh cycle.
	f := &fakeFaultReport{}
	report := testReporter(f)

	report.record(faultreport.Report{Code: "GOSD-APP-CRASH"})
	report.record(faultreport.Report{Code: "GOSD-APP-CRASH"})
	if got := f.writeCount(); got != 1 {
		t.Errorf("wrote %d reports during one crash loop, want 1", got)
	}

	report.markStableRun()
	report.record(faultreport.Report{Code: "GOSD-APP-CRASH"})
	if got := f.writeCount(); got != 2 {
		t.Errorf("wrote %d reports across two crash loops, want 2", got)
	}
}

// TestFatalReporterBoundsTotalWritesPerBoot pins the gosd-s9uq re-arming
// bound: an app that reliably crashes just after StableRunThreshold cycles
// markStableRun (re-arm + cleanup) and record (write) forever, which armed
// alone doesn't cap. Simulating 50 such cycles must still leave the total
// write count at maxReportsPerBoot, not 50.
func TestFatalReporterBoundsTotalWritesPerBoot(t *testing.T) {
	f := &fakeFaultReport{}
	report := testReporter(f)

	for range 50 {
		report.markStableRun()
		report.record(faultreport.Report{Code: "GOSD-APP-CRASH"})
	}

	if got := f.writeCount(); got != maxReportsPerBoot {
		t.Errorf("wrote %d reports across 50 crash/recover cycles, want exactly the cap (%d)", got, maxReportsPerBoot)
	}
}

// TestFatalReporterCleansUpOnceAfterTheCapIsHit proves the cap's promise
// that a device which genuinely recovers after its last recorded crash
// still ends up looking recovered: cleanup isn't gated by the cap, only new
// writes are, so the final stale report is still deleted exactly once.
func TestFatalReporterCleansUpOnceAfterTheCapIsHit(t *testing.T) {
	f := &fakeFaultReport{}
	report := testReporter(f)

	for range maxReportsPerBoot {
		report.markStableRun()
		report.record(faultreport.Report{Code: "GOSD-APP-CRASH"})
	}
	if got := f.writeCount(); got != maxReportsPerBoot {
		t.Fatalf("wrote %d reports reaching the cap, want %d", got, maxReportsPerBoot)
	}

	// The app now recovers for good: no more crashes, just repeated stable
	// runs (the OnStableRun timer fires on every tick regardless of whether
	// the app ever crashes again).
	removalsBefore := len(f.removals())
	for range 20 {
		report.markStableRun()
	}

	removals := f.removals()
	if len(removals) != removalsBefore+1 {
		t.Fatalf("cleanup ran %d times after the cap, want exactly 1 (then nothing left to delete)", len(removals)-removalsBefore)
	}
	if got := f.writeCount(); got != maxReportsPerBoot {
		t.Errorf("wrote %d reports after the cap, want it to stay at %d", got, maxReportsPerBoot)
	}
}

func TestFatalReporterLeavesAHealthyCardAlone(t *testing.T) {
	// The staleness rule fires on every stable run, which on a working
	// device is every run. It must cost nothing: no report on the card
	// means no remount at all.
	f := &fakeFaultReport{}

	testReporter(f).markStableRun()

	if removals := f.removals(); len(removals) != 0 {
		t.Errorf("remounted the boot partition to delete %v, though there was nothing to delete", removals)
	}
}

func TestFatalReporterDeletesAStaleReportOnceTheAppRunsStably(t *testing.T) {
	// A device that crashed last week and has been fine since must not
	// still look broken to whoever picks up the card.
	f := &fakeFaultReport{present: map[string]bool{faultreport.FileName: true}}

	testReporter(f).markStableRun()

	removals := f.removals()
	if len(removals) != 1 || len(removals[0]) != 1 || removals[0][0] != faultreport.FileName {
		t.Errorf("deleted %v, want exactly [%s]", removals, faultreport.FileName)
	}
}

func TestFatalReporterDeletesTheLegacyLogItReplaces(t *testing.T) {
	// A card flashed by a release older than gosd-pun9 can still carry
	// boot-failure.log. Leaving it beside a new report gives its owner two
	// files disagreeing about what went wrong.
	f := &fakeFaultReport{present: map[string]bool{faultreport.LegacyFileName: true}}

	testReporter(f).record(faultreport.Report{Code: "GOSD-DATA-CORRUPT"})

	removals := f.removals()
	if len(removals) != 1 || len(removals[0]) != 1 || removals[0][0] != faultreport.LegacyFileName {
		t.Errorf("deleted %v, want exactly [%s]", removals, faultreport.LegacyFileName)
	}
}

func TestFatalReporterStaysArmedWhenTheWriteFails(t *testing.T) {
	// A failed write recorded nothing, so the gate must not close on it:
	// the next failure is still worth trying to record.
	f := &fakeFaultReport{writeErr: errors.New("card gone")}
	report := testReporter(f)

	report.record(faultreport.Report{Code: "GOSD-APP-CRASH"})
	f.writeErr = nil
	report.record(faultreport.Report{Code: "GOSD-APP-CRASH"})

	if got := f.writeCount(); got != 1 {
		t.Errorf("wrote %d reports, want the second attempt to have succeeded", got)
	}
}

func TestFatalReporterGathersTheHeaderAtTheMomentOfTheFailure(t *testing.T) {
	f := &fakeFaultReport{
		deviceModel: "Raspberry Pi Zero 2 W Rev 1.0",
		uptime:      4*time.Minute + 12*time.Second,
		uptimeKnown: true,
		clockSynced: true,
	}
	report := testReporter(f)
	report.setBootCount(37)

	report.record(faultreport.Report{Code: "GOSD-DATA-CORRUPT"})

	for _, want := range []string{
		"uptime: 4m12s",
		"boot: 37",
		"device: Raspberry Pi Zero 2 W Rev 1.0 (pi-zero-2w)",
		"clock: ntp-synced",
	} {
		if !strings.Contains(f.written(), want) {
			t.Errorf("report header is missing %q:\n%s", want, f.written())
		}
	}
}

func TestFatalReporterRedactsSecretsSetAfterConstruction(t *testing.T) {
	// setSecrets exists precisely because the reporter is constructed
	// before the app env is assembled (see sequence.go's ordering
	// comment); this proves a rule handed over that way actually reaches
	// the rendered report.
	f := &fakeFaultReport{}
	report := testReporter(f)
	report.setSecrets([]redact.Rule{{Needle: "sk_live_super_secret_key", Replacement: "{$STRIPE_KEY}"}})

	report.record(faultreport.Report{Code: "GOSD-APP-CRASH", Detail: "panic: auth failed with key sk_live_super_secret_key"})

	written := f.written()
	if strings.Contains(written, "sk_live_super_secret_key") {
		t.Errorf("rendered report still contains the secret value:\n%s", written)
	}
	if !strings.Contains(written, "{$STRIPE_KEY}") {
		t.Errorf("rendered report is missing the redaction placeholder:\n%s", written)
	}
}

func TestFatalReporterReadsRegisteredSecretsFreshAtEachRecord(t *testing.T) {
	// A registration made moments before a crash must still redact: proven
	// here by registering only between two record() calls, across two
	// stable-run cycles, and checking the first is unredacted while the
	// second is.
	f := &fakeFaultReport{}
	report := testReporter(f)

	report.record(faultreport.Report{Code: "GOSD-APP-CRASH", Detail: "token=abcdef0123456789 rejected"})
	if !strings.Contains(f.written(), "abcdef0123456789") {
		t.Fatalf("first report was redacted before anything was registered:\n%s", f.written())
	}

	report.markStableRun()
	f.setRegisteredSecrets([]redact.Rule{{Needle: "abcdef0123456789", Replacement: "{secret: session-token}"}})
	report.record(faultreport.Report{Code: "GOSD-APP-CRASH", Detail: "token=abcdef0123456789 rejected"})

	written := f.written()
	if strings.Contains(written, "abcdef0123456789") {
		t.Errorf("second report still contains the registered secret:\n%s", written)
	}
	if !strings.Contains(written, "{secret: session-token}") {
		t.Errorf("second report is missing the registered-secret placeholder:\n%s", written)
	}
}

func TestFatalReporterCombinesEnvAndRegisteredSecrets(t *testing.T) {
	// The two mechanisms are independent seams (setSecrets for the static
	// env scan, RegisteredSecrets for the dynamic /run channel) that both
	// have to land in the same rendered report.
	f := &fakeFaultReport{}
	f.setRegisteredSecrets([]redact.Rule{{Needle: "registered-secret-value", Replacement: "{secret: api-token}"}})
	report := testReporter(f)
	report.setSecrets([]redact.Rule{{Needle: "env-var-secret-value", Replacement: "{$API_KEY}"}})

	report.record(faultreport.Report{
		Code:   "GOSD-APP-CRASH",
		Detail: "env-var-secret-value and registered-secret-value both appeared",
	})

	written := f.written()
	for _, secret := range []string{"env-var-secret-value", "registered-secret-value"} {
		if strings.Contains(written, secret) {
			t.Errorf("rendered report still contains %q:\n%s", secret, written)
		}
	}
	for _, placeholder := range []string{"{$API_KEY}", "{secret: api-token}"} {
		if !strings.Contains(written, placeholder) {
			t.Errorf("rendered report is missing %q:\n%s", placeholder, written)
		}
	}
}

func TestFatalReporterLeavesOrdinaryShortValuesReadable(t *testing.T) {
	// Over-redaction is the failure mode that matters more than a missed
	// match (see redact.MinNeedleLength's doc): an app with everyday
	// config like DEBUG=1 or PORT=80 must still get a legible report, not
	// one where every "1" and "80" has been blanked out.
	f := &fakeFaultReport{}
	report := testReporter(f)
	report.setSecrets(envRedactionRules([]string{"DEBUG=1", "PORT=80", "STRIPE_KEY=sk_live_51H_reallysecret"}))

	report.record(faultreport.Report{
		Code:   "GOSD-APP-CRASH",
		Detail: "panic at line 180: debug=1, key sk_live_51H_reallysecret rejected",
	})

	written := f.written()
	if !strings.Contains(written, "line 180") || !strings.Contains(written, "debug=1") {
		t.Errorf("short values were swept out of an unrelated context, wrecking readability:\n%s", written)
	}
	if strings.Contains(written, "sk_live_51H_reallysecret") {
		t.Errorf("the genuinely long secret survived into the report:\n%s", written)
	}
	if !strings.Contains(written, "{$STRIPE_KEY}") {
		t.Errorf("the long secret's placeholder is missing:\n%s", written)
	}
}

// TestFatalReporterLogsTheFullReportToTheConsole proves gosd-init logs the
// exact bytes it just committed to the card, on the console, every time it
// records a report. This is the copy gosd-72ga's fix relies on: it can never
// be folded back into a future report's technical detail (that only ever
// happens to /app's own stdout/stderr — see sequence.go's appOutput tee),
// unlike anything the declaring app could print for itself, and it is
// strictly more complete since it carries the device model, uptime and boot
// count only gosd-init ever knows.
func TestFatalReporterLogsTheFullReportToTheConsole(t *testing.T) {
	f := &fakeFaultReport{
		deviceModel: "FriendlyElec NanoPi Zero2",
		uptime:      3 * time.Second,
		uptimeKnown: true,
		clockSynced: true,
	}
	var logged []string
	report := newFatalReporter(
		Deps{FaultReport: f.deps(), Now: func() time.Time { return time.Unix(0, 0) }},
		func(format string, args ...any) { logged = append(logged, fmt.Sprintf(format, args...)) },
		faultreport.Context{AppName: "hello", BoardID: "nanopi-zero2"},
	)
	report.setBootCount(1)

	report.record(faultreport.Report{Code: "HELLO-DEMO-FATAL", Problem: "HELLO_FATAL was set"})

	written := f.written()
	if written == "" {
		t.Fatal("nothing was written to the card")
	}

	if console := strings.Join(logged, "\n"); !strings.Contains(console, written) {
		t.Errorf("the console log does not contain the exact report the card received:\nconsole:\n%s\nwant it to contain:\n%s", console, written)
	}
}

func TestNewFatalReporterIsNilWithNowhereToWrite(t *testing.T) {
	// The nil reporter is the pre-boot-mount state, and every method on it
	// has to be a no-op rather than a panic in PID 1.
	report := newFatalReporter(Deps{}, func(string, ...any) {}, faultreport.Context{})
	if report != nil {
		t.Fatalf("newFatalReporter() = %v, want nil when there's nowhere to write", report)
	}

	report.setBootCount(1)
	report.setSecrets([]redact.Rule{{Needle: "irrelevant", Replacement: "{$X}"}})
	report.markStableRun()
	if report.record(faultreport.Report{Code: "X"}) {
		t.Error("a nil reporter claimed to have recorded a report")
	}
}

func TestParseDeviceModel(t *testing.T) {
	// Device-tree strings are NUL-terminated, and sysfs hands the property
	// over verbatim.
	if got := parseDeviceModel([]byte("Raspberry Pi Zero 2 W Rev 1.0\x00")); got != "Raspberry Pi Zero 2 W Rev 1.0" {
		t.Errorf("parseDeviceModel() = %q, want the trimmed model", got)
	}
	if got := parseDeviceModel(nil); got != "" {
		t.Errorf("parseDeviceModel(nil) = %q, want empty", got)
	}
}

func TestParseUptime(t *testing.T) {
	cases := map[string]struct {
		raw   string
		want  time.Duration
		wantK bool
	}{
		"proc uptime":  {"22615.44 43120.53\n", 22615440 * time.Millisecond, true},
		"just booted":  {"0.31 0.05\n", 310 * time.Millisecond, true},
		"not a number": {"garbage\n", 0, false},
		"empty":        {"", 0, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := parseUptime(tc.raw)
			if ok != tc.wantK || got.Round(time.Millisecond) != tc.want {
				t.Errorf("parseUptime(%q) = %v, %t; want %v, %t", tc.raw, got, ok, tc.want, tc.wantK)
			}
		})
	}
}
