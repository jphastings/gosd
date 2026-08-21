package boot

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/cardconfig"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/dataexpand"
	"github.com/jphastings/gosd/internal/configtree"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/faultreport"
	"github.com/jphastings/gosd/internal/hostsfile"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/naming"
	"github.com/jphastings/gosd/internal/provision"
	"github.com/jphastings/gosd/internal/redact"
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
		StartNetworking: func(cfg initcfg.Config, config cardconfig.Tree, log func(string, ...any)) {
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
	// one on SD (mmcblk1) both mount as FAT and both carry a config tree, so
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
			t.Errorf("boot-partition probe mounted %s; gosd.bootdev=mmcblk1 must restrict probing to /dev/mmcblk1p1", call.source)
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

func TestRunReappliesHostnameFromTheCardAfterBootMount(t *testing.T) {
	// A hostname on the card must win over config.json's, and take effect
	// via a second SetHostname call, since the config tree can only be read
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
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(map[string]string{"hostname": "hand-edited-name"}),
		Sleep:          func(d time.Duration) { clock.Sleep(d) },
		Now:            clock.Now,
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
	if !strings.Contains(console.String(), "config/hostname applied") {
		t.Errorf("console output missing the config/hostname re-apply log line: %q", console.String())
	}
}
func TestRunKeepsTheBakedHostnameWhenTheCardNamesNone(t *testing.T) {
	// An empty hostname file is the ordinary state of a card nobody has
	// edited: the name the image was built with stands, and nothing is
	// re-applied over it.
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
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(map[string]string{"hostname": ""}),
		Sleep:          func(d time.Duration) { clock.Sleep(d) },
		Now:            clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if want := []string{"baked-in-name"}; !slices.Equal(hostname.set, want) {
		t.Errorf("SetHostname calls = %v, want %v (config.json's name, applied once)", hostname.set, want)
	}
}
func TestRunPassesTheCardsSettingsToStartNetworking(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	configReceived := make(chan cardconfig.Tree, 1)

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:        &fakeMounter{},
		Hostname:       &fakeHostname{},
		AppStarter:     appStarter,
		Reaper:         fakeReaper{},
		Rebooter:       &fakeRebooter{},
		OpenConsole:    func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog:    func(string, ...any) {},
		ReadConfig:     func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(map[string]string{"wifi/ssid": "hand-edited"}),
		Sleep:          func(d time.Duration) { clock.Sleep(d) },
		Now:            clock.Now,
		StartNetworking: func(cfg initcfg.Config, config cardconfig.Tree, log func(string, ...any)) {
			configReceived <- config
		},
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	select {
	case got := <-configReceived:
		if got.Get("wifi/ssid") != "hand-edited" {
			t.Errorf("StartNetworking got wifi/ssid = %q, want %q", got.Get("wifi/ssid"), "hand-edited")
		}
	case <-time.After(time.Second):
		t.Error("StartNetworking was never called")
	}
}
func TestRunConsumesACloudInitSeedIntoTheConfigTree(t *testing.T) {
	// The locked ordering (epic gosd-rw6n): the seed is deleted, durably,
	// BEFORE its values are written into the tree. The two read-write
	// windows below are the two points a power cut can freeze the card at,
	// so what each one leaves behind is the whole behaviour: never a card
	// that still holds a seed and has already been written to.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "user-data"), "hostname: cloud-init-name\n")

	hostname := &fakeHostname{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	var windows []string
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
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(nil),
		ReadProvisioning: func(log func(string, ...any)) provision.Result {
			return provision.Result{Hostname: "cloud-init-name", SeedFiles: []string{"user-data"}}
		},
		EditBoot: func(edit func(root string) error) error {
			err := edit(root)
			windows = append(windows, cardState(t, root))
			return err
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantWindows := []string{"seed absent, hostname unwritten", "seed absent, hostname=cloud-init-name"}
	if !slices.Equal(windows, wantWindows) {
		t.Errorf("card after each read-write window = %v, want %v (the seed goes first, durably)", windows, wantWindows)
	}
	wantCalls := []string{"baked-in-name", "cloud-init-name"}
	if !slices.Equal(hostname.set, wantCalls) {
		t.Errorf("SetHostname calls = %v, want %v (the wizard's answer applies to this boot too)", hostname.set, wantCalls)
	}
	if !strings.Contains(console.String(), "config/hostname set from cloud-init provisioning") {
		t.Errorf("console output missing the setting-written log line: %q", console.String())
	}
}

func TestRunAppliesACloudInitSeedItCannotWriteToTheCard(t *testing.T) {
	// A card that has gone read-only costs the device the wizard's answers
	// on the NEXT boot, never on this one: what was asked for still takes
	// effect, and nothing is written to a card whose seed couldn't be
	// deleted first.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "user-data"), "hostname: cloud-init-name\n")

	hostname := &fakeHostname{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	edits := 0
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
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(nil),
		ReadProvisioning: func(log func(string, ...any)) provision.Result {
			return provision.Result{Hostname: "cloud-init-name", SeedFiles: []string{"user-data"}}
		},
		EditBoot: func(edit func(root string) error) error {
			edits++
			return errors.New("read-only filesystem")
		},
		Sleep: func(d time.Duration) { clock.Sleep(d) },
		Now:   clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (an unwritable card is not fatal)", err)
	}

	if edits != 1 {
		t.Errorf("EditBoot called %d times, want 1 (nothing is written once the seed couldn't be deleted)", edits)
	}
	if got := cardState(t, root); got != "seed present, hostname unwritten" {
		t.Errorf("card = %q, want the seed left alone and nothing written", got)
	}
	if want := []string{"baked-in-name", "cloud-init-name"}; !slices.Equal(hostname.set, want) {
		t.Errorf("SetHostname calls = %v, want %v (the answer still applies to this boot)", hostname.set, want)
	}
}
func TestRunCloudInitOverwritesASettingAlreadyOnTheCard(t *testing.T) {
	// Somebody ran the flashing tool's wizard after this image was built,
	// which makes their answer the most recent statement of intent there
	// is — including over a value the image (or an earlier edit) put in the
	// tree. This is what keeps a wizard's hostname from being shadowed by a
	// default, the bug gosd-4hz1 fixed.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "user-data"), "hostname: cloud-init-name\n")

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
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(map[string]string{"hostname": "already-on-the-card"}),
		ReadProvisioning: func(log func(string, ...any)) provision.Result {
			return provision.Result{Hostname: "cloud-init-name", SeedFiles: []string{"user-data"}}
		},
		EditBoot: func(edit func(root string) error) error { return edit(root) },
		Sleep:    func(d time.Duration) { clock.Sleep(d) },
		Now:      clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantCalls := []string{"baked-in-name", "cloud-init-name"}
	if !slices.Equal(hostname.set, wantCalls) {
		t.Errorf("SetHostname calls = %v, want %v (the wizard's answer wins)", hostname.set, wantCalls)
	}
	if got := cardState(t, root); got != "seed absent, hostname=cloud-init-name" {
		t.Errorf("card = %q, want the wizard's answer written over the old setting", got)
	}
}
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
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(map[string]string{"hostname": "hand-edited-name"}),
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

	if want := []string{"hand-edited-name"}; !slices.Equal(gotHostnames, want) {
		t.Errorf("WriteHosts calls = %v, want %v (called once, with the name the card gave)", gotHostnames, want)
	}
}
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

func TestRunPassesACloudInitWifiNetworkToStartNetworkingAsACardSetting(t *testing.T) {
	// A wizard's WiFi answers reach wifiup the same way a hand-edit does:
	// as settings in the tree, consumed from the seed before anything reads
	// them.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "network-config"), "version: 2\n")

	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	configReceived := make(chan cardconfig.Tree, 1)

	appStarter := funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})

	deps := Deps{
		Mounter:        &fakeMounter{},
		Hostname:       &fakeHostname{},
		AppStarter:     appStarter,
		Reaper:         fakeReaper{},
		Rebooter:       &fakeRebooter{},
		OpenConsole:    func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog:    func(string, ...any) {},
		ReadConfig:     func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(nil),
		ReadProvisioning: func(log func(string, ...any)) provision.Result {
			return provision.Result{
				Wifi:      []provision.WifiNetwork{{SSID: "cloud-init-ssid", Password: "cloud-init-password"}},
				SeedFiles: []string{"network-config"},
			}
		},
		EditBoot: func(edit func(root string) error) error { return edit(root) },
		Sleep:    func(d time.Duration) { clock.Sleep(d) },
		Now:      clock.Now,
		StartNetworking: func(cfg initcfg.Config, config cardconfig.Tree, log func(string, ...any)) {
			configReceived <- config
		},
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	select {
	case got := <-configReceived:
		if got.Get("wifi/ssid") != "cloud-init-ssid" || got.Get("wifi/passphrase") != "cloud-init-password" {
			t.Errorf("StartNetworking got wifi/ssid=%q wifi/passphrase set=%v, want the network the seed named", got.Get("wifi/ssid"), got.Get("wifi/passphrase") != "")
		}
	case <-time.After(time.Second):
		t.Error("StartNetworking was never called")
	}

	onCard := readCardValue(t, root, "wifi/ssid")
	if onCard != "cloud-init-ssid" {
		t.Errorf("config/wifi/ssid on the card = %q, want it written from the seed so it survives a reflash", onCard)
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
			log("firstrun.sh found on the boot partition; gosd-init never parses or executes it — edit the files in config/ to configure this device instead")
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

func TestRunRejectsAnOverlongHostnameOnTheCard(t *testing.T) {
	// A hostname typed onto the card over naming.MaxLength bytes must never
	// reach SetHostname: it's rejected, logged against the file to fix, and
	// the previous (baked-in) hostname is kept (gosd-jeaw).
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
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(map[string]string{"hostname": tooLong}),
		Sleep:          clock.Sleep,
		Now:            clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (an unusable hostname is not fatal)", err)
	}
	if rebooter.rebooted {
		t.Error("Run() rebooted over an unusable hostname on the card")
	}

	if want := []string{"baked-in-name"}; !slices.Equal(hostname.set, want) {
		t.Errorf("SetHostname calls = %v, want %v (unusable hostname rejected, previous kept)", hostname.set, want)
	}
	if !strings.Contains(console.String(), "config/hostname") || !strings.Contains(console.String(), "can't be used as a hostname") {
		t.Errorf("console output missing the hostname rejection log line: %q", console.String())
	}
}
func TestRunRejectsAnInvalidCharsetHostnameFromCloudInit(t *testing.T) {
	// Charset validation shares the same naming.Sanitize semantics as the
	// length cap, and applies to a wizard's answer too: it lands in the
	// tree, where its author can see and correct it, but never reaches
	// SetHostname.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "user-data"), "hostname: Not A Valid Host!\n")

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
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(nil),
		ReadProvisioning: func(log func(string, ...any)) provision.Result {
			return provision.Result{Hostname: "Not A Valid Host!", SeedFiles: []string{"user-data"}}
		},
		EditBoot: func(edit func(root string) error) error { return edit(root) },
		Sleep:    clock.Sleep,
		Now:      clock.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil (an unusable hostname is not fatal)", err)
	}

	if want := []string{"baked-in-name"}; !slices.Equal(hostname.set, want) {
		t.Errorf("SetHostname calls = %v, want %v (unusable hostname rejected, previous kept)", hostname.set, want)
	}
	if got := readCardValue(t, root, "hostname"); got != "Not A Valid Host!" {
		t.Errorf("config/hostname on the card = %q, want what was asked for, visible for its author to correct", got)
	}
	if !strings.Contains(console.String(), "can't be used as a hostname") {
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

func TestRunCannotRecordAFatalBeforeTheBootPartitionIsMounted(t *testing.T) {
	// The two failures gosd-init can raise before the boot mount succeeds
	// have nowhere to write a report — the partition they'd be written to
	// is the one that isn't there. The console must say so rather than the
	// device silently leaving no trace at all.
	mounter := &fakeMounter{fn: func(c mountCall) error {
		if c.target == "/boot" {
			return errBoom
		}
		return nil
	}}
	clock := newFakeClock(time.Unix(0, 0))
	console := &bytes.Buffer{}
	reports := &fakeFaultReport{}
	var sleeps []time.Duration

	deps := testDepsForFatalPath(mounter, &fakeHostname{}, &fakeRebooter{}, clock, &sleeps)
	deps.OpenConsole = func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil }
	deps.FaultReport = reports.deps()

	if err := Run(deps, testOptions()); err == nil {
		t.Fatal("Run() = nil, want an error about mounting the boot partition")
	}
	if got := reports.writeCount(); got != 0 {
		t.Errorf("wrote %d reports to a boot partition that never mounted", got)
	}
	if !strings.Contains(console.String(), "LAST_FATAL_ERROR.md can't be written") {
		t.Errorf("console output doesn't explain why there's no crash report: %q", console.String())
	}
}

func TestRunCountsTheBootOnlyOnceTheDataPartitionIsMounted(t *testing.T) {
	// The counter is durable state, so it lives on /data and can't be
	// touched before that mount — see countBoot in cmd/gosd-init.
	mounter := &fakeMounter{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	reports := &fakeFaultReport{bootCount: 37}
	dataMountsAtCount := -1
	counted := 0

	faultDeps := reports.deps()
	countBoot := faultDeps.CountBoot
	faultDeps.CountBoot = func() (int, bool) {
		dataMountsAtCount = mounter.callsFor("/data")
		counted++
		return countBoot()
	}

	deps := Deps{
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
		ReadConfig:           func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline:          func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		EnsureDataMountpoint: func() error { return nil },
		FaultReport:          faultDeps,
		Sleep:                func(d time.Duration) { clock.Sleep(d) },
		Now:                  clock.Now,
	}
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if counted != 1 {
		t.Errorf("counted the boot %d times, want exactly once", counted)
	}
	if dataMountsAtCount == 0 {
		t.Error("the boot was counted before /data was mounted, so the count had nowhere durable to go")
	}
}

func TestRunDeletesAStaleReportOnceTheAppHasRunStably(t *testing.T) {
	// A device that crashed, rebooted and has been running happily since
	// must not still look broken to whoever pulls the card — and the delete
	// has to happen while the app is still up, since an app that works
	// never exits at all.
	mounter := &fakeMounter{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	stable := make(chan time.Time, 1)
	deleted := make(chan struct{})
	reports := &fakeFaultReport{present: map[string]bool{faultreport.FileName: true}}

	faultDeps := reports.deps()
	remove := faultDeps.Remove
	faultDeps.Remove = func(names []string) error {
		err := remove(names)
		close(deleted)
		return err
	}

	deps := Deps{
		Mounter:  mounter,
		Hostname: &fakeHostname{},
		AppStarter: funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
			stable <- time.Unix(0, 0)
			return 1, nil
		}),
		Reaper: funcReaper(func(int) (ExitStatus, error) {
			select {
			case <-deleted:
			case <-time.After(2 * time.Second):
			}
			close(stop)
			return ExitStatus{}, nil
		}),
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		FaultReport: faultDeps,
		Sleep:       func(d time.Duration) { clock.Sleep(d) },
		Now:         clock.Now,
		After:       func(time.Duration) <-chan time.Time { return stable },
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	removals := reports.removals()
	if len(removals) != 1 || len(removals[0]) != 1 || removals[0][0] != faultreport.FileName {
		t.Errorf("deleted %v from the boot partition, want exactly [%s]", removals, faultreport.FileName)
	}
}

// TestRunTeesAppOutputToConsoleUnchanged pins gosd-s9uq's hardest console
// requirement: serial is the bench's only diagnostic channel, so teeing
// /app's stdout/stderr into the crash-tail buffer must not alter a single
// byte of what still reaches it.
func TestRunTeesAppOutputToConsoleUnchanged(t *testing.T) {
	console := &bytes.Buffer{}
	stop := make(chan struct{})

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		_, _ = fmt.Fprint(stdout, "hello from stdout\n")
		_, _ = fmt.Fprint(stderr, "hello from stderr\n")
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
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		Sleep:       func(time.Duration) {},
		Now:         time.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if !strings.Contains(console.String(), "hello from stdout\n") {
		t.Errorf("console output missing the app's stdout verbatim: %q", console.String())
	}
	if !strings.Contains(console.String(), "hello from stderr\n") {
		t.Errorf("console output missing the app's stderr verbatim: %q", console.String())
	}
}

// TestRunWritesAnAppCrashReportOnANonZeroExit is the acceptance test for
// gosd-s9uq: a panic, segfault or OOM kill previously only ever scrolled
// past on a serial cable nobody had attached. Now PID 1 holds a copy and
// records it.
func TestRunWritesAnAppCrashReportOnANonZeroExit(t *testing.T) {
	stop := make(chan struct{})
	reports := &fakeFaultReport{}

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		_, _ = fmt.Fprint(stdout, "about to explode\n")
		return 1, nil
	})

	deps := Deps{
		Mounter:    &fakeMounter{},
		Hostname:   &fakeHostname{},
		AppStarter: appStarter,
		Reaper: funcReaper(func(int) (ExitStatus, error) {
			close(stop)
			return ExitStatus{ExitCode: 2}, nil
		}),
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		FaultReport: reports.deps(),
		Sleep:       func(time.Duration) {},
		Now:         time.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	written := reports.written()
	for _, want := range []string{"GOSD-APP-CRASH", "status 2", "about to explode"} {
		if !strings.Contains(written, want) {
			t.Errorf("LAST_FATAL_ERROR.md content %q missing %q", written, want)
		}
	}
}

// TestRunDoesNotWriteAReportOnACleanExit locks in the bean's explicit
// carve-out: exit 0 is not a crash, even though the supervisor restarts an
// app that exits 0 exactly the same as it would a crash.
func TestRunDoesNotWriteAReportOnACleanExit(t *testing.T) {
	stop := make(chan struct{})
	reports := &fakeFaultReport{}
	starts := 0

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
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
		Reaper:      fakeReaper{}, // always exits 0, unsignaled
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		FaultReport: reports.deps(),
		Sleep:       func(time.Duration) {},
		Now:         time.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if got := reports.writeCount(); got != 0 {
		t.Errorf("wrote %d reports for an app that only ever exited 0, want 0", got)
	}
}

// TestRunNamesASignalDeathInTheAppCrashReport pins the bean's explicit
// requirement that a signal death is named in human terms — "ran out of
// memory", not "signal 9" — using the widened Reaper contract's Signaled/
// Signal fields.
func TestRunNamesASignalDeathInTheAppCrashReport(t *testing.T) {
	stop := make(chan struct{})
	reports := &fakeFaultReport{}

	deps := Deps{
		Mounter:    &fakeMounter{},
		Hostname:   &fakeHostname{},
		AppStarter: funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) { return 1, nil }),
		Reaper: funcReaper(func(int) (ExitStatus, error) {
			close(stop)
			return ExitStatus{Signaled: true, Signal: syscall.SIGKILL, ExitCode: -1}, nil
		}),
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		FaultReport: reports.deps(),
		Sleep:       func(time.Duration) {},
		Now:         time.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	written := reports.written()
	if !strings.Contains(written, "ran out of memory") {
		t.Errorf("LAST_FATAL_ERROR.md content %q does not name the OOM kill in human terms", written)
	}
	if strings.Contains(written, "signal 9") {
		t.Errorf("LAST_FATAL_ERROR.md content %q leaked the bare signal number instead of naming it", written)
	}
}

// TestRunRedactsAnEnvSecretFromAnAppCrashReport confirms the crash-tail path
// is scrubbed by construction (gosd-m6py), not by anything specific to this
// bean: it feeds newAppCrashReport through the exact same report.record ->
// faultreport.Render path every other fatal already uses, so the app's own
// env secret — leaked into its own stdout, the most realistic way a secret
// reaches a console — must not survive into the written report.
func TestRunRedactsAnEnvSecretFromAnAppCrashReport(t *testing.T) {
	stop := make(chan struct{})
	reports := &fakeFaultReport{}

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		_, _ = fmt.Fprint(stdout, "auth failed with key sk_live_super_secret_key_1234\n")
		return 1, nil
	})

	deps := Deps{
		Mounter:    &fakeMounter{},
		Hostname:   &fakeHostname{},
		AppStarter: appStarter,
		Reaper: funcReaper(func(int) (ExitStatus, error) {
			close(stop)
			return ExitStatus{ExitCode: 1}, nil
		}),
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Env: map[string]string{"STRIPE_KEY": "sk_live_super_secret_key_1234"}}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		FaultReport: reports.deps(),
		Sleep:       func(time.Duration) {},
		Now:         time.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	written := reports.written()
	if strings.Contains(written, "sk_live_super_secret_key_1234") {
		t.Errorf("LAST_FATAL_ERROR.md still contains the app's own secret env value:\n%s", written)
	}
	if !strings.Contains(written, "{$STRIPE_KEY}") {
		t.Errorf("LAST_FATAL_ERROR.md is missing the redaction placeholder:\n%s", written)
	}
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
	// An image built with --data-size=0 (or from before the data partition existed)
	// has no partition 2: boot must proceed normally and the app must start,
	// but /data is mounted read-only so a write there fails with EROFS
	// instead of silently landing in RAM. The data partition's vfat mount (which
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
// setting: a baked config.json DataFlush value, and a card whose
// data_flush file holds card (empty for a card nobody has edited).
func dataFlushTestDeps(mounter *fakeMounter, console *bytes.Buffer, clock *fakeClock, stop chan struct{}, baked bool, card string, gotEnv *[]string) Deps {
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
		ReadConfigTree:       readsCard(map[string]string{"data_flush": card}),
		Sleep:                func(d time.Duration) { clock.Sleep(d) },
		Now:                  clock.Now,
	}
	return deps
}

