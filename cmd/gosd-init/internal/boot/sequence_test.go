package boot

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/initcfg"
)

func TestRunHappyPathOrchestratesTheBootSequence(t *testing.T) {
	mounter := &fakeMounter{}
	hostname := &fakeHostname{}
	rebooter := &fakeRebooter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration
	stop := make(chan struct{})

	starts := 0
	var gotEnv []string
	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		starts++
		gotEnv = env
		if starts == 2 {
			close(stop)
		}
		return starts, nil
	})

	deps := Deps{
		Mounter:     mounter,
		Hostname:    hostname,
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    rebooter,
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Board: "pi-zero-2w", Hostname: "my-device"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       func(d time.Duration) { sleeps = append(sleeps, d); clock.Sleep(d) },
		Now:         clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if starts != 2 {
		t.Fatalf("app started %d times, want 2", starts)
	}
	if hostname.set == nil || hostname.set[0] != "my-device" {
		t.Errorf("SetHostname calls = %v, want [\"my-device\"]", hostname.set)
	}
	if mounter.callsFor("/boot") == 0 {
		t.Error("boot partition was never mounted")
	}
	for _, target := range []string{"/dev", "/proc", "/sys", "/run"} {
		if mounter.callsFor(target) == 0 {
			t.Errorf("early mount of %s never happened", target)
		}
	}
	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device"}
	if len(gotEnv) != len(wantEnv) || gotEnv[0] != wantEnv[0] || gotEnv[1] != wantEnv[1] {
		t.Errorf("app env = %v, want %v", gotEnv, wantEnv)
	}
	if rebooter.rebooted {
		t.Error("Run() rebooted on the happy path")
	}
	if !strings.Contains(console.String(), "[gosd] hostname set to") {
		t.Errorf("console output missing expected log line: %q", console.String())
	}
}

func TestRunStartsNetworkingWithoutBlockingAppStart(t *testing.T) {
	// StartNetworking must never delay /app's launch: Run should
	// dispatch it and move straight on to supervision.
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration
	stop := make(chan struct{})
	networkingStarted := make(chan struct{})

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:     &fakeMounter{},
		Hostname:    &fakeHostname{},
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       func(d time.Duration) { sleeps = append(sleeps, d); clock.Sleep(d) },
		Now:         clock.Now,
		StartNetworking: func(log func(string, ...any)) {
			close(networkingStarted)
		},
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	select {
	case <-networkingStarted:
	case <-time.After(time.Second):
		t.Error("StartNetworking was never called")
	}
}

func TestRunReadsCmdlineOnlyAfterProcIsMounted(t *testing.T) {
	// Regression test: gosd.board / gosd.debug come from /proc/cmdline,
	// which isn't readable until step 1 (mountEarly) has mounted /proc.
	// Reading it any earlier would silently and permanently disable both
	// overrides on real hardware.
	mounter := &fakeMounter{}
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration
	stop := make(chan struct{})
	cmdlineReadAfterProcMount := false

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:     mounter,
		Hostname:    &fakeHostname{},
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) {
			cmdlineReadAfterProcMount = mounter.callsFor("/proc") > 0
			return initcfg.CmdlineArgs{}, nil
		},
		Sleep: func(d time.Duration) { sleeps = append(sleeps, d); clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if !cmdlineReadAfterProcMount {
		t.Error("ReadCmdline was called before /proc was mounted")
	}
}

func TestRunAppliesCmdlineBoardOverrideAndLogsDebug(t *testing.T) {
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration
	stop := make(chan struct{})
	var gotEnv []string

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		gotEnv = env
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:     &fakeMounter{},
		Hostname:    &fakeHostname{},
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Board: "radxa-zero-3e", Hostname: "my-device"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) {
			return initcfg.CmdlineArgs{Board: "pi-zero-2w", Debug: true}, nil
		},
		Sleep: func(d time.Duration) { sleeps = append(sleeps, d); clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device"}
	if len(gotEnv) != len(wantEnv) || gotEnv[0] != wantEnv[0] || gotEnv[1] != wantEnv[1] {
		t.Errorf("app env = %v, want %v (cmdline gosd.board should override config.json)", gotEnv, wantEnv)
	}
	if !strings.Contains(console.String(), "debug mode enabled") {
		t.Errorf("console output missing debug-mode log line: %q", console.String())
	}
}

