package boot

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/provision"
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
		StartNetworking: func(cfg initcfg.Config, gosdToml gosdtoml.Config, provisionWifi []provision.WifiNetwork, log func(string, ...any)) {
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

func TestRunReappliesHostnameFromGosdTomlAfterBootMount(t *testing.T) {
	// gosd.toml's hostname must win over config.json's, and take effect
	// via a second SetHostname call, since gosd.toml can only be read
	// after the boot partition is mounted (step 5) — after step 4 already
	// applied config.json's hostname.
	mounter := &fakeMounter{}
	hostname := &fakeHostname{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:     mounter,
		Hostname:    hostname,
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "baked-in-name"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadGosdToml: func() (gosdtoml.Config, error) {
			return gosdtoml.Config{Hostname: "hand-edited-name"}, nil
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantCalls := []string{"baked-in-name", "hand-edited-name"}
	if len(hostname.set) != len(wantCalls) || hostname.set[0] != wantCalls[0] || hostname.set[1] != wantCalls[1] {
		t.Errorf("SetHostname calls = %v, want %v", hostname.set, wantCalls)
	}
	if !strings.Contains(console.String(), "gosd.toml applied") {
		t.Errorf("console output missing gosd.toml re-apply log line: %q", console.String())
	}
}

func TestRunFallsBackToConfigJSONWhenGosdTomlFailsToParse(t *testing.T) {
	// A hand-editing typo in gosd.toml must never crash boot: Run logs a
	// warning and keeps config.json's hostname.
	console := &bytes.Buffer{}
	hostname := &fakeHostname{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:     &fakeMounter{},
		Hostname:    hostname,
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "baked-in-name"}, nil
		},
		ReadCmdline:  func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadGosdToml: func() (gosdtoml.Config, error) { return gosdtoml.Config{}, errors.New("garbage TOML") },
		Sleep:        func(d time.Duration) { clock.Sleep(d) },
		Now:          clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (a broken gosd.toml is not fatal)", err)
	}

	wantCalls := []string{"baked-in-name", "baked-in-name"}
	if len(hostname.set) != len(wantCalls) || hostname.set[0] != wantCalls[0] || hostname.set[1] != wantCalls[1] {
		t.Errorf("SetHostname calls = %v, want %v (falls back to config.json both times)", hostname.set, wantCalls)
	}
	if !strings.Contains(console.String(), "reading gosd.toml failed") {
		t.Errorf("console output missing gosd.toml warning log line: %q", console.String())
	}
}

func TestRunPassesGosdTomlToStartNetworking(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	gosdTomlReceived := make(chan gosdtoml.Config, 1)

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:      &fakeMounter{},
		Hostname:     &fakeHostname{},
		AppStarter:   appStarter,
		Reaper:       fakeReaper{},
		Rebooter:     &fakeRebooter{},
		OpenConsole:  func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog:  func(string, ...any) {},
		ReadConfig:   func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline:  func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadGosdToml: func() (gosdtoml.Config, error) { return gosdtoml.Config{Wifi: gosdtoml.Wifi{SSID: "hand-edited"}}, nil },
		Sleep:        func(d time.Duration) { clock.Sleep(d) },
		Now:          clock.Now,
		StartNetworking: func(cfg initcfg.Config, gosdToml gosdtoml.Config, provisionWifi []provision.WifiNetwork, log func(string, ...any)) {
			gosdTomlReceived <- gosdToml
		},
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	select {
	case gotGosdToml := <-gosdTomlReceived:
		if gotGosdToml.Wifi.SSID != "hand-edited" {
			t.Errorf("StartNetworking got gosdToml.Wifi.SSID = %q, want %q", gotGosdToml.Wifi.SSID, "hand-edited")
		}
	case <-time.After(time.Second):
		t.Error("StartNetworking was never called")
	}
}

func TestRunAppliesCloudInitHostnameWhenGosdTomlHasNone(t *testing.T) {
	// Precedence: gosd.toml > cloud-init > config.json. With no gosd.toml
	// hostname (here: no gosd.toml at all), cloud-init's user-data must
	// still win over the baked-in config.json value.
	hostname := &fakeHostname{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:     &fakeMounter{},
		Hostname:    hostname,
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "baked-in-name"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadProvisioning: func(log func(string, ...any)) provision.Result {
			return provision.Result{Hostname: "cloud-init-name"}
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantCalls := []string{"baked-in-name", "cloud-init-name"}
	if len(hostname.set) != len(wantCalls) || hostname.set[0] != wantCalls[0] || hostname.set[1] != wantCalls[1] {
		t.Errorf("SetHostname calls = %v, want %v", hostname.set, wantCalls)
	}
	if !strings.Contains(console.String(), "hostname from cloud-init user-data") {
		t.Errorf("console output missing cloud-init hostname source log line: %q", console.String())
	}
}

func TestRunGosdTomlHostnameTakesPrecedenceOverCloudInit(t *testing.T) {
	hostname := &fakeHostname{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:     &fakeMounter{},
		Hostname:    hostname,
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "baked-in-name"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadProvisioning: func(log func(string, ...any)) provision.Result {
			return provision.Result{Hostname: "cloud-init-name"}
		},
		ReadGosdToml: func() (gosdtoml.Config, error) {
			return gosdtoml.Config{Hostname: "hand-edited-name"}, nil
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantCalls := []string{"baked-in-name", "hand-edited-name"}
	if len(hostname.set) != len(wantCalls) || hostname.set[0] != wantCalls[0] || hostname.set[1] != wantCalls[1] {
		t.Errorf("SetHostname calls = %v, want %v (gosd.toml wins over cloud-init)", hostname.set, wantCalls)
	}
}

func TestRunPassesCloudInitWifiToStartNetworking(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	wifiReceived := make(chan []provision.WifiNetwork, 1)

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
		ReadProvisioning: func(log func(string, ...any)) provision.Result {
			return provision.Result{Wifi: []provision.WifiNetwork{{SSID: "cloud-init-ssid"}}}
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
		StartNetworking: func(cfg initcfg.Config, gosdToml gosdtoml.Config, provisionWifi []provision.WifiNetwork, log func(string, ...any)) {
			wifiReceived <- provisionWifi
		},
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	select {
	case got := <-wifiReceived:
		if len(got) != 1 || got[0].SSID != "cloud-init-ssid" {
			t.Errorf("StartNetworking got provisionWifi = %+v, want one network %q", got, "cloud-init-ssid")
		}
	case <-time.After(time.Second):
		t.Error("StartNetworking was never called")
	}
}

func TestRunLogsFirstrunShDetectionButDoesNotUseIt(t *testing.T) {
	hostname := &fakeHostname{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:     &fakeMounter{},
		Hostname:    hostname,
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "baked-in-name"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadProvisioning: func(log func(string, ...any)) provision.Result {
			log("firstrun.sh found on the boot partition; gosd-init never parses or executes it — use gosd.toml to configure this device instead")
			return provision.Result{FirstrunPresent: true}
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if hostname.set[len(hostname.set)-1] != "baked-in-name" {
		t.Errorf("SetHostname calls = %v, want the last call to still be config.json's value (firstrun.sh is never parsed)", hostname.set)
	}
	if !strings.Contains(console.String(), "firstrun.sh found") {
		t.Errorf("console output missing the firstrun.sh detection log line: %q", console.String())
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

// testDataOptions returns Options with the data partition configured, for
// tests exercising the /data mount.
func testDataOptions() Options {
	opts := testOptions()
	opts.DataTarget = "/data"
	opts.DataDevices = []string{"/dev/mmcblk0p2", "/dev/mmcblk1p2"}
	opts.DataTimeout = 10 * time.Second
	return opts
}

func TestRunMountsDataPartitionAndExportsGosdData(t *testing.T) {
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	markerCreated := false
	var gotEnv []string

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		gotEnv = env
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:              mounter,
		Hostname:             &fakeHostname{},
		AppStarter:           appStarter,
		Reaper:               fakeReaper{},
		Rebooter:             &fakeRebooter{},
		OpenConsole:          func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog:          func(string, ...any) {},
		ReadConfig:           func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline:          func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		EnsureDataMountpoint: func() error { return nil },
		EnsureDataMarker:     func() error { markerCreated = true; return nil },
		Sleep:                func(d time.Duration) { clock.Sleep(d) },
		Now:                  clock.Now,
	}
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if mounter.callsFor("/data") == 0 {
		t.Error("data partition was never mounted")
	}
	found := false
	for _, e := range gotEnv {
		if e == "GOSD_DATA=/data" {
			found = true
		}
	}
	if !found {
		t.Errorf("app env = %v, want it to contain GOSD_DATA=/data", gotEnv)
	}
	if !markerCreated {
		t.Error("the .gosd-data marker was never created after a successful data mount")
	}
	if !strings.Contains(console.String(), "data partition mounted") {
		t.Errorf("console output missing data mount log line: %q", console.String())
	}
}

func TestRunContinuesWithoutGosdDataWhenPartitionIsMissing(t *testing.T) {
	// An image built with --data-size=0 (or from before GOSD-DATA existed)
	// has no partition 2: boot must proceed normally, the app must start,
	// and it simply gets no GOSD_DATA.
	mounter := &fakeMounter{fn: func(c mountCall) error {
		if c.target == "/data" {
			return fs.ErrNotExist
		}
		return nil
	}}
	rebooter := &fakeRebooter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotEnv []string

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		gotEnv = env
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:              mounter,
		Hostname:             &fakeHostname{},
		AppStarter:           appStarter,
		Reaper:               fakeReaper{},
		Rebooter:             rebooter,
		OpenConsole:          func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog:          func(string, ...any) {},
		ReadConfig:           func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline:          func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		EnsureDataMountpoint: func() error { return nil },
		EnsureDataMarker:     func() error { t.Error("EnsureDataMarker called though the data partition never mounted"); return nil },
		Sleep:                func(d time.Duration) { clock.Sleep(d) },
		Now:                  clock.Now,
	}
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (a missing data partition is not fatal)", err)
	}

	if rebooter.rebooted {
		t.Error("Run() rebooted over a missing data partition")
	}
	for _, e := range gotEnv {
		if strings.HasPrefix(e, "GOSD_DATA=") {
			t.Errorf("app env contains %q; want no GOSD_DATA when the partition is missing", e)
		}
	}
	if !strings.Contains(console.String(), "no data partition") {
		t.Errorf("console output missing the no-data-partition log line: %q", console.String())
	}
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