// TestRunDataFlushDefaultsToNoFlush covers gosd-9m1k's locked default: with
// no --data-flush baked in and an empty data_flush file, /data is mounted
// without the vfat "flush" option, and the app sees GOSD_DATA_FLUSH=0.
func TestRunDataFlushDefaultsToNoFlush(t *testing.T) {
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotEnv []string

	deps := dataFlushTestDeps(mounter, console, clock, stop, false, "", &gotEnv)
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
// --data-flush: the baked default alone, with nothing on the card, must
// reach both the /data mount and GOSD_DATA_FLUSH.
func TestRunDataFlushBakedTrueAppliesToMountAndEnv(t *testing.T) {
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotEnv []string

	deps := dataFlushTestDeps(mounter, console, clock, stop, true, "", &gotEnv)
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

// TestRunCardDataFlushOverridesBakedDefault covers the card-editable
// setting turning flush ON despite a baked default of false, and reaching
// both the /data mount and the app's environment. Any word at all turns it
// on, which is what the file's own explanation on the card asks for.
func TestRunCardDataFlushOverridesBakedDefault(t *testing.T) {
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	var gotEnv []string

	deps := dataFlushTestDeps(mounter, console, clock, stop, false, "yes", &gotEnv)
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	calls := mounter.recordedCalls("/data")
	if len(calls) == 0 || calls[len(calls)-1].data != "flush" {
		t.Errorf("/data mount options = %+v, want the flush option (the card asked for it)", calls)
	}
	if !slices.Contains(gotEnv, "GOSD_DATA_FLUSH=1") {
		t.Errorf("app env = %v, want GOSD_DATA_FLUSH=1", gotEnv)
	}
	if !strings.Contains(console.String(), "data partition flush: true (config/data_flush)") {
		t.Errorf("console output missing the data-flush override log line: %q", console.String())
	}
}

// envDeps builds the minimal Deps needed to exercise app-env merging: a
// baked config.json (with the given Env) and a card whose config/env/
// directory holds cardEnv. gotEnv is populated with whatever env slice the
// AppStarter receives.
func envDeps(bakedEnv, cardEnv map[string]string, console *bytes.Buffer, gotEnv *[]string) (Deps, chan struct{}) {
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		*gotEnv = env
		close(stop)
		return 1, nil
	})

	card := make(map[string]string, len(cardEnv))
	for name, value := range cardEnv {
		card["env/"+name] = value
	}

	return Deps{
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
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(card),
		Sleep:          func(d time.Duration) { clock.Sleep(d) },
		Now:            clock.Now,
	}, stop
}
func TestRunInjectsBakedEnvWhenTheCardSetsNone(t *testing.T) {
	console := &bytes.Buffer{}
	var gotEnv []string
	deps, stop := envDeps(map[string]string{"FOO": "baked-foo"}, nil, console, &gotEnv)
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
func TestRunInjectsTheCardsEnvWhenNoneIsBaked(t *testing.T) {
	console := &bytes.Buffer{}
	var gotEnv []string
	deps, stop := envDeps(nil, map[string]string{"FOO": "card-foo"}, console, &gotEnv)
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0", "FOO=card-foo"}
	if !equalEnv(gotEnv, wantEnv) {
		t.Errorf("app env = %v, want %v", gotEnv, wantEnv)
	}
	if !strings.Contains(console.String(), "app env: FOO (config/env)") {
		t.Errorf("console output missing the card's env summary line: %q", console.String())
	}
}

func TestRunTreatsAnEmptyEnvFileAsUnset(t *testing.T) {
	// An empty file is how a card says "not set", so it falls back to the
	// baked value rather than handing the app an empty string.
	console := &bytes.Buffer{}
	var gotEnv []string
	deps, stop := envDeps(map[string]string{"FOO": "baked-foo"}, map[string]string{"FOO": ""}, console, &gotEnv)
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0", "FOO=baked-foo"}
	if !equalEnv(gotEnv, wantEnv) {
		t.Errorf("app env = %v, want %v (an empty setting file is unset, not empty)", gotEnv, wantEnv)
	}
}
func TestRunCardEnvOverridesBakedPerName(t *testing.T) {
	// FOO only baked, BAR overridden on the card, BAZ only on the card:
	// the merge is per-name, not a whole-map replace.
	console := &bytes.Buffer{}
	var gotEnv []string
	baked := map[string]string{"FOO": "baked-foo", "BAR": "baked-bar"}
	card := map[string]string{"BAR": "card-bar", "BAZ": "card-baz"}
	deps, stop := envDeps(baked, card, console, &gotEnv)
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0", "BAR=card-bar", "BAZ=card-baz", "FOO=baked-foo"}
	if !equalEnv(gotEnv, wantEnv) {
		t.Errorf("app env = %v, want %v (the card wins per-name over baked)", gotEnv, wantEnv)
	}
}
func TestRunRejectsReservedEnvNamesFromTheCard(t *testing.T) {
	// A card can't override the GOSD_* namespace gosd-init itself owns,
	// nor smuggle in an unrelated GOSD_-prefixed var; both are dropped —
	// named by the file to delete, since `gosd build` would have refused
	// them and only a hand-created file can get here — and the real
	// GOSD_BOARD/GOSD_HOSTNAME stay exactly as gosd-init set them.
	console := &bytes.Buffer{}
	var gotEnv []string
	card := map[string]string{
		"GOSD_BOARD": "attacker-board",
		"GOSD_X":     "should-be-dropped",
		"SAFE":       "card-safe",
	}
	deps, stop := envDeps(nil, card, console, &gotEnv)
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantEnv := []string{"GOSD_BOARD=pi-zero-2w", "GOSD_HOSTNAME=my-device", "GOSD_DATA_FLUSH=0", "SAFE=card-safe"}
	if !equalEnv(gotEnv, wantEnv) {
		t.Errorf("app env = %v, want %v (reserved names dropped, real GOSD_* intact)", gotEnv, wantEnv)
	}
	if !strings.Contains(console.String(), "ignoring config/env/GOSD_BOARD") {
		t.Errorf("console output missing GOSD_BOARD rejection log line: %q", console.String())
	}
	if !strings.Contains(console.String(), "ignoring config/env/GOSD_X") {
		t.Errorf("console output missing GOSD_X rejection log line: %q", console.String())
	}
}
func TestRunAppEnvIsUnchangedWhenNoUserEnvIsSet(t *testing.T) {
	console := &bytes.Buffer{}
	var gotEnv []string
	deps, stop := envDeps(nil, nil, console, &gotEnv)
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

func TestEnvRedactionRulesRedactsEachValueByItsOwnKey(t *testing.T) {
	rules := envRedactionRules([]string{"API_KEY=sekrit", "PORT=8080"})

	want := []redact.Rule{
		{Needle: "sekrit", Replacement: "{$API_KEY}"},
		{Needle: "8080", Replacement: "{$PORT}"},
	}
	if len(rules) != len(want) {
		t.Fatalf("envRedactionRules() = %v, want %v", rules, want)
	}
	for i := range want {
		if rules[i] != want[i] {
			t.Errorf("rules[%d] = %+v, want %+v", i, rules[i], want[i])
		}
	}
}

func TestEnvRedactionRulesIgnoresMalformedEntries(t *testing.T) {
	// mergeUserEnv only ever emits well-formed KEY=VALUE strings; this just
	// keeps the function from panicking if that ever stopped being true.
	if rules := envRedactionRules([]string{"NO_EQUALS_SIGN"}); len(rules) != 0 {
		t.Errorf("envRedactionRules(%q) = %v, want none", "NO_EQUALS_SIGN", rules)
	}
}

func TestIngressRedactionRulesNameEachCredentialByItsField(t *testing.T) {
	tree := cardconfig.Tree{}
	tree.Set(cloudflaredTokenPath, "eyJhIjoiZXhhbXBsZS10dW5uZWwifQ")
	tree.Set(tsfunnelAuthkeyPath, "tskey-auth-kEXAMPLE-123456")

	rules := ingressRedactionRules(tree)

	want := []redact.Rule{
		{Needle: "eyJhIjoiZXhhbXBsZS10dW5uZWwifQ", Replacement: "{ingress: cloudflared-token}"},
		{Needle: "tskey-auth-kEXAMPLE-123456", Replacement: "{ingress: tailscale-funnel-authkey}"},
	}
	if len(rules) != len(want) {
		t.Fatalf("ingressRedactionRules() = %v, want %v", rules, want)
	}
	for i := range want {
		if rules[i] != want[i] {
			t.Errorf("rules[%d] = %+v, want %+v", i, rules[i], want[i])
		}
	}
}

func TestIngressRedactionRulesIgnoresCredentialsNobodySet(t *testing.T) {
	// The overwhelmingly common card: ingress shipped in the image but
	// never configured. An empty needle would match nothing anyway, but a
	// rule for it would still be logged as skipped for being too short,
	// naming a credential this device doesn't have.
	if rules := ingressRedactionRules(cardconfig.Tree{}); len(rules) != 0 {
		t.Errorf("ingressRedactionRules() = %v, want none for a card that configures no ingress", rules)
	}
}

// The tunnel token is the one class of secret gosd-init holds ITSELF (bean
// gosd-tzd1), and it reaches a report by exactly the route an app's env
// value does: something printed it, and the console tail became the
// report's technical detail.
func TestRunRedactsTheCardsTunnelTokenFromAnAppCrashReport(t *testing.T) {
	const token = "eyJhIjoiZXhhbXBsZS10dW5uZWwifQ"
	stop := make(chan struct{})
	reports := &fakeFaultReport{}

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		_, _ = fmt.Fprintf(stdout, "tunnel refused token %s\n", token)
		return 1, nil
	})

	deps := Deps{
		Mounter:    &fakeMounter{},
		Hostname:   &fakeHostname{},
		AppStarter: appStarter,
		Reaper: funcReaper(func(int) (ExitStatus, error) {
			close(stop)
			return ExitStatus{ExitCode: 1}, nil
		}),
		Rebooter:       &fakeRebooter{},
		OpenConsole:    func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog:    func(string, ...any) {},
		ReadConfig:     func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(map[string]string{cloudflaredTokenPath: token}),
		FaultReport:    reports.deps(),
		Sleep:          func(time.Duration) {},
		Now:            time.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	written := reports.written()
	if strings.Contains(written, token) {
		t.Errorf("LAST_FATAL_ERROR.md still contains the card's tunnel token:\n%s", written)
	}
	if !strings.Contains(written, "{ingress: cloudflared-token}") {
		t.Errorf("LAST_FATAL_ERROR.md is missing the redaction placeholder:\n%s", written)
	}
}

func TestWifiRedactionRulesCoverBothPlacesAPassphraseComesFrom(t *testing.T) {
	tree := cardconfig.Tree{}
	tree.Set(wifiSSIDPath, "kitchen-mesh")
	tree.Set(wifiPassphrasePath, "correct-horse-battery-staple")
	cfg := initcfg.Config{Wifi: initcfg.Wifi{SSID: "factory-net", Passphrase: "baked-in-passphrase"}}

	rules := wifiRedactionRules(tree, cfg)

	want := []redact.Rule{
		{Needle: "correct-horse-battery-staple", Replacement: "{wifi: passphrase}"},
		{Needle: "baked-in-passphrase", Replacement: "{wifi: passphrase}"},
	}
	if len(rules) != len(want) {
		t.Fatalf("wifiRedactionRules() = %v, want %v", rules, want)
	}
	for i := range want {
		if rules[i] != want[i] {
			t.Errorf("rules[%d] = %+v, want %+v", i, rules[i], want[i])
		}
	}
}

// An SSID is broadcast to anyone in radio range and is logged on purpose;
// redacting it would cost a WiFi failure the one detail that makes it
// diagnosable. A card that names an open network — SSID set, no passphrase
// — must produce no rule at all, or the console would log a skipped
// "{wifi: passphrase}" for a device that has no passphrase.
func TestWifiRedactionRulesIgnoreTheSSIDAndAnUnsetPassphrase(t *testing.T) {
	tree := cardconfig.Tree{}
	tree.Set(wifiSSIDPath, "open-guest-network")

	if rules := wifiRedactionRules(tree, initcfg.Config{}); len(rules) != 0 {
		t.Errorf("wifiRedactionRules() = %v, want none for an open network", rules)
	}
}

// The WiFi passphrase reaches a report by the same route an app env value
// or a tunnel token does (bean gosd-sk8v): something printed it, and the
// console tail became the report's technical detail.
func TestRunRedactsBothWifiPassphrasesFromAnAppCrashReport(t *testing.T) {
	const cardPassphrase = "correct-horse-battery-staple"
	const bakedPassphrase = "baked-in-passphrase"
	stop := make(chan struct{})
	reports := &fakeFaultReport{}

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		_, _ = fmt.Fprintf(stdout, "wpa handshake failed for %q / %q\n", cardPassphrase, bakedPassphrase)
		return 1, nil
	})

	deps := Deps{
		Mounter:    &fakeMounter{},
		Hostname:   &fakeHostname{},
		AppStarter: appStarter,
		Reaper: funcReaper(func(int) (ExitStatus, error) {
			close(stop)
			return ExitStatus{ExitCode: 1}, nil
		}),
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Wifi: initcfg.Wifi{SSID: "factory-net", Passphrase: bakedPassphrase}}, nil
		},
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(map[string]string{
			wifiSSIDPath:       "kitchen-mesh",
			wifiPassphrasePath: cardPassphrase,
		}),
		FaultReport: reports.deps(),
		Sleep:       func(time.Duration) {},
		Now:         time.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	written := reports.written()
	for _, passphrase := range []string{cardPassphrase, bakedPassphrase} {
		if strings.Contains(written, passphrase) {
			t.Errorf("LAST_FATAL_ERROR.md still contains the WiFi passphrase %q:\n%s", passphrase, written)
		}
	}
	if !strings.Contains(written, "{wifi: passphrase}") {
		t.Errorf("LAST_FATAL_ERROR.md is missing the redaction placeholder:\n%s", written)
	}
}

