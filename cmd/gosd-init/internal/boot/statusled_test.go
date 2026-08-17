package boot

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/dataexpand"
	"github.com/jphastings/gosd/internal/initcfg"
)

// TestRunSetsStatusLEDBootingThenRunning is the acceptance test for
// gosd-xtcs's two happy-path call sites: Booting as early as practical
// (right after the console opens) and Running exactly once, the first time
// /app starts successfully — not on every later restart, since a transient
// crash and its backoff-driven retry aren't one of the three states.
func TestRunSetsStatusLEDBootingThenRunning(t *testing.T) {
	led := &fakeStatusLED{}
	stop := make(chan struct{})
	starts := 0
	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		starts++
		if starts == 2 {
			close(stop)
		}
		return starts, nil
	})

	deps := Deps{
		Mounter:     &fakeMounter{},
		Hostname:    &fakeHostname{},
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Board: "pi-zero-2w"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       func(time.Duration) {},
		Now:         time.Now,
		StatusLED:   led,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	calls := led.callList()
	if len(calls) == 0 || calls[0] != "Booting" {
		t.Fatalf("status LED calls = %v, want Booting first", calls)
	}
	runningCount := 0
	for _, c := range calls {
		if c == "Running" {
			runningCount++
		}
	}
	if runningCount != 1 {
		t.Errorf("status LED calls = %v, want exactly one Running (only the first successful start hands over control)", calls)
	}
}

// TestRunDoesNotSetFatalStatusLEDOnARebootingFatal locks in gosd-xtcs's
// halt-only scope for the fatal blink: GOSD-BOOT-MOUNT reboots after 5s
// rather than halting, so the LED must stay on its booting blink — only a
// halt (fatal's own halt branch, or haltForAppFault) ever switches it.
func TestRunDoesNotSetFatalStatusLEDOnARebootingFatal(t *testing.T) {
	mounter := &fakeMounter{fn: func(c mountCall) error {
		if c.target == "/boot" {
			return errBoom
		}
		return nil
	}}
	led := &fakeStatusLED{}
	rebooter := &fakeRebooter{}
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration

	deps := testDepsForFatalPath(mounter, &fakeHostname{}, rebooter, clock, &sleeps)
	deps.StatusLED = led
	opts := testOptions()

	if err := Run(deps, opts); err == nil {
		t.Fatal("Run() = nil, want an error about mounting the boot partition")
	}

	calls := led.callList()
	if len(calls) != 1 || calls[0] != "Booting" {
		t.Errorf("status LED calls = %v, want exactly [Booting] (this fatal reboots, it never halts)", calls)
	}
}

// TestRunSetsFatalStatusLEDWhenHaltingOnDataCorruption exercises the other
// current halting fatal class (haltForAppFault has its own test in
// appfault_test.go) through fatal()'s own halt branch.
func TestRunSetsFatalStatusLEDWhenHaltingOnDataCorruption(t *testing.T) {
	mounter := &fakeMounter{}
	rebooter := &fakeRebooter{}
	led := &fakeStatusLED{}
	stop := make(chan struct{})
	var expandedWith []string

	deps := expandTestDeps(mounter, newFakeClock(time.Unix(0, 0)), stop, true,
		fmt.Errorf("%w: /dev/mmcblk0p2 holds nothing (blank space)", dataexpand.ErrDataCorrupt), &expandedWith)
	deps.Rebooter = rebooter
	deps.StatusLED = led
	deps.AppStarter = funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		t.Error("the app was started despite a corrupt data partition")
		close(stop)
		return 1, nil
	})
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err == nil {
		t.Fatal("Run() = nil, want the corruption error")
	}
	if !rebooter.halted {
		t.Fatal("the device was not halted")
	}

	want := []string{"Booting", "Fatal"}
	if calls := led.callList(); len(calls) != len(want) || calls[0] != want[0] || calls[1] != want[1] {
		t.Errorf("status LED calls = %v, want %v", calls, want)
	}
}
