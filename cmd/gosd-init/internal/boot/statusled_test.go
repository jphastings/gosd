package boot

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
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

// TestRunHoldsStatusLEDUntilAppSignalsReady is the acceptance test for
// gosd-42vb: with config.json's AppSignalsReady set, /app starting
// successfully must NOT flip the LED to Running by itself — only the app's
// own ready marker (deps.AppReady) does — and both documented log lines
// must appear, at the right points.
func TestRunHoldsStatusLEDUntilAppSignalsReady(t *testing.T) {
	led := &fakeStatusLED{}
	console := &bytes.Buffer{}
	stop := make(chan struct{})
	appExited := make(chan struct{})
	var marked atomic.Bool

	instantAfter := func(time.Duration) <-chan time.Time {
		c := make(chan time.Time, 1)
		c <- time.Now()
		return c
	}

	deps := Deps{
		Mounter:    &fakeMounter{},
		Hostname:   &fakeHostname{},
		AppStarter: funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) { return 1, nil }),
		Reaper: funcReaper(func(int) (ExitStatus, error) {
			<-appExited
			return ExitStatus{}, nil
		}),
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Board: "pi-zero-2w", AppSignalsReady: true}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       func(time.Duration) {},
		Now:         time.Now,
		After:       instantAfter,
		StatusLED:   led,
		AppReady:    marked.Load,
	}
	opts := testOptions()
	opts.Stop = stop

	done := make(chan struct{})
	go func() {
		if err := Run(deps, opts); err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
		close(done)
	}()

	// Give the poller several iterations against the still-unmarked
	// AppReady before asserting the LED hasn't moved past Booting.
	time.Sleep(50 * time.Millisecond)
	if calls := led.callList(); hasCall(calls, "Running") {
		t.Fatalf("status LED calls = %v, want no Running before the app signals ready", calls)
	}

	marked.Store(true)

	deadline := time.Now().Add(2 * time.Second)
	for !hasCall(led.callList(), "Running") {
		if time.Now().After(deadline) {
			t.Fatal("status LED never reached Running after the app signalled ready")
		}
		time.Sleep(time.Millisecond)
	}

	close(appExited)
	close(stop)
	<-done

	calls := led.callList()
	if len(calls) != 2 || calls[0] != "Booting" || calls[1] != "Running" {
		t.Errorf("status LED calls = %v, want exactly [Booting Running]", calls)
	}

	logOut := console.String()
	const holdingLine = `status LED: holding "booting" until the app calls ready.Signal() — it imports github.com/jphastings/gosd/ready, so the LED only shows "running" once that call is made`
	if !strings.Contains(logOut, holdingLine) {
		t.Errorf("console output missing the holding log line; got %q", logOut)
	}
	if !strings.Contains(logOut, `status LED: app signalled ready`) {
		t.Errorf("console output missing the app-signalled-ready log line; got %q", logOut)
	}
}

func hasCall(calls []string, want string) bool {
	for _, c := range calls {
		if c == want {
			return true
		}
	}
	return false
}