// The Imager wizard is the flagship way an operator supplies a passphrase,
// and it reaches redaction only because the seed is consumed into the tree
// BEFORE setSecrets reads it. Moving either would silently stop scrubbing
// the one passphrase most devices are given, with every other test still
// passing — so the ordering is asserted here rather than assumed.
func TestRunRedactsAWizardSuppliedWifiPassphraseFromAnAppCrashReport(t *testing.T) {
	const passphrase = "wizard-typed-passphrase"
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "network-config"), "version: 2\n")

	stop := make(chan struct{})
	reports := &fakeFaultReport{}

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		_, _ = fmt.Fprintf(stdout, "wpa handshake failed for %q\n", passphrase)
		return 1, nil
	})

	deps := Deps{
		Mounter:    &fakeMounter{},
		Hostname:   &fakeHostname{},
		AppStarter: appStarter,
		Reaper: funcReaper(func(int) (ExitStatus, error) {
			close(stop)
			return ExitStatus{ExitCode: 1}, nil
		}),
		Rebooter:       &fakeRebooter{},
		OpenConsole:    func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog:    func(string, ...any) {},
		ReadConfig:     func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(nil),
		ReadProvisioning: func(log func(string, ...any)) provision.Result {
			return provision.Result{
				Wifi:      []provision.WifiNetwork{{SSID: "kitchen-mesh", Password: passphrase}},
				SeedFiles: []string{"network-config"},
			}
		},
		EditBoot:    func(edit func(root string) error) error { return edit(root) },
		FaultReport: reports.deps(),
		Sleep:       func(time.Duration) {},
		Now:         time.Now,
	}
	opts := testOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	written := reports.written()
	if strings.Contains(written, passphrase) {
		t.Errorf("LAST_FATAL_ERROR.md still contains the wizard's WiFi passphrase:\n%s", written)
	}
	if !strings.Contains(written, "{wifi: passphrase}") {
		t.Errorf("LAST_FATAL_ERROR.md is missing the redaction placeholder:\n%s", written)
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

// testDataLabel is the per-app data-partition label these tests' config.json
// carries (`gosd build --label-prefix`): the boot sequence's job is to pass
// it through untouched, so nothing about this value is special.
const testDataLabel = "myapp-data"

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
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{DataExpand: dataExpand, DataLabel: testDataLabel}, nil
		},
		ReadCmdline:          func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		EnsureDataMountpoint: func() error { return nil },
		ExpandData: func(bootDevice string, filesystem diskfmt.FS, dataLabel string, expand bool, log func(format string, args ...any)) error {
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
	var gotDataLabel string

	deps := expandTestDeps(mounter, newFakeClock(time.Unix(0, 0)), stop, true, nil, &expandedWith)
	expand := deps.ExpandData
	deps.ExpandData = func(bootDevice string, filesystem diskfmt.FS, dataLabel string, doExpand bool, log func(format string, args ...any)) error {
		dataMountsAtExpand = mounter.callsFor("/data")
		gotDataLabel = dataLabel
		return expand(bootDevice, filesystem, dataLabel, doExpand, log)
	}
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	// The device passed is the one the boot mount actually used, so only
	// the disk the system truly booted from can ever be expanded.
	if len(expandedWith) != 1 || expandedWith[0] != "/dev/mmcblk0p1" {
		t.Errorf("ExpandData called with %v, want exactly [/dev/mmcblk0p1]", expandedWith)
	}
	if gotDataLabel != testDataLabel {
		t.Errorf("ExpandData data label = %q, want config.json's %q", gotDataLabel, testDataLabel)
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
	// LAST_FATAL_ERROR.md and halt — not reboot-loop, not reformat, and
	// above all not start the app against a read-only fallback as if
	// nothing were wrong.
	mounter := &fakeMounter{}
	rebooter := &fakeRebooter{}
	stop := make(chan struct{})
	var expandedWith []string
	reports := &fakeFaultReport{}

	deps := expandTestDeps(mounter, newFakeClock(time.Unix(0, 0)), stop, true,
		fmt.Errorf("%w: /dev/mmcblk0p2 holds nothing (blank space)", dataexpand.ErrDataCorrupt), &expandedWith)
	deps.Rebooter = rebooter
	deps.AppStarter = funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		t.Error("the app was started despite a corrupt data partition")
		close(stop)
		return 1, nil
	})
	deps.FaultReport = reports.deps()
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
	// gosd-fs34: this halt path had no 5s reboot pause to accidentally
	// cover for it, which is exactly why it was the path that lost its
	// console copy on the bench — the flush must happen before Halt.
	assertBefore(t, rebooter.callOrder(), "FlushConsole", "Halt")
	// The recovery instructions have to name the exact volume the next boot
	// will accept — this image's own filesystem and per-app label — since
	// that is all whoever reads the report has to go on. This is an expand
	// image, so deleting the partition really does get it recreated.
	recorded := reports.written()
	for _, want := range []string{"GOSD-DATA-CORRUPT", "/dev/mmcblk0p2", "save anything you need", "FAT32", testDataLabel, "the next boot will recreate it"} {
		if !strings.Contains(recorded, want) {
			t.Errorf("LAST_FATAL_ERROR.md content %q is missing %q", recorded, want)
		}
	}
	// The counter lives on /data, which is exactly what's broken here.
	if !strings.Contains(recorded, "boot: unknown") {
		t.Errorf("LAST_FATAL_ERROR.md content %q claims a boot number it can't know", recorded)
	}
	if mounter.callsFor("/data") != 0 {
		t.Error("/data was mounted despite the halt")
	}
}

