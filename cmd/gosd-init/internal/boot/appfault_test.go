package boot

import (
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/faultreport"
)

// declaredFault wires a reporter, a rebooter and a drop-file pickup
// together the way Run does.
func declaredFault(t *testing.T, f *fakeFaultReport, take func() (faultreport.Report, bool)) (Deps, *fatalReporter, *fakeRebooter) {
	t.Helper()
	rebooter := &fakeRebooter{}
	deps := Deps{FaultReport: f.deps(), Rebooter: rebooter, Now: func() time.Time { return time.Unix(0, 0) }}
	deps.FaultReport.AppFault = take
	reporter := newFatalReporter(deps, func(string, ...any) {}, faultreport.Context{AppName: "myapp"})
	return deps, reporter, rebooter
}

func TestAFaultTheAppDeclaredReachesTheCardAndStopsTheDevice(t *testing.T) {
	// The locked half of fault.Fatal: an app that declares a fatal is
	// asserting no restart can help, so the device stays down with the
	// report on the card rather than looping.
	f := &fakeFaultReport{}
	deps, reporter, rebooter := declaredFault(t, f, func() (faultreport.Report, bool) {
		return faultreport.Report{Code: "NO-API-KEY", Problem: "the weather service rejected our API key"}, true
	})

	declared, ok := appFault(deps)
	if !ok {
		t.Fatal("the declared fault was not picked up")
	}
	haltForAppFault(deps, func(string, ...any) {}, reporter, declared, "panic: nil map write")

	if !rebooter.halted {
		t.Error("the device was not halted")
	}
	if rebooter.rebooted {
		t.Error("the device rebooted, which would bury the report under a crash loop")
	}
	if rebooter.syncCalls == 0 {
		t.Error("the card was not synced before halting")
	}

	if f.writeCount() != 1 || !strings.Contains(f.written(), "the weather service rejected our API key") {
		t.Errorf("wrote %d reports (last: %q), want exactly the app's own", f.writeCount(), f.written())
	}
	if !strings.Contains(f.written(), "panic: nil map write") {
		t.Errorf("wrote %q, want the console tail kept as technical detail", f.written())
	}
}

func TestAnExitThatDeclaresNothingIsLeftToTheSupervisor(t *testing.T) {
	// An ordinary crash is not this path's business: nobody classified
	// it, transience is unknowable, and the supervisor restarts it.
	deps, _, _ := declaredFault(t, &fakeFaultReport{}, func() (faultreport.Report, bool) {
		return faultreport.Report{}, false
	})

	if _, ok := appFault(deps); ok {
		t.Error("an exit that declared nothing was read as a declared fault")
	}
}

func TestWithoutAWayToPickUpAFaultNothingIsDeclared(t *testing.T) {
	deps, _, _ := declaredFault(t, &fakeFaultReport{}, nil)

	if _, ok := appFault(deps); ok {
		t.Error("a fault was reported with no drop file to read it from")
	}
}

func TestSupervisionEndsWhenTheExitHookAsksItTo(t *testing.T) {
	starts := 0
	sup := &Supervisor{
		Start:       func() (int, error) { starts++; return starts, nil },
		Wait:        func(int) (ExitStatus, error) { return ExitStatus{ExitCode: 70}, nil },
		Sleep:       func(time.Duration) { t.Error("the supervisor waited to restart an app that declared a fatal fault") },
		Now:         newFakeClock(time.Unix(0, 0)).Now,
		Backoff:     NewBackoff(time.Second, time.Minute),
		StableAfter: 30 * time.Second,
		OnExit:      func(ExitStatus, time.Duration) bool { return true },
		Log:         func(string, ...any) {},
	}

	sup.Run(nil)

	if starts != 1 {
		t.Errorf("the app was started %d times, want 1: a declared fatal is not restarted", starts)
	}
}
