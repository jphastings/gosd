package boot

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/dataexpand"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/provsnapshot"
	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/naming"
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
	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0"}
	if !equalEnv(gotEnv, wantEnv) {
		t.Errorf("app env = %v, want %v", gotEnv, wantEnv)
	}
	if rebooter.rebooted {
		t.Error("Run() rebooted on the happy path")
	}
	if !strings.Contains(console.String(), "[gosd] hostname set to") {
		t.Errorf("console output missing expected log line: %q", console.String())
	}
	if !strings.Contains(console.String(), "boot partition mounted at /boot from /dev/mmcblk0p1") {
		t.Errorf("console output missing boot partition source device: %q", console.String())
	}
}

// TestRunLogsImageIdentityWhenPresent is the acceptance test for gosd-acdn's
// boot-time log line: config.json's Identity, when baked in, is logged as a
// short, human-scannable digest so a bench session can eyeball which build
// is running.
func TestRunLogsImageIdentityWhenPresent(t *testing.T) {
	stop := make(chan struct{})
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
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
			return initcfg.Config{Identity: "30e629b6f8caf1ff8f16ee98d8f1c5c7eb3138b9c63944e235e9678744f2094b"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       func(d time.Duration) { clock.Sleep(d) },
		Now:         clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if !strings.Contains(console.String(), "[gosd] image identity: 30e629b6f8ca") {
		t.Errorf("console output missing the image identity log line: %q", console.String())
	}
}

// TestRunDoesNotLogImageIdentityWhenAbsent covers backward compatibility
// with images built before gosd-acdn: config.json's Identity is optional,
// so an empty one is not an error and produces no misleading log line
// (e.g. "image identity: ").
func TestRunDoesNotLogImageIdentityWhenAbsent(t *testing.T) {
	stop := make(chan struct{})
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
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
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{Hostname: "my-device"}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       func(d time.Duration) { clock.Sleep(d) },
		Now:         clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (a pre-gosd-acdn config.json must still boot)", err)
	}

	if strings.Contains(console.String(), "image identity") {
		t.Errorf("console output unexpectedly logged an image identity for a config.json with none: %q", console.String())
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

func TestRunProbesOnlyTheBootdevDiskForGosdBoot(t *testing.T) {
	// The gosd-vzk2 repro: a stale GoSD image on eMMC (mmcblk0) and a fresh
	// one on SD (mmcblk1) both mount as FAT and both carry gosd.toml, so
	// device-name order alone would pick the stale eMMC. With gosd.bootdev
	// naming the booted SD disk, only its partition may ever be probed.
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
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
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) {
			return initcfg.CmdlineArgs{BootDev: "mmcblk1"}, nil
		},
		Sleep: clock.Sleep,
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	for _, call := range mounter.calls {
		if call.target == "/boot" && call.source != "/dev/mmcblk1p1" {
			t.Errorf("GOSD-BOOT probe mounted %s; gosd.bootdev=mmcblk1 must restrict probing to /dev/mmcblk1p1", call.source)
		}
	}
	if !strings.Contains(console.String(), "boot partition mounted at /boot from /dev/mmcblk1p1") {
		t.Errorf("console output missing the booted-disk mount: %q", console.String())
	}
}

func TestRunProbesAllCandidatesWhenBootdevMatchesNothing(t *testing.T) {
	// An unrecognized gosd.bootdev (a typo, or a future board naming a disk
	// gosd-init doesn't list) must degrade to the existing full walk, not
	// fail the boot.
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
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
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) {
			return initcfg.CmdlineArgs{BootDev: "nvme0n1"}, nil
		},
		Sleep: clock.Sleep,
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if !strings.Contains(console.String(), "boot partition mounted at /boot from /dev/mmcblk0p1") {
		t.Errorf("console output missing the fallback mount from the first candidate: %q", console.String())
	}
	if !strings.Contains(console.String(), "matches no boot partition candidate") {
		t.Errorf("console output missing the no-match warning: %q", console.String())
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
		ReadGosdToml: func() (gosdtoml.Config, []string, error) {
			return gosdtoml.Config{Hostname: "hand-edited-name"}, nil, nil
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
		ReadGosdToml: func() (gosdtoml.Config, []string, error) { return gosdtoml.Config{}, nil, errors.New("garbage TOML") },
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
		Mounter:     &fakeMounter{},
		Hostname:    &fakeHostname{},
		AppStarter:  appStarter,
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadGosdToml: func() (gosdtoml.Config, []string, error) {
			return gosdtoml.Config{Wifi: gosdtoml.Wifi{SSID: "hand-edited"}}, nil, nil
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
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
		ReadGosdToml: func() (gosdtoml.Config, []string, error) {
			return gosdtoml.Config{Hostname: "hand-edited-name"}, nil, nil
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

// TestRunWritesEtcHostsOnceWithTheFinalSettledHostname is the acceptance
// test for gosd-e3xi part 2: gosd-init writes /etc/hosts exactly once per
// boot, and only after gosd.toml has had its say — not with config.json's
// earlier, since-overridden value.
func TestRunWritesEtcHostsOnceWithTheFinalSettledHostname(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotHostnames []string

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
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "baked-in-name"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadGosdToml: func() (gosdtoml.Config, []string, error) {
			return gosdtoml.Config{Hostname: "hand-edited-name"}, nil, nil
		},
		WriteHosts: func(hostname string) error {
			gotHostnames = append(gotHostnames, hostname)
			return nil
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if want := []string{"hand-edited-name"}; len(gotHostnames) != len(want) || gotHostnames[0] != want[0] {
		t.Errorf("WriteHosts calls = %v, want %v (called once, with the final gosd.toml hostname)", gotHostnames, want)
	}
}

// TestRunWriteHostsFailureIsNotFatal mirrors
// TestRunSetHostnameFailureIsNotFatal: a broken /etc/hosts write is
// cosmetic (DNS still resolves whatever it was going to), not worth a
// reboot loop over.
func TestRunWriteHostsFailureIsNotFatal(t *testing.T) {
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

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
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "my-device"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		WriteHosts: func(hostname string) error {
			return errors.New("read-only filesystem")
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (a broken /etc/hosts write is not fatal)", err)
	}
	if !strings.Contains(console.String(), "writing /etc/hosts failed") {
		t.Errorf("console output missing the /etc/hosts failure log line: %q", console.String())
	}
}

// TestRunWritesEtcHostsWithProvisioningSnapshotRestoredHostname confirms
// /etc/hosts reflects a hostname restored by the first-boot-after-reflash
// self-heal (provsnapshot), which settles even later than gosd.toml/
// cloud-init: without this, a reflashed board's kernel hostname
// (sethostname(2)) and its /etc/hosts entry would disagree until the next
// reboot.
func TestRunWritesEtcHostsWithProvisioningSnapshotRestoredHostname(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotHostnames []string

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:              &fakeMounter{},
		Hostname:             &fakeHostname{},
		AppStarter:           appStarter,
		Reaper:               fakeReaper{},
		Rebooter:             &fakeRebooter{},
		OpenConsole:          func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog:          func(string, ...any) {},
		EnsureDataMountpoint: func() error { return nil },
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "baked-in-name", Identity: "new"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ProvisionSnapshot: func(in provsnapshot.Input, log func(string, ...any)) provsnapshot.Result {
			return provsnapshot.Result{
				GosdToml:         gosdtoml.Config{Hostname: "restored-from-snapshot"},
				HostnameRestored: true,
			}
		},
		WriteHosts: func(hostname string) error {
			gotHostnames = append(gotHostnames, hostname)
			return nil
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if want := []string{"restored-from-snapshot"}; len(gotHostnames) != len(want) || gotHostnames[0] != want[0] {
		t.Errorf("WriteHosts calls = %v, want %v (the snapshot-restored hostname, not the earlier baked one)", gotHostnames, want)
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

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0"}
	if !equalEnv(gotEnv, wantEnv) {
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

	wantEnv := []string{"GOSD_BOARD=", "GOSD_HOSTNAME=", "GOSD_DATA_FLUSH=0"}
	if !equalEnv(gotEnv, wantEnv) {
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

func TestRunSetHostnameFailureIsNotFatal(t *testing.T) {
	// A wrong hostname is cosmetic; a reboot loop over sethostname(2)
	// rejecting it is not (gosd-jeaw). SetHostname failing must log and
	// let boot continue — never trigger the fatal reboot path.
	mounter := &fakeMounter{}
	hostname := &fakeHostname{err: errBoom}
	rebooter := &fakeRebooter{}
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
		Rebooter:    rebooter,
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Board: "pi-zero-2w", Hostname: "my-device"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       clock.Sleep,
		Now:         clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (a SetHostname failure must not be fatal)", err)
	}
	if rebooter.rebooted {
		t.Error("Run() rebooted after a SetHostname failure")
	}
	if !strings.Contains(console.String(), "setting hostname to") || !strings.Contains(console.String(), "failed") {
		t.Errorf("console output missing the hostname failure log line: %q", console.String())
	}
}

func TestRunRejectsOverlongGosdTomlHostname(t *testing.T) {
	// A hand-edited gosd.toml hostname over naming.MaxLength bytes must
	// never reach SetHostname: it's rejected, logged, and the previous
	// (baked-in) hostname is kept for both the initial and re-applied
	// SetHostname calls (gosd-jeaw).
	tooLong := strings.Repeat("a", naming.MaxLength+2)
	mounter := &fakeMounter{}
	hostname := &fakeHostname{}
	rebooter := &fakeRebooter{}
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
		Rebooter:    rebooter,
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "baked-in-name"}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadGosdToml: func() (gosdtoml.Config, []string, error) {
			return gosdtoml.Config{Hostname: tooLong}, nil, nil
		},
		Sleep: clock.Sleep,
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (an invalid gosd.toml hostname is not fatal)", err)
	}
	if rebooter.rebooted {
		t.Error("Run() rebooted over an invalid gosd.toml hostname")
	}

	wantCalls := []string{"baked-in-name", "baked-in-name"}
	if len(hostname.set) != len(wantCalls) || hostname.set[0] != wantCalls[0] || hostname.set[1] != wantCalls[1] {
		t.Errorf("SetHostname calls = %v, want %v (invalid hostname rejected, previous kept)", hostname.set, wantCalls)
	}
	if !strings.Contains(console.String(), "invalid") || !strings.Contains(console.String(), "gosd.toml") {
		t.Errorf("console output missing the hostname rejection log line: %q", console.String())
	}
}

func TestRunRejectsInvalidCharsetCloudInitHostname(t *testing.T) {
	// Charset validation shares the same naming.Sanitize semantics as the
	// length cap, and applies to cloud-init's hostname too, not just
	// gosd.toml's.
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
			return provision.Result{Hostname: "Not A Valid Host!"}
		},
		Sleep: clock.Sleep,
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (an invalid cloud-init hostname is not fatal)", err)
	}

	wantCalls := []string{"baked-in-name", "baked-in-name"}
	if len(hostname.set) != len(wantCalls) || hostname.set[0] != wantCalls[0] || hostname.set[1] != wantCalls[1] {
		t.Errorf("SetHostname calls = %v, want %v (invalid hostname rejected, previous kept)", hostname.set, wantCalls)
	}
	if !strings.Contains(console.String(), "invalid") || !strings.Contains(console.String(), "cloud-init") {
		t.Errorf("console output missing the hostname rejection log line: %q", console.String())
	}
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

func TestRunMountsDataPartitionReadWrite(t *testing.T) {
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
	for _, e := range gotEnv {
		if strings.HasPrefix(e, "GOSD_DATA=") {
			t.Errorf("app env contains %q; GOSD_DATA is no longer exported", e)
		}
	}
	if !markerCreated {
		t.Error("the .gosd-data marker was never created after a successful data mount")
	}
	if !strings.Contains(console.String(), "data partition mounted") {
		t.Errorf("console output missing data mount log line: %q", console.String())
	}
}

func TestRunMountsReadOnlyDataWhenPartitionIsMissing(t *testing.T) {
	// An image built with --data-size=0 (or from before GOSD-DATA existed)
	// has no partition 2: boot must proceed normally and the app must start,
	// but /data is mounted read-only so a write there fails with EROFS
	// instead of silently landing in RAM. The GOSD-DATA vfat mount (which
	// reports the device node missing) fails; the tmpfs fallback succeeds.
	mounter := &fakeMounter{fn: func(c mountCall) error {
		if c.target == "/data" && c.fstype == "vfat" {
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
			t.Errorf("app env contains %q; GOSD_DATA is no longer exported", e)
		}
	}
	var readOnlyFallback bool
	for _, c := range mounter.recordedCalls("/data") {
		if c.fstype == "tmpfs" && c.flags&msRdOnly != 0 {
			readOnlyFallback = true
		}
	}
	if !readOnlyFallback {
		t.Errorf("want a read-only tmpfs mounted at /data when the partition is missing; calls: %+v", mounter.recordedCalls("/data"))
	}
	if !strings.Contains(console.String(), "no data partition on this image; mounting /data read-only") {
		t.Errorf("console output missing the read-only fallback log line: %q", console.String())
	}
}

// dataFlushTestDeps builds the Deps needed to exercise the data-flush
// override: a baked config.json DataFlush value, and, if override is
// non-nil, a gosd.toml carrying a data_flush key.
func dataFlushTestDeps(mounter *fakeMounter, console *bytes.Buffer, clock *fakeClock, stop chan struct{}, baked bool, override *bool, gotEnv *[]string) Deps {
	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		*gotEnv = env
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
		ReadConfig:           func() (initcfg.Config, error) { return initcfg.Config{DataFlush: baked}, nil },
		ReadCmdline:          func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		EnsureDataMountpoint: func() error { return nil },
		Sleep:                func(d time.Duration) { clock.Sleep(d) },
		Now:                  clock.Now,
	}
	if override != nil {
		deps.ReadGosdToml = func() (gosdtoml.Config, []string, error) {
			return gosdtoml.Config{DataFlush: override}, nil, nil
		}
	}
	return deps
}

// TestRunDataFlushDefaultsToNoFlush covers gosd-9m1k's locked default: with
// no --data-flush baked in and no gosd.toml override, /data is mounted
// without the vfat "flush" option, and the app sees GOSD_DATA_FLUSH=0.
func TestRunDataFlushDefaultsToNoFlush(t *testing.T) {
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotEnv []string

	deps := dataFlushTestDeps(mounter, console, clock, stop, false, nil, &gotEnv)
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	calls := mounter.recordedCalls("/data")
	if len(calls) == 0 || calls[len(calls)-1].data != "" {
		t.Errorf("/data mount options = %+v, want no flush option by default", calls)
	}
	if !slices.Contains(gotEnv, "GOSD_DATA_FLUSH=0") {
		t.Errorf("app env = %v, want GOSD_DATA_FLUSH=0", gotEnv)
	}
	if strings.Contains(console.String(), "data partition flush:") {
		t.Errorf("console output logged a data-flush line though nothing overrode the baked default: %q", console.String())
	}
}

// TestRunDataFlushBakedTrueAppliesToMountAndEnv covers a build made with
// --data-flush: the baked default alone, with no gosd.toml override, must
// reach both the /data mount and GOSD_DATA_FLUSH.
func TestRunDataFlushBakedTrueAppliesToMountAndEnv(t *testing.T) {
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotEnv []string

	deps := dataFlushTestDeps(mounter, console, clock, stop, true, nil, &gotEnv)
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	calls := mounter.recordedCalls("/data")
	if len(calls) == 0 || calls[len(calls)-1].data != "flush" {
		t.Errorf("/data mount options = %+v, want the flush option (config.json's baked --data-flush)", calls)
	}
	if !slices.Contains(gotEnv, "GOSD_DATA_FLUSH=1") {
		t.Errorf("app env = %v, want GOSD_DATA_FLUSH=1", gotEnv)
	}
}

// TestRunGosdTomlDataFlushOverridesBakedDefault covers the card-editable
// override turning flush ON despite a baked default of false, and reaching
// both the /data mount and the app's environment.
func TestRunGosdTomlDataFlushOverridesBakedDefault(t *testing.T) {
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotEnv []string
	override := true

	deps := dataFlushTestDeps(mounter, console, clock, stop, false, &override, &gotEnv)
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	calls := mounter.recordedCalls("/data")
	if len(calls) == 0 || calls[len(calls)-1].data != "flush" {
		t.Errorf("/data mount options = %+v, want the flush option (gosd.toml override)", calls)
	}
	if !slices.Contains(gotEnv, "GOSD_DATA_FLUSH=1") {
		t.Errorf("app env = %v, want GOSD_DATA_FLUSH=1", gotEnv)
	}
	if !strings.Contains(console.String(), "data partition flush: true (gosd.toml)") {
		t.Errorf("console output missing the data-flush override log line: %q", console.String())
	}
}

// TestRunGosdTomlDataFlushOverridesBakedTrueToDisable is the reverse of the
// above: gosd.toml can turn flush back OFF even when --data-flush baked it
// in.
func TestRunGosdTomlDataFlushOverridesBakedTrueToDisable(t *testing.T) {
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotEnv []string
	override := false

	deps := dataFlushTestDeps(mounter, console, clock, stop, true, &override, &gotEnv)
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	calls := mounter.recordedCalls("/data")
	if len(calls) == 0 || calls[len(calls)-1].data != "" {
		t.Errorf("/data mount options = %+v, want no flush option (gosd.toml disabled it)", calls)
	}
	if !slices.Contains(gotEnv, "GOSD_DATA_FLUSH=0") {
		t.Errorf("app env = %v, want GOSD_DATA_FLUSH=0", gotEnv)
	}
	if !strings.Contains(console.String(), "data partition flush: false (gosd.toml)") {
		t.Errorf("console output missing the data-flush override log line: %q", console.String())
	}
}

// envDeps builds the minimal Deps needed to exercise app-env merging: a
// baked config.json (with the given Env), and, if gosdToml is non-nil, a
// gosd.toml with the given Env/warnings. gotEnv is populated with whatever
// env slice the AppStarter receives.
func envDeps(bakedEnv map[string]string, gosdToml *gosdtoml.Config, warnings []string, console *bytes.Buffer, gotEnv *[]string) (Deps, chan struct{}) {
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		*gotEnv = env
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
			return initcfg.Config{Board: "pi-zero-2w", Hostname: "my-device", Env: bakedEnv}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       func(d time.Duration) { clock.Sleep(d) },
		Now:         clock.Now,
	}
	if gosdToml != nil {
		deps.ReadGosdToml = func() (gosdtoml.Config, []string, error) { return *gosdToml, warnings, nil }
	}
	return deps, stop
}

func TestRunInjectsBakedEnvWhenNoGosdTomlOverride(t *testing.T) {
	console := &bytes.Buffer{}
	var gotEnv []string
	deps, stop := envDeps(map[string]string{"FOO": "baked-foo"}, nil, nil, console, &gotEnv)
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0", "FOO=baked-foo"}
	if !equalEnv(gotEnv, wantEnv) {
		t.Errorf("app env = %v, want %v", gotEnv, wantEnv)
	}
	if !strings.Contains(console.String(), "app env: FOO (baked)") {
		t.Errorf("console output missing baked env summary line: %q", console.String())
	}
}

func TestRunInjectsGosdTomlEnvWhenNoBakedDefaults(t *testing.T) {
	console := &bytes.Buffer{}
	var gotEnv []string
	gosdToml := &gosdtoml.Config{Env: map[string]string{"FOO": "card-foo"}}
	deps, stop := envDeps(nil, gosdToml, nil, console, &gotEnv)
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0", "FOO=card-foo"}
	if !equalEnv(gotEnv, wantEnv) {
		t.Errorf("app env = %v, want %v", gotEnv, wantEnv)
	}
	if !strings.Contains(console.String(), "app env: FOO (gosd.toml)") {
		t.Errorf("console output missing gosd.toml env summary line: %q", console.String())
	}
}

func TestRunGosdTomlEnvOverridesBakedPerKey(t *testing.T) {
	// FOO only baked, BAR overridden by gosd.toml, BAZ only in gosd.toml:
	// the merge is per-key, not a whole-map replace.
	console := &bytes.Buffer{}
	var gotEnv []string
	baked := map[string]string{"FOO": "baked-foo", "BAR": "baked-bar"}
	gosdToml := &gosdtoml.Config{Env: map[string]string{"BAR": "card-bar", "BAZ": "card-baz"}}
	deps, stop := envDeps(baked, gosdToml, nil, console, &gotEnv)
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0", "BAR=card-bar", "BAZ=card-baz", "FOO=baked-foo"}
	if !equalEnv(gotEnv, wantEnv) {
		t.Errorf("app env = %v, want %v (gosd.toml wins per-key over baked)", gotEnv, wantEnv)
	}
}

func TestRunRejectsReservedEnvKeysFromGosdToml(t *testing.T) {
	// A card can't override the GOSD_* namespace gosd-init itself owns,
	// nor smuggle in an unrelated GOSD_-prefixed var; both are dropped,
	// and the real GOSD_BOARD/GOSD_HOSTNAME stay exactly as gosd-init set
	// them.
	console := &bytes.Buffer{}
	var gotEnv []string
	gosdToml := &gosdtoml.Config{Env: map[string]string{
		"GOSD_BOARD": "attacker-board",
		"GOSD_X":     "should-be-dropped",
		"SAFE":       "card-safe",
	}}
	deps, stop := envDeps(nil, gosdToml, nil, console, &gotEnv)
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0", "SAFE=card-safe"}
	if !equalEnv(gotEnv, wantEnv) {
		t.Errorf("app env = %v, want %v (reserved keys dropped, real GOSD_* intact)", gotEnv, wantEnv)
	}
	if !strings.Contains(console.String(), "ignoring reserved env key GOSD_BOARD") {
		t.Errorf("console output missing GOSD_BOARD rejection log line: %q", console.String())
	}
	if !strings.Contains(console.String(), "ignoring reserved env key GOSD_X") {
		t.Errorf("console output missing GOSD_X rejection log line: %q", console.String())
	}
}

func TestRunLogsGosdTomlParseWarnings(t *testing.T) {
	console := &bytes.Buffer{}
	var gotEnv []string
	gosdToml := &gosdtoml.Config{Env: map[string]string{"PORT": "8080"}}
	warnings := []string{`gosd.toml [env] PORT is a bare number, not a quoted string; using "8080" — add quotes to silence this warning`}
	deps, stop := envDeps(nil, gosdToml, warnings, console, &gotEnv)
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if !strings.Contains(console.String(), "PORT is a bare number") {
		t.Errorf("console output missing the gosd.toml [env] coercion warning: %q", console.String())
	}
}

func TestRunAppEnvIsUnchangedWhenNoUserEnvIsSet(t *testing.T) {
	console := &bytes.Buffer{}
	var gotEnv []string
	deps, stop := envDeps(nil, nil, nil, console, &gotEnv)
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0"}
	if !equalEnv(gotEnv, wantEnv) {
		t.Errorf("app env = %v, want %v (no user env vars set anywhere)", gotEnv, wantEnv)
	}
	if strings.Contains(console.String(), "app env:") {
		t.Errorf("console output has an app env summary line when there's nothing to report: %q", console.String())
	}
}

// equalEnv compares two env slices exactly, in order: mergeUserEnv's output
// is fully deterministic (GOSD_* vars in the fixed order Run builds them,
// then the merged user env sorted by key), so tests can assert on it
// positionally rather than just as a set.
func equalEnv(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
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

// expandTestDeps builds the Deps for the --data-size=expand sequence tests:
// a config whose dataExpand flag is set (or not), and an ExpandData hook
// whose invocations are recorded through the returned pointers.
func expandTestDeps(mounter *fakeMounter, clock *fakeClock, stop chan struct{}, dataExpand bool, expandErr error, expandedWith *[]string) Deps {
	return Deps{
		Mounter:  mounter,
		Hostname: &fakeHostname{},
		AppStarter: funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
			close(stop)
			return 1, nil
		}),
		Reaper:               fakeReaper{},
		Rebooter:             &fakeRebooter{},
		OpenConsole:          func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog:          func(string, ...any) {},
		ReadConfig:           func() (initcfg.Config, error) { return initcfg.Config{DataExpand: dataExpand}, nil },
		ReadCmdline:          func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		EnsureDataMountpoint: func() error { return nil },
		ExpandData: func(bootDevice string, log func(format string, args ...any)) error {
			*expandedWith = append(*expandedWith, bootDevice)
			return expandErr
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
}

func TestRunExpandsDataFromTheBootDeviceBeforeMountingIt(t *testing.T) {
	mounter := &fakeMounter{}
	stop := make(chan struct{})
	var expandedWith []string
	var dataMountsAtExpand int

	deps := expandTestDeps(mounter, newFakeClock(time.Unix(0, 0)), stop, true, nil, &expandedWith)
	expand := deps.ExpandData
	deps.ExpandData = func(bootDevice string, log func(format string, args ...any)) error {
		dataMountsAtExpand = mounter.callsFor("/data")
		return expand(bootDevice, log)
	}
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	// The device passed is the one GOSD-BOOT actually mounted from, so only
	// the disk the system truly booted from can ever be expanded.
	if len(expandedWith) != 1 || expandedWith[0] != "/dev/mmcblk0p1" {
		t.Errorf("ExpandData called with %v, want exactly [/dev/mmcblk0p1]", expandedWith)
	}
	if dataMountsAtExpand != 0 {
		t.Error("the data partition was mounted before ExpandData had run")
	}
	if mounter.callsFor("/data") == 0 {
		t.Error("data partition was never mounted after expansion")
	}
}

func TestRunSkipsExpansionWithoutTheConfigFlag(t *testing.T) {
	mounter := &fakeMounter{}
	stop := make(chan struct{})
	var expandedWith []string

	deps := expandTestDeps(mounter, newFakeClock(time.Unix(0, 0)), stop, false, nil, &expandedWith)
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if len(expandedWith) != 0 {
		t.Errorf("ExpandData called with %v though config.json has no dataExpand", expandedWith)
	}
}

func TestRunContinuesToTheDataFallbackWhenExpansionFails(t *testing.T) {
	// Expansion failing (no room misreported, node never appearing, a bad
	// card) must behave exactly like any other missing data partition: boot
	// proceeds, and /data gets the read-only placeholder.
	mounter := &fakeMounter{fn: func(c mountCall) error {
		if c.target == "/data" && c.fstype == "vfat" {
			return fs.ErrNotExist
		}
		return nil
	}}
	stop := make(chan struct{})
	var expandedWith []string

	deps := expandTestDeps(mounter, newFakeClock(time.Unix(0, 0)), stop, true, errBoom, &expandedWith)
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (a failed expansion is never fatal)", err)
	}
	if len(expandedWith) != 1 {
		t.Errorf("ExpandData called %d times, want once", len(expandedWith))
	}
	calls := mounter.recordedCalls("/data")
	if len(calls) == 0 || calls[len(calls)-1].fstype != "tmpfs" || calls[len(calls)-1].flags&msRdOnly == 0 {
		t.Errorf("final /data mount = %+v, want the read-only tmpfs fallback", calls)
	}
}

func TestRunHaltsAndRecordsWhenTheDataPartitionIsCorrupt(t *testing.T) {
	// An established expand partition whose filesystem is gone may still
	// hold recoverable app data: the device must record what happened to
	// boot-failure.log and halt — not reboot-loop, not reformat, and above
	// all not start the app against a read-only fallback as if nothing were
	// wrong.
	mounter := &fakeMounter{}
	rebooter := &fakeRebooter{}
	stop := make(chan struct{})
	var expandedWith []string
	var recorded string

	deps := expandTestDeps(mounter, newFakeClock(time.Unix(0, 0)), stop, true,
		fmt.Errorf("%w: /dev/mmcblk0p2 holds nothing (blank space)", dataexpand.ErrDataCorrupt), &expandedWith)
	deps.Rebooter = rebooter
	deps.AppStarter = funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		t.Error("the app was started despite a corrupt data partition")
		close(stop)
		return 1, nil
	})
	deps.WriteBootFailure = func(msg string) error { recorded = msg; return nil }
	opts := testDataOptions()
	opts.Stop = stop

	err := Run(deps, opts)
	if err == nil || !errors.Is(err, dataexpand.ErrDataCorrupt) {
		t.Fatalf("Run() = %v, want the corruption error", err)
	}
	if !rebooter.halted {
		t.Error("the device was not halted")
	}
	if rebooter.rebooted {
		t.Error("the device rebooted; corruption must halt, not reboot-loop")
	}
	if rebooter.syncCalls == 0 {
		t.Error("no sync before halting")
	}
	for _, want := range []string{"/dev/mmcblk0p2", "salvage", "GOSD-DATA"} {
		if !strings.Contains(recorded, want) {
			t.Errorf("boot-failure.log content %q is missing %q", recorded, want)
		}
	}
	if mounter.callsFor("/data") != 0 {
		t.Error("/data was mounted despite the halt")
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

// TestRunAppliesProvisioningRestoredFromTheSnapshot covers the boot
// sequence's half of the reflash self-heal (bean gosd-ry3b): whatever
// provsnapshot restores has to reach the running device on this boot, not
// just the card, so the hostname is re-applied and the app's environment is
// built from the restored gosd.toml.
func TestRunAppliesProvisioningRestoredFromTheSnapshot(t *testing.T) {
	hostname := &fakeHostname{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotEnv []string
	var dataMountedFirst bool

	mounter := &fakeMounter{}
	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		gotEnv = env
		close(stop)
		return 1, nil
	})

	var gotInput provsnapshot.Input
	deps := Deps{
		Mounter:              mounter,
		Hostname:             hostname,
		AppStarter:           appStarter,
		Reaper:               fakeReaper{},
		Rebooter:             &fakeRebooter{},
		OpenConsole:          func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog:          func(string, ...any) {},
		EnsureDataMountpoint: func() error { return nil },
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{
				Board:    "qemu-virt",
				Hostname: "hello",
				Identity: "new",
				Env:      map[string]string{"API_URL": "https://example.com"},
			}, nil
		},
		ReadCmdline:  func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadGosdToml: func() (gosdtoml.Config, []string, error) { return gosdtoml.Config{Hostname: "hello"}, nil, nil },
		ProvisionSnapshot: func(in provsnapshot.Input, log func(string, ...any)) provsnapshot.Result {
			gotInput = in
			dataMountedFirst = mounter.callsFor("/data") > 0
			return provsnapshot.Result{
				GosdToml:         gosdtoml.Config{Hostname: "kitchen-pi", Env: map[string]string{"API_URL": "https://mine"}},
				HostnameRestored: true,
			}
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if !dataMountedFirst {
		t.Error("the snapshot ran before the data partition it lives on was mounted")
	}
	if gotInput.Baked.Hostname != "hello" || gotInput.Baked.Env["API_URL"] != "https://example.com" {
		t.Errorf("snapshot input baked defaults = %+v, want config.json's, captured before any override", gotInput.Baked)
	}
	if last := hostname.set[len(hostname.set)-1]; last != "kitchen-pi" {
		t.Errorf("final SetHostname = %q, want the restored hostname applied to this boot", last)
	}
	for _, want := range []string{"API_URL=https://mine", "GOSD_HOSTNAME=kitchen-pi"} {
		if !slices.Contains(gotEnv, want) {
			t.Errorf("app env = %v, missing %q", gotEnv, want)
		}
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