// TestRunRedactsARegisteredSecretFromTheDataCorruptionReport proves the
// RegisteredSecrets seam is wired all the way from FaultReportDeps through
// to a real report Run() writes: unlike the env-value sweep (only set up
// once mergeUserEnv runs, well after this halt), a /run registration is
// read fresh at record time regardless of where in the boot sequence the
// failure happens, so it must redact here too.
func TestRunRedactsARegisteredSecretFromTheDataCorruptionReport(t *testing.T) {
	mounter := &fakeMounter{}
	stop := make(chan struct{})
	var expandedWith []string
	reports := &fakeFaultReport{}
	reports.setRegisteredSecrets([]redact.Rule{{Needle: "mmcblk0p2-secret-token", Replacement: "{secret: disk-token}"}})

	deps := expandTestDeps(mounter, newFakeClock(time.Unix(0, 0)), stop, true,
		fmt.Errorf("%w: mmcblk0p2-secret-token was leaked into this error by mistake", dataexpand.ErrDataCorrupt), &expandedWith)
	deps.Rebooter = &fakeRebooter{}
	deps.AppStarter = funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		close(stop)
		return 1, nil
	})
	deps.FaultReport = reports.deps()
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); !errors.Is(err, dataexpand.ErrDataCorrupt) {
		t.Fatalf("Run() = %v, want the corruption error", err)
	}

	recorded := reports.written()
	if strings.Contains(recorded, "mmcblk0p2-secret-token") {
		t.Errorf("LAST_FATAL_ERROR.md still contains the registered secret:\n%s", recorded)
	}
	if !strings.Contains(recorded, "{secret: disk-token}") {
		t.Errorf("LAST_FATAL_ERROR.md is missing the registered-secret placeholder:\n%s", recorded)
	}
}

