package boot

import (
	"github.com/jphastings/gosd/internal/faultreport"
)

// appFault picks up a fault the app declared for itself through the public
// fault package (bean gosd-aa1p): the drop file it left in /run before
// exiting. It reports false for the ordinary case — most exits declare
// nothing — and for a gosd-init with no way to read one at all.
func appFault(deps Deps) (faultreport.Report, bool) {
	if deps.FaultReport.AppFault == nil {
		return faultreport.Report{}, false
	}
	return deps.FaultReport.AppFault()
}

// haltForAppFault commits a declared fault to the card and stops the
// device. Halting is the locked half of fault.Fatal: an app that declares a
// fatal is asserting that a restart cannot help, so restarting it would
// only grind the card and bury the report under a crash loop. This governs
// the DECLARED path alone — an app that merely crashes is still restarted
// with backoff, because nobody classified that failure and its transience
// is unknowable.
//
// tail is the console output captured for this run (gosd-s9uq), folded into
// the report by withConsoleTail; see there for why a declared report and a
// console tail are not alternatives.
//
// Like fatal(), this returns after asking for the halt: in production the
// machine is already on its way down, and the return only matters to tests.
func haltForAppFault(deps Deps, log func(format string, args ...any), reporter *fatalReporter, report faultreport.Report, tail string) {
	report = withConsoleTail(report, tail)

	declared := report.Code
	if declared == "" {
		declared = "a fatal fault"
	}
	log("fatal: the app declared %s and asked to stop: %s; halting", declared, report.Problem)
	reporter.record(report)

	deps.Rebooter.Sync()
	deps.Rebooter.Halt()
}

// withConsoleTail folds the console output captured for this run into a
// declared report's technical detail.
//
// The app's own report always wins the human sections: it knows what its
// user was promised and what would fix it, and a console tail never can.
// But the two are not alternatives — a fault.Fatal call on one goroutine
// and a panic on another can genuinely coincide, and when they do the panic
// is the part whoever gets the file forwarded to them needs. So both are
// kept, the app's own detail first.
func withConsoleTail(report faultreport.Report, tail string) faultreport.Report {
	if tail == "" {
		return report
	}
	if report.Detail == "" {
		report.Detail = tail
		return report
	}
	report.Detail += "\n\nconsole output up to the exit:\n\n" + tail
	return report
}
