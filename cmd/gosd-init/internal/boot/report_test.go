package boot

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/faultreport"
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

func TestNewFatalReporterIsNilWithNowhereToWrite(t *testing.T) {
	// The nil reporter is the pre-boot-mount state, and every method on it
	// has to be a no-op rather than a panic in PID 1.
	report := newFatalReporter(Deps{}, func(string, ...any) {}, faultreport.Context{})
	if report != nil {
		t.Fatalf("newFatalReporter() = %v, want nil when there's nowhere to write", report)
	}

	report.setBootCount(1)
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