// TestRunTellsAFixedSizeImageToReflashRatherThanWaitForARecreatedPartition
// covers the other image shape that can reach the halt: a fixed-size
// --data-filesystem=ext4 image ships partition 2 in the image and never
// creates one at boot, so the recovery advice an expand image gets ("delete
// it, the next boot recreates it") would strand whoever followed it.
func TestRunTellsAFixedSizeImageToReflashRatherThanWaitForARecreatedPartition(t *testing.T) {
	mounter := &fakeMounter{}
	stop := make(chan struct{})
	var expandedWith []string
	reports := &fakeFaultReport{}

	deps := expandTestDeps(mounter, newFakeClock(time.Unix(0, 0)), stop, false,
		fmt.Errorf("%w: /dev/mmcblk0p2 holds nothing (blank space)", dataexpand.ErrDataCorrupt), &expandedWith)
	deps.ReadConfig = func() (initcfg.Config, error) {
		return initcfg.Config{DataFilesystem: "ext4", DataLabel: testDataLabel}, nil
	}
	deps.Rebooter = &fakeRebooter{}
	deps.AppStarter = funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
		t.Error("the app was started despite a corrupt data partition")
		close(stop)
		return 1, nil
	})
	deps.FaultReport = reports.deps()
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); !errors.Is(err, dataexpand.ErrDataCorrupt) {
		t.Fatalf("Run() = %v, want the corruption error", err)
	}
	recorded := reports.written()
	if !strings.Contains(recorded, "flash this image") {
		t.Errorf("LAST_FATAL_ERROR.md content %q doesn't tell a fixed-size image's owner to re-flash", recorded)
	}
	if strings.Contains(recorded, "the next boot will recreate it") {
		t.Errorf("LAST_FATAL_ERROR.md content %q promises a recreated partition this image never creates", recorded)
	}
}

