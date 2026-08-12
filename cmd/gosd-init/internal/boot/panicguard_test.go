package boot

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/provision"
)

func TestGuardTurnsAPanicIntoAStackTraceAndAReboot(t *testing.T) {
	rebooter := &fakeRebooter{}
	log := &bytes.Buffer{}
	var slept []time.Duration
	guard := PanicGuard{
		Rebooter: rebooter,
		Sleep:    func(d time.Duration) { slept = append(slept, d) },
		Log:      NewLogger(log).Printf,
	}

	guard.Guard("netup", func() { panic("malformed packet") })

	if !rebooter.rebooted {
		t.Error("a panic in a guarded function did not reboot")
	}
	if rebooter.syncCalls == 0 {
		t.Error("a panic in a guarded function rebooted without syncing first")
	}
	// gosd-fs34: a panic's stack trace deserves the same flush-before-reboot
	// guarantee as the boot sequence's own fatal path.
	assertBefore(t, rebooter.callOrder(), "FlushConsole", "Reboot")
	if len(slept) != 1 || slept[0] != PanicRebootDelay {
		t.Errorf("sleeps before reboot = %v, want [%s]", slept, PanicRebootDelay)
	}
	for _, want := range []string{"panic in netup", "malformed packet", "goroutine"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("panic log = %q, want it to contain %q", log.String(), want)
		}
	}
}

func TestGoRebootsWhenTheGoroutinePanics(t *testing.T) {
	rebooter := &notifyingRebooter{rebooted: make(chan struct{})}
	guard := PanicGuard{Rebooter: rebooter, Sleep: func(time.Duration) {}}

	guard.Go("the mDNS responder", func() { panic("nil map write") })

	select {
	case <-rebooter.rebooted:
	case <-time.After(time.Second):
		t.Fatal("a panic in a guarded goroutine never rebooted")
	}
}

func TestGuardLeavesNonPanickingWorkAlone(t *testing.T) {
	rebooter := &fakeRebooter{}
	guard := PanicGuard{Rebooter: rebooter, Sleep: func(time.Duration) {}}

	ran := false
	guard.Guard("timesync", func() { ran = true })

	if !ran {
		t.Error("guarded function never ran")
	}
	if rebooter.rebooted {
		t.Error("a guarded function that returned normally still rebooted")
	}
}

func TestRunRebootsWhenNetworkingPanics(t *testing.T) {
	// The gosd-fkkr failure mode: a malformed packet panics a networking
	// loop, and without a guard that panic kills PID 1 outright.
	rebooter := &notifyingRebooter{rebooted: make(chan struct{})}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

	deps := testDeps(rebooter, clock, stop)
	deps.StartNetworking = func(initcfg.Config, gosdtoml.Config, []provision.WifiNetwork, func(string, ...any)) {
		panic("malformed multicast packet")
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	select {
	case <-rebooter.rebooted:
	case <-time.After(time.Second):
		t.Fatal("a panic in the networking goroutine never rebooted")
	}
}

func TestRunAndRebootRebootsWhenTheBootSequenceReturns(t *testing.T) {
	// PID 1 returning is a kernel panic, so even a clean return from the
	// boot sequence must become a reboot.
	rebooter := &fakeRebooter{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	close(stop)

	opts := testOptions()
	opts.Stop = stop

	RunAndReboot(testDeps(rebooter, clock, stop), opts)

	if !rebooter.rebooted {
		t.Error("RunAndReboot returned without rebooting")
	}
}

func TestRunAndRebootRebootsWhenTheBootSequencePanics(t *testing.T) {
	rebooter := &fakeRebooter{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

	deps := testDeps(rebooter, clock, stop)
	deps.ReadConfig = func() (initcfg.Config, error) { panic("corrupt config.json") }
	opts := testOptions()
	opts.Stop = stop

	RunAndReboot(deps, opts)

	if !rebooter.rebooted {
		t.Error("a panic in the boot sequence itself did not reboot")
	}
}

// testDeps is a minimal happy-path Deps whose /app supervision ends as soon
// as it starts, so Run returns instead of supervising forever.
func testDeps(rebooter Rebooter, clock *fakeClock, stop chan struct{}) Deps {
	var once sync.Once
	return Deps{
		Mounter:  &fakeMounter{},
		Hostname: &fakeHostname{},
		Reaper:   fakeReaper{},
		Rebooter: rebooter,
		AppStarter: funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
			once.Do(func() { close(stop) })
			return 1, nil
		}),

		// io.Discard, not a buffer: the panic guards log from their own
		// goroutines, concurrently with the boot sequence's own logging.
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{io.Discard}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       clock.Sleep,
		Now:         clock.Now,
	}
}

// notifyingRebooter signals rebooted as soon as Reboot lands, for the
// guards whose reboot happens on another goroutine.
type notifyingRebooter struct {
	fakeRebooter
	once     sync.Once
	rebooted chan struct{}
}

func (r *notifyingRebooter) Reboot() {
	r.fakeRebooter.Reboot()
	r.once.Do(func() { close(r.rebooted) })
}
