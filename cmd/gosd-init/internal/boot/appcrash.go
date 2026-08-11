package boot

import (
	"fmt"
	"syscall"

	"github.com/jphastings/gosd/internal/faultreport"
)

// appCrashCode is LAST_FATAL_ERROR.md's error_code for a crash gosd-init
// caught but /app never declared or explained itself — a panic, a segfault,
// an OOM kill, or any other non-zero/signaled exit. It is namespaced GOSD-*
// like gosd-init's own fatal classes (see the fatalClass table) even though
// the crash is the app's, because gosd-init is what raised the report: the
// app never got a chance to (see the epic's locked header-fields section).
const appCrashCode = "GOSD-APP-CRASH"

// isCrash reports whether an /app exit is worth a crash report: any signal
// death, or a non-zero exit code. Exit 0 is always clean — an app that
// deliberately exits 0 is not broken, even though the supervisor restarts it
// exactly the same as it would a crash (gosd-s9uq, locked).
func isCrash(status ExitStatus) bool {
	return status.Signaled || status.ExitCode != 0
}

// newAppCrashReport builds the report for an /app exit isCrash has already
// judged worth recording. tail is the console-tail buffer's current
// content, reproduced verbatim as the only technical detail this path can
// offer — there is no app-supplied context on this path at all (unlike a
// declared fault.Fatal or gosd-init's own fatal classes), so Doing and
// Problem are both deliberately generic. Detail is a clean seam a future,
// second source (the app's own drop-file report, gosd-aa1p) can combine
// with: it always carries the tail, so a later caller only has to decide
// whether to also override Problem/Fix/Doing with what the app declared
// about itself.
func newAppCrashReport(status ExitStatus, tail string) faultreport.Report {
	return faultreport.Report{
		Code:    appCrashCode,
		Doing:   "running",
		Problem: crashProblem(status),
		Detail:  tail,
	}
}

// crashProblem renders /app's exit in terms its owner can understand. A
// signal death is named plainly — gosd-s9uq requires "ran out of memory",
// not "signal 9" — and a bare non-zero exit code is the fallback for
// anything else abnormal, which covers an unrecovered Go panic: the Go
// runtime reports one via exit(2), not a signal.
func crashProblem(status ExitStatus) string {
	if status.Signaled {
		return "The app stopped unexpectedly: it " + signalDescription(status.Signal) + "."
	}
	return fmt.Sprintf("The app stopped unexpectedly, exiting with status %d instead of shutting down on its own.", status.ExitCode)
}

// signalDescription names a signal death the way its most common real-world
// cause on an unattended board actually reads, rather than the bare signal
// name: SIGKILL on a device with no shell and nobody to send it is
// overwhelmingly the OOM killer, not a person running `kill -9`. Every
// branch composes after "it " (see crashProblem), and the default falls back
// to the signal's own OS-provided description for anything not worth a
// bespoke explanation.
func signalDescription(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGKILL:
		return "was stopped by the operating system, most likely because it ran out of memory"
	case syscall.SIGSEGV:
		return "crashed with a segmentation fault, trying to use memory it wasn't allowed to"
	case syscall.SIGABRT:
		return "aborted itself, usually because an internal check failed"
	case syscall.SIGBUS:
		return "crashed with a bus error, usually from misaligned or invalid memory access"
	case syscall.SIGFPE:
		return "crashed from an arithmetic error, such as dividing by zero"
	case syscall.SIGILL:
		return "crashed after executing an invalid instruction, often a sign of memory corruption"
	default:
		return fmt.Sprintf("was stopped by signal %d (%s)", int(sig), sig)
	}
}