// TestRunExpandsAFixedSizeEXT4ImageEvenWithoutTheExpandFlag covers the
// second job dataexpand does: a fixed-size --data-filesystem=ext4 image has
// dataExpand=false (partition 2 already exists), but still needs its
// golden filesystem grown to the partition's real size on first boot, so
// ExpandData must still run.
func TestRunExpandsAFixedSizeEXT4ImageEvenWithoutTheExpandFlag(t *testing.T) {
	mounter := &fakeMounter{}
	stop := make(chan struct{})
	var expandedWith []string
	var gotFilesystem diskfmt.FS
	var gotExpand bool

	deps := Deps{
		Mounter:  mounter,
		Hostname: &fakeHostname{},
		AppStarter: funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
			close(stop)
			return 1, nil
		}),
		Reaper:      fakeReaper{},
		Rebooter:    &fakeRebooter{},
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{DataFilesystem: "ext4", DataLabel: testDataLabel}, nil
		},
		ReadCmdline:          func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		EnsureDataMountpoint: func() error { return nil },
		ExpandData: func(bootDevice string, filesystem diskfmt.FS, dataLabel string, expand bool, log func(format string, args ...any)) error {
			expandedWith = append(expandedWith, bootDevice)
			gotFilesystem, gotExpand = filesystem, expand
			return nil
		},
		Sleep: func(d time.Duration) { newFakeClock(time.Unix(0, 0)).Sleep(d) },
		Now:   newFakeClock(time.Unix(0, 0)).Now,
	}
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if len(expandedWith) != 1 {
		t.Fatalf("ExpandData called %d times, want exactly once (dataFilesystem=ext4 alone must trigger it)", len(expandedWith))
	}
	if gotFilesystem != diskfmt.EXT4 {
		t.Errorf("ExpandData filesystem = %s, want ext4", gotFilesystem)
	}
	if gotExpand {
		t.Error("ExpandData expand = true, want false (this image was not built with --data-size=expand)")
	}
}