func TestRunFallsBackToDefaultsWhenConfigAndCmdlineFail(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration
	stop := make(chan struct{})
	var gotEnv []string

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		gotEnv = env
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:     &fakeMounter{},
		Hostname:    &fakeHostname{},
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, errors.New("no such file") },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, errors.New("no such file") },
		Sleep:       func(d time.Duration) { sleeps = append(sleeps, d); clock.Sleep(d) },
		Now:         clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (a missing config/cmdline is not fatal)", err)
	}

	wantEnv := []string{"GOSD_BOARD=", "GOSD_HOSTNAME="}
	if len(gotEnv) != len(wantEnv) || gotEnv[0] != wantEnv[0] || gotEnv[1] != wantEnv[1] {
		t.Errorf("app env = %v, want %v (zero-value defaults)", gotEnv, wantEnv)
	}
}

func TestRunFatalPathOnEarlyMountFailure(t *testing.T) {
	mounter := &fakeMounter{fn: func(mountCall) error { return errBoom }}
	rebooter := &fakeRebooter{}
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration

	deps := testDepsForFatalPath(mounter, &fakeHostname{}, rebooter, clock, &sleeps)
	opts := testOptions()

	err := Run(deps, opts)
	if err == nil || !strings.Contains(err.Error(), "mounting early filesystems") {
		t.Fatalf("Run() = %v, want an error about mounting early filesystems", err)
	}
	assertFatalPathTriggered(t, rebooter, sleeps)
}

func TestRunFatalPathOnHostnameFailure(t *testing.T) {
	mounter := &fakeMounter{}
	hostname := &fakeHostname{err: errBoom}
	rebooter := &fakeRebooter{}
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration

	deps := testDepsForFatalPath(mounter, hostname, rebooter, clock, &sleeps)
	opts := testOptions()

	err := Run(deps, opts)
	if err == nil || !strings.Contains(err.Error(), "setting hostname") {
		t.Fatalf("Run() = %v, want an error about setting hostname", err)
	}
	assertFatalPathTriggered(t, rebooter, sleeps)
}

func TestRunFatalPathOnBootPartitionMountTimeout(t *testing.T) {
	mounter := &fakeMounter{fn: func(c mountCall) error {
		if c.target == "/boot" {
			return errBoom
		}
		return nil
	}}
	rebooter := &fakeRebooter{}
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration

	deps := testDepsForFatalPath(mounter, &fakeHostname{}, rebooter, clock, &sleeps)
	opts := testOptions()

	err := Run(deps, opts)
	if err == nil || !strings.Contains(err.Error(), "mounting boot partition") {
		t.Fatalf("Run() = %v, want an error about mounting the boot partition", err)
	}
	assertFatalPathTriggered(t, rebooter, sleeps)
}

func testDepsForFatalPath(mounter Mounter, hostname HostnameSetter, rebooter Rebooter, clock *fakeClock, sleeps *[]time.Duration) Deps {
	return Deps{
		Mounter:     mounter,
		Hostname:    hostname,
		AppStarter:  funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) { return 0, nil }),
		Reaper:      fakeReaper{},
		Rebooter:    rebooter,
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Board: "pi-zero-2w", Hostname: "my-device"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       func(d time.Duration) { *sleeps = append(*sleeps, d); clock.Sleep(d) },
		Now:         clock.Now,
	}
}

func testOptions() Options {
	return Options{
		AppPath:     "/app",
		BootTarget:  "/boot",
		BootDevices: []string{"/dev/mmcblk0p1", "/dev/mmcblk1p1"},
		BootTimeout: 10 * time.Second,
	}
}

func assertFatalPathTriggered(t *testing.T, rebooter *fakeRebooter, sleeps []time.Duration) {
	t.Helper()
	if rebooter.syncCalls == 0 {
		t.Error("fatal path did not sync before rebooting")
	}
	if !rebooter.rebooted {
		t.Error("fatal path did not reboot")
	}
	found := false
	for _, s := range sleeps {
		if s == 5*time.Second {
			found = true
		}
	}
	if !found {
		t.Errorf("fatal path did not sleep 5s before rebooting (sleeps=%v)", sleeps)
	}
}
