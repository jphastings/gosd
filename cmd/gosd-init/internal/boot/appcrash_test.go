package boot

import (
	"strings"
	"syscall"
	"testing"
)

func TestIsCrash(t *testing.T) {
	cases := []struct {
		name   string
		status ExitStatus
		want   bool
	}{
		{"clean exit 0", ExitStatus{ExitCode: 0}, false},
		{"non-zero exit", ExitStatus{ExitCode: 1}, true},
		{"signal death", ExitStatus{Signaled: true, Signal: syscall.SIGSEGV, ExitCode: -1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCrash(tc.status); got != tc.want {
				t.Errorf("isCrash(%+v) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestCrashProblemNamesSignalsInHumanTerms pins the bean's explicit
// requirement: "ran out of memory", not "signal 9".
func TestCrashProblemNamesSignalsInHumanTerms(t *testing.T) {
	cases := []struct {
		name   string
		status ExitStatus
		want   string
	}{
		{
			"OOM kill",
			ExitStatus{Signaled: true, Signal: syscall.SIGKILL, ExitCode: -1},
			"ran out of memory",
		},
		{
			"segfault",
			ExitStatus{Signaled: true, Signal: syscall.SIGSEGV, ExitCode: -1},
			"segmentation fault",
		},
		{
			"non-zero exit",
			ExitStatus{ExitCode: 2},
			"status 2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := crashProblem(tc.status)
			if !strings.Contains(got, tc.want) {
				t.Errorf("crashProblem(%+v) = %q, want it to contain %q", tc.status, got, tc.want)
			}
			if strings.Contains(got, "signal 9") {
				t.Errorf("crashProblem(%+v) = %q, leaked the bare signal number instead of naming it", tc.status, got)
			}
		})
	}
}

func TestCrashProblemFallsBackToTheSignalNumberForAnUnnamedSignal(t *testing.T) {
	// Not every signal earns a bespoke explanation; an unhandled one must
	// still say something concrete rather than nothing at all.
	got := crashProblem(ExitStatus{Signaled: true, Signal: syscall.SIGTRAP, ExitCode: -1})
	if !strings.Contains(got, "signal 5") {
		t.Errorf("crashProblem() = %q, want it to name the signal number for an unhandled signal", got)
	}
}

func TestNewAppCrashReportCarriesTheTailVerbatimAsDetail(t *testing.T) {
	report := newAppCrashReport(ExitStatus{ExitCode: 2}, "panic: something went wrong\ngoroutine 1 [running]:\n")

	if report.Code != appCrashCode {
		t.Errorf("Code = %q, want %q", report.Code, appCrashCode)
	}
	if report.Detail != "panic: something went wrong\ngoroutine 1 [running]:\n" {
		t.Errorf("Detail = %q, want the tail reproduced verbatim", report.Detail)
	}
	if report.Doing == "" {
		t.Error("Doing is empty; the rendered sentence would read 'Your device stopped.' with no context")
	}
}