// TestRunMountsTheDataPartitionAsEXT4WhenConfigured covers config.json's
// dataFilesystem reaching the actual /data mount call, not just the
// dataexpand step.
func TestRunMountsTheDataPartitionAsEXT4WhenConfigured(t *testing.T) {
	mounter := &fakeMounter{}
	console := &bytes.Buffer{}
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})

	deps := Deps{
		Mounter:  mounter,
		Hostname: &fakeHostname{},
		AppStarter: funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
			close(stop)
			return 1, nil
		}),
		Reaper:               fakeReaper{},
		Rebooter:             &fakeRebooter{},
		OpenConsole:          func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog:          func(string, ...any) {},
		ReadConfig:           func() (initcfg.Config, error) { return initcfg.Config{DataFilesystem: "ext4"}, nil },
		ReadCmdline:          func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		EnsureDataMountpoint: func() error { return nil },
		Sleep:                func(d time.Duration) { clock.Sleep(d) },
		Now:                  clock.Now,
	}
	opts := testDataOptions()
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	calls := mounter.recordedCalls("/data")
	if len(calls) == 0 || calls[len(calls)-1].fstype != "ext4" {
		t.Errorf("/data mount = %+v, want fstype ext4", calls)
	}
	if !strings.Contains(console.String(), "data partition filesystem: ext4") {
		t.Errorf("console output missing the resolved data filesystem: %q", console.String())
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
	// gosd-fs34: the console must be flushed before Reboot, not merely at
	// some point during the fatal path — the 5s sleep here happened to give
	// this path cover even before FlushConsole existed, but the guarantee
	// must not rest on that alone (see fatal's comment).
	assertBefore(t, rebooter.callOrder(), "FlushConsole", "Reboot")
}

// TestRunHaltsWithTheAppsOwnReportWhenItDeclaresAFault is the acceptance
// test for gosd-aa1p: an app that calls fault.Fatal leaves a report in /run
// and exits non-zero, which also looks like a crash. Exactly one report
// reaches the card — the app's own, since it names a fix the console tail
// never could — and the device stops rather than restarting into a failure
// the app has already said no restart can help.
func TestRunHaltsWithTheAppsOwnReportWhenItDeclaresAFault(t *testing.T) {
	reports := &fakeFaultReport{}
	rebooter := &fakeRebooter{}

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		_, _ = fmt.Fprintln(stdout, "panic: nil map write")
		return 1, nil
	})

	faultDeps := reports.deps()
	// One declared fault, delivered once — exactly what faultdrop.Take
	// does with the drop file the app left behind.
	declared := 0
	faultDeps.AppFault = func() (faultreport.Report, bool) {
		declared++
		if declared > 1 {
			return faultreport.Report{}, false
		}
		return faultreport.Report{
			Code:    "NO-API-KEY",
			Problem: "the weather service rejected our API key",
			Fix:     "add WEATHER_API_KEY to config/env/ on this card",
		}, true
	}

	deps := Deps{
		Mounter:     &fakeMounter{},
		Hostname:    &fakeHostname{},
		AppStarter:  appStarter,
		Reaper:      funcReaper(func(int) (ExitStatus, error) { return ExitStatus{ExitCode: 70}, nil }),
		Rebooter:    rebooter,
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		FaultReport: faultDeps,
		Sleep:       func(time.Duration) { t.Error("the supervisor waited to restart an app that declared a fatal fault") },
		Now:         time.Now,
	}

	// No opts.Stop: supervision has to end on its own, which is the
	// behaviour under test.
	if err := Run(deps, testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if !rebooter.halted {
		t.Error("the device did not halt")
	}
	if rebooter.rebooted {
		t.Error("the device rebooted; a declared fault must leave it down")
	}
	// gosd-fs34: this is the exact path the bench evidence reproduced —
	// the console copy of the app's own declared fault must be flushed
	// before the halt that follows it, not merely written at some point.
	assertBefore(t, rebooter.callOrder(), "FlushConsole", "Halt")
	if reports.writeCount() != 1 {
		t.Fatalf("wrote %d reports for one exit, want 1", reports.writeCount())
	}
	written := reports.written()
	for _, want := range []string{"NO-API-KEY", "add WEATHER_API_KEY to config/env/ on this card", "panic: nil map write"} {
		if !strings.Contains(written, want) {
			t.Errorf("LAST_FATAL_ERROR.md content %q missing %q", written, want)
		}
	}
	if strings.Contains(written, "GOSD-APP-CRASH") {
		t.Error("the crash-tail report won over the app's own; the fix it names is the reason this path exists")
	}
}

// TestGosdInitsOwnConsoleLinesNeverReachTheCardReport is the load-bearing
// assumption behind gosd-72ga's fix: gosd-init logs the full report to its
// own console (see fatalReporter.record), and that is only safe because
// nothing gosd-init itself writes to the console ever flows into
// consoletail — only /app's own stdout/stderr does (sequence.go's appOutput
// tee). If a future change routed gosd-init's own logging through that same
// tee, this report's Detail would start accumulating gosd-init's console
// lines — including, eventually, a whole previous report — the same
// self-nesting bug by a different route.
func TestGosdInitsOwnConsoleLinesNeverReachTheCardReport(t *testing.T) {
	reports := &fakeFaultReport{}
	rebooter := &fakeRebooter{}
	console := &bytes.Buffer{}

	appStarter := funcAppStarter(func(path string, env []string, stdout, stderr io.Writer) (int, error) {
		_, _ = fmt.Fprintln(stdout, "panic: nil map write")
		return 1, nil
	})

	faultDeps := reports.deps()
	declared := 0
	faultDeps.AppFault = func() (faultreport.Report, bool) {
		declared++
		if declared > 1 {
			return faultreport.Report{}, false
		}
		return faultreport.Report{Code: "NO-API-KEY", Problem: "the weather service rejected our API key"}, true
	}

	deps := Deps{
		Mounter:     &fakeMounter{},
		Hostname:    &fakeHostname{},
		AppStarter:  appStarter,
		Reaper:      funcReaper(func(int) (ExitStatus, error) { return ExitStatus{ExitCode: 70}, nil }),
		Rebooter:    rebooter,
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{console}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig:  func() (initcfg.Config, error) { return initcfg.Config{}, nil },
		ReadCmdline: func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		FaultReport: faultDeps,
		Sleep:       func(time.Duration) {},
		Now:         time.Now,
	}

	if err := Run(deps, testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	if reports.writeCount() != 1 {
		t.Fatalf("wrote %d reports for one exit, want 1", reports.writeCount())
	}

	// gosd-init's own lines are prefixed "[gosd] " (see logger.go); proving
	// that prefix reached the real console but never the card confirms the
	// two channels are actually independent, not just independent by
	// coincidence in this test's fixtures.
	if !strings.Contains(console.String(), "[gosd] ") {
		t.Fatal("the console never received any of gosd-init's own log lines; this test can't prove anything")
	}
	written := reports.written()
	if strings.Contains(written, "[gosd] ") {
		t.Errorf("LAST_FATAL_ERROR.md contains a gosd-init console line:\n%s\nwant only /app's own output folded in as technical detail", written)
	}
}

func TestRunPutsKeptSettingsBackOnTheCardAfterAReFlash(t *testing.T) {
	// The store lives on the data partition, which a re-flash leaves alone:
	// one directory outliving both boots below, while each boot gets the
	// card its own image was flashed onto.
	store := t.TempDir()
	baked := bakedDigests("hostname")

	// Somebody has named this device and given their app an environment
	// variable, so both are kept.
	bootOnce(t, store, t.TempDir(), "image-a", baked, map[string]string{"hostname": "kitchen-pi", "env/GREETING": "hello"})

	// A different image is written over the card: its config tree is back
	// to the values that image ships, and neither setting is on it.
	card := t.TempDir()
	hostname, env := bootOnce(t, store, card, "image-b", baked, map[string]string{"hostname": ""})

	if want := []string{"hello", "kitchen-pi"}; !slices.Equal(hostname.set, want) {
		t.Errorf("SetHostname calls = %v, want %v: the new image's own name, then the one put back, before /app starts", hostname.set, want)
	}
	if got := readCardValue(t, card, "hostname"); got != "kitchen-pi" {
		t.Errorf("hostname on the card = %q, want it back in the file its owner left it in", got)
	}
	if !slices.Contains(env, "GREETING=hello") {
		t.Errorf("app env = %v, want the restored GREETING among it", env)
	}
}

// bootOnce runs one whole boot of the image identity against a card holding
// card's settings and a store directory that outlives it, reporting the
// hostnames that boot set and the environment it started /app with.
func bootOnce(t *testing.T, store, root, identity string, baked, card map[string]string) (*fakeHostname, []string) {
	t.Helper()
	stop := make(chan struct{})
	var gotEnv []string
	hostname := &fakeHostname{}

	deps := Deps{
		Mounter:  &fakeMounter{},
		Hostname: hostname,
		Reaper:   fakeReaper{},
		Rebooter: &fakeRebooter{},
		AppStarter: funcAppStarter(func(_ string, env []string, _, _ io.Writer) (int, error) {
			gotEnv = env
			close(stop)
			return 1, nil
		}),
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "hello", Identity: identity, ConfigDigests: baked}, nil
		},
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(card),
		EditBoot:       func(edit func(root string) error) error { return edit(root) },
		Sleep:          func(time.Duration) {},
		Now:            time.Now,
	}
	opts := testOptions()
	opts.ConfigStoreDir = store
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	return hostname, gotEnv
}

// bakedDigests is the digest map config.json carries for an image whose
// settings all ship unset — the state a freshly flashed card is in.
func bakedDigests(paths ...string) map[string]string {
	tree := cardconfig.Tree{}
	digests := make(map[string]string, len(paths))
	for _, path := range paths {
		tree.Set(path, "")
		digests[path] = tree[path].SHA256()
	}
	return digests
}

// readsCard wires Deps.ReadConfigTree to hand Run a config tree holding
// values, keyed by tree path ("wifi/ssid", "env/API_TOKEN") — a card
// somebody has edited, or (nil) one nobody has.
func readsCard(values map[string]string) func(func(string, ...any)) cardconfig.Tree {
	tree := cardconfig.Tree{}
	for path, value := range values {
		tree.Set(path, value)
	}
	return func(func(string, ...any)) cardconfig.Tree { return tree }
}

// writeFile puts a file on a fake boot partition (a temp directory), for
// the tests that let Run really delete a cloud-init seed and really write
// settings.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// cardState summarises what a fake boot partition holds, so a test can
// assert what was durable at the end of each read-write window — the points
// a power cut could freeze the card at.
func cardState(t *testing.T, root string) string {
	t.Helper()
	state := "seed absent"
	if _, err := os.Stat(filepath.Join(root, "user-data")); err == nil {
		state = "seed present"
	}
	content, err := os.ReadFile(filepath.Join(root, configtree.Dir, "hostname"))
	if err != nil {
		return state + ", hostname unwritten"
	}
	return state + ", hostname=" + configtree.TrimValue(content)
}

// readCardValue reads one setting back off a fake boot partition, the way
// the device would.
func readCardValue(t *testing.T, root, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, configtree.Dir, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("reading %s off the card: %v", cardconfig.OnCard(path), err)
	}
	return configtree.TrimValue(content)
}

func TestRunRefusesARestoredHostnameThatWouldForgeAnEtcHostsLine(t *testing.T) {
	// Bean gosd-39da's attack, end to end and through the real
	// /etc/hosts renderer: something with write access to /data leaves a
	// hostname carrying a newline behind, and waits for the owner to
	// re-flash believing that resets the device. Go's pure resolver reads
	// /etc/hosts ahead of DNS for every lookup the app makes, so an extra
	// line here would silently re-point the app's API endpoint.
	store := t.TempDir()
	baked := bakedDigests("hostname")
	forged := "evil\n1.2.3.4 api.vendor.example"

	// Boot one plants it: a hand-edited card is exactly how a value enters
	// the store, and it differs from what the image ships, so it is kept.
	bootOnce(t, store, t.TempDir(), "image-a", baked, map[string]string{"hostname": forged})

	// Boot two is the re-flash: a different image, a card back at its
	// defaults, and the kept value put onto it.
	hosts := filepath.Join(t.TempDir(), "hosts")
	hostname := bootOnceWritingHosts(t, store, t.TempDir(), "image-b", baked, map[string]string{"hostname": ""}, hosts)

	content, err := os.ReadFile(hosts)
	if err != nil {
		t.Fatalf("reading the rendered /etc/hosts: %v", err)
	}
	if strings.Contains(string(content), "api.vendor.example") {
		t.Errorf("/etc/hosts gained an attacker-chosen mapping:\n%s", content)
	}
	if !strings.Contains(string(content), hostsfile.Static()) {
		t.Errorf("/etc/hosts lost its static localhost lines:\n%s", content)
	}
	if slices.Contains(hostname.set, forged) {
		t.Errorf("SetHostname calls = %q, want the forged name refused", hostname.set)
	}
}

func TestRunIgnoresARestoredEnvNameThatIsNotOne(t *testing.T) {
	// A config/env/ file's name becomes an environment variable's name.
	// The build refuses a malformed one, but the store is written long
	// after the build had its say and nothing authenticates it, so the
	// runtime holds it to the same rule rather than handing execve(2)
	// something it will reject for the whole app.
	store := t.TempDir()
	baked := bakedDigests("hostname")

	bootOnce(t, store, t.TempDir(), "image-a", baked, map[string]string{"env/NOT A NAME": "x", "env/FINE": "y"})
	_, env := bootOnce(t, store, t.TempDir(), "image-b", baked, map[string]string{"hostname": ""})

	for _, entry := range env {
		if strings.HasPrefix(entry, "NOT A NAME=") {
			t.Errorf("app env = %v, want the malformed name dropped", env)
		}
	}
	if !slices.Contains(env, "FINE=y") {
		t.Errorf("app env = %v, want the well-formed name beside it still restored", env)
	}
}

// bootOnceWritingHosts is bootOnce with /etc/hosts really rendered, to the
// given path, by the same package main wires in — so what a test asserts is
// the file a device would actually resolve against.
func bootOnceWritingHosts(t *testing.T, store, root, identity string, baked, card map[string]string, hostsPath string) *fakeHostname {
	t.Helper()
	stop := make(chan struct{})
	hostname := &fakeHostname{}

	deps := Deps{
		Mounter:  &fakeMounter{},
		Hostname: hostname,
		Reaper:   fakeReaper{},
		Rebooter: &fakeRebooter{},
		AppStarter: funcAppStarter(func(string, []string, io.Writer, io.Writer) (int, error) {
			close(stop)
			return 1, nil
		}),
		OpenConsole: func() (io.WriteCloser, error) { return nopWriteCloser{&bytes.Buffer{}}, nil },
		FallbackLog: func(string, ...any) {},
		ReadConfig: func() (initcfg.Config, error) {
			return initcfg.Config{Hostname: "hello", Identity: identity, ConfigDigests: baked}, nil
		},
		ReadCmdline:    func() (initcfg.CmdlineArgs, error) { return initcfg.CmdlineArgs{}, nil },
		ReadConfigTree: readsCard(card),
		EditBoot:       func(edit func(root string) error) error { return edit(root) },
		WriteHosts:     func(h string) error { return hostsfile.Write(hostsPath, h) },
		Sleep:          func(time.Duration) {},
		Now:            time.Now,
	}
	opts := testOptions()
	opts.ConfigStoreDir = store
	opts.Stop = stop

	if err := Run(deps, opts); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	return hostname
}
