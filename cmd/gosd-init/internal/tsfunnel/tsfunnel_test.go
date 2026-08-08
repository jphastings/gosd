package tsfunnel

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/childbackoff"
	"github.com/jphastings/gosd/internal/gosdtoml"
)

const testDeviceHostname = "app.example.com"

// validConfig returns a fully-valid [ingress.tailscale-funnel] section for
// tests that need the shim to actually run.
func validConfig() gosdtoml.IngressTailscaleFunnel {
	return gosdtoml.IngressTailscaleFunnel{Authkey: testAuthkey, Hostname: testDeviceHostname, Port: 8080, FunnelPort: 443}
}

// baseTestDeps wires every Deps field to an inert default; individual tests
// override whichever fields they're exercising.
func baseTestDeps(clock *fakeClock, log *testLog) Deps {
	return Deps{
		StartProcess: func(string, []string, []string, io.Writer, io.Writer) (int, error) { return 0, nil },
		Wait:         func(int) (int, error) { return 0, nil },
		NetworkUp:    func() (bool, error) { return true, nil },
		TimeSynced:   func() (bool, error) { return true, nil },
		MkdirAll:     func(string, os.FileMode) error { return nil },
		Clock:        clock,
		NewBackoff:   func() *childbackoff.Backoff { return childbackoff.NewBackoff(time.Second, 30*time.Second) },
		Log:          log.Printf,
	}
}

func baseTestOptions(cfg gosdtoml.IngressTailscaleFunnel, stop <-chan struct{}) Options {
	return Options{
		BinaryPath:             "/usr/bin/gosd-tsfunnel",
		Baked:                  true,
		Config:                 cfg,
		Hostname:               testDeviceHostname,
		NetworkUpPollInterval:  2 * time.Second,
		TimeSyncedTimeout:      2 * time.Minute,
		TimeSyncedPollInterval: 2 * time.Second,
		Stop:                   stop,
	}
}

func TestRunDoesNothingWhenNotConfiguredAndNotBaked(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	deps := baseTestDeps(clock, log)

	var started int
	deps.StartProcess = func(string, []string, []string, io.Writer, io.Writer) (int, error) {
		started++
		return 1, nil
	}
	deps.NetworkUp = func() (bool, error) {
		t.Fatal("NetworkUp checked when tailscale-funnel has nothing to do")
		return false, nil
	}

	opts := baseTestOptions(gosdtoml.IngressTailscaleFunnel{}, nil)
	opts.Baked = false

	Run(deps, opts)

	if started != 0 {
		t.Errorf("StartProcess called %d times, want 0", started)
	}
	if got := log.snapshot(); len(got) != 0 {
		t.Errorf("logged lines = %v, want none", got)
	}
}

func TestRunLogsOneQuietLineWhenBakedButNotConfigured(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	deps := baseTestDeps(clock, log)
	deps.NetworkUp = func() (bool, error) {
		t.Fatal("NetworkUp checked when tailscale-funnel has nothing to do")
		return false, nil
	}

	opts := baseTestOptions(gosdtoml.IngressTailscaleFunnel{}, nil)

	Run(deps, opts)

	got := log.snapshot()
	if len(got) != 1 {
		t.Fatalf("logged lines = %v, want exactly 1", got)
	}
	if !strings.Contains(got[0], "nothing to do") {
		t.Errorf("logged line = %q, want it to mention there's nothing to do", got[0])
	}
}

func TestRunWaitsForNetworkUpBeforeStateDirPreflight(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	up := &flag{}
	deps := baseTestDeps(clock, log)
	deps.NetworkUp = func() (bool, error) { return up.get(), nil }

	var mkdirCalls int
	deps.MkdirAll = func(string, os.FileMode) error { mkdirCalls++; return nil }

	stop := make(chan struct{})
	defer close(stop)
	opts := baseTestOptions(validConfig(), stop)

	go Run(deps, opts)

	if !waitForPending(clock, 1) {
		t.Fatal("tailscale-funnel never registered the network-up poll timer")
	}
	if mkdirCalls != 0 {
		t.Fatalf("MkdirAll called %d times before network was up, want 0", mkdirCalls)
	}

	up.set(true)
	clock.Advance(opts.NetworkUpPollInterval)

	if !waitForLog(log, "started (pid", 1) {
		t.Fatal("tailscale-funnel never started once the network came up")
	}
}

func TestRunStopsWaitingForNetworkUpWhenStopClosed(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	deps := baseTestDeps(clock, log)
	deps.NetworkUp = func() (bool, error) { return false, nil } // never comes up

	stop := make(chan struct{})
	close(stop)
	opts := baseTestOptions(validConfig(), stop)

	done := make(chan struct{})
	go func() { Run(deps, opts); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after Stop was closed")
	}
}

func TestRunProceedsAfterTimeSyncedTimeoutWithWarning(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	deps := baseTestDeps(clock, log)
	deps.TimeSynced = func() (bool, error) { return false, nil } // never syncs

	stop := make(chan struct{})
	defer close(stop)
	opts := baseTestOptions(validConfig(), stop)
	opts.TimeSyncedTimeout = 10 * time.Second
	opts.TimeSyncedPollInterval = 2 * time.Second

	go Run(deps, opts)

	// Poll interval fires 5 times (10s / 2s) before the deadline is hit on
	// the 6th check.
	for i := 0; i < 5; i++ {
		if !waitForPending(clock, 1) {
			t.Fatalf("time-synced poll timer %d never registered", i+1)
		}
		clock.Advance(opts.TimeSyncedPollInterval)
	}

	if !waitForLog(log, "time sync did not complete", 1) {
		t.Fatal("tailscale-funnel never logged the time-sync timeout warning")
	}
	if !waitForLog(log, "started (pid", 1) {
		t.Fatal("tailscale-funnel never started after the time-sync timeout")
	}
}

func TestRunProceedsImmediatelyWhenAlreadyTimeSynced(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	deps := baseTestDeps(clock, log)
	deps.TimeSynced = func() (bool, error) { return true, nil }

	stop := make(chan struct{})
	defer close(stop)
	opts := baseTestOptions(validConfig(), stop)

	go Run(deps, opts)

	if !waitForLog(log, "started (pid", 1) {
		t.Fatal("tailscale-funnel never started")
	}
	if log.contains("time sync did not complete") {
		t.Error("logged a time-sync timeout warning despite already being synced")
	}
}

func TestRunPreflightsStateDirBeforeStarting(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	dirs := newFakeDirs()
	deps := baseTestDeps(clock, log)
	deps.MkdirAll = dirs.MkdirAll

	stop := make(chan struct{})
	defer close(stop)
	opts := baseTestOptions(validConfig(), stop)

	go Run(deps, opts)

	if !waitForLog(log, "started (pid", 1) {
		t.Fatal("tailscale-funnel shim never started")
	}

	if got := dirs.dirsCreated(); len(got) != 1 || got[0] != StateDir {
		t.Fatalf("MkdirAll calls = %v, want exactly [%q]", got, StateDir)
	}
}

// TestRunLogsActionableLineWhenStateDirNotWritable exercises epic gosd-65uy
// decision 3's "read-only /data at runtime" case: a read-only data
// partition must produce one actionable line and never start the shim,
// since it could never persist tsnet's node identity anyway.
func TestRunLogsActionableLineWhenStateDirNotWritable(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	deps := baseTestDeps(clock, log)

	var started int
	deps.StartProcess = func(string, []string, []string, io.Writer, io.Writer) (int, error) {
		started++
		return 1, nil
	}
	deps.MkdirAll = func(string, os.FileMode) error { return errors.New("read-only file system") }

	opts := baseTestOptions(validConfig(), nil)

	Run(deps, opts)

	if started != 0 {
		t.Errorf("StartProcess called %d times, want 0", started)
	}
	got := log.snapshot()
	if len(got) != 1 {
		t.Fatalf("logged lines = %v, want exactly 1", got)
	}
	if !strings.Contains(got[0], StateDir) {
		t.Errorf("logged line = %q, want it to name %s", got[0], StateDir)
	}
	if !strings.Contains(got[0], "needs a data partition") {
		t.Errorf("logged line = %q, want it to mention the data-partition requirement", got[0])
	}
}

func TestSuperviseArgvAndEnvExactMatch(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	procs := newFakeProcesses()
	deps := baseTestDeps(clock, log)
	deps.StartProcess = procs.Start
	deps.Wait = procs.Wait

	stop := make(chan struct{})
	defer close(stop)
	cfg := validConfig()
	opts := baseTestOptions(cfg, stop)
	m := resolveMode(cfg, opts.Baked, opts.Hostname)

	go supervise(deps, opts, m)

	if !waitForStartCount(procs, 1) {
		t.Fatal("tailscale-funnel shim never started")
	}
	call := procs.lastCall()

	if call.path != opts.BinaryPath {
		t.Errorf("path = %q, want %q", call.path, opts.BinaryPath)
	}
	wantArgs := []string{"--statedir", StateDir, "--hostname", testDeviceHostname, "--backend", "http://localhost:8080", "--funnel-port", "443"}
	if len(call.args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", call.args, wantArgs)
	}
	for i := range wantArgs {
		if call.args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %q, want %q", i, call.args[i], wantArgs[i])
		}
	}
	wantEnv := []string{"TS_AUTHKEY=" + testAuthkey}
	if len(call.env) != len(wantEnv) || call.env[0] != wantEnv[0] {
		t.Errorf("env = %v, want %v", call.env, wantEnv)
	}
}

func TestSuperviseArgvOmitsAuthkeyOnceStateAlreadyExists(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	procs := newFakeProcesses()
	deps := baseTestDeps(clock, log)
	deps.StartProcess = procs.Start
	deps.Wait = procs.Wait

	stop := make(chan struct{})
	defer close(stop)
	cfg := gosdtoml.IngressTailscaleFunnel{Port: 8080} // no authkey: fine once tsnet state exists
	opts := baseTestOptions(cfg, stop)
	m := resolveMode(cfg, opts.Baked, opts.Hostname)

	go supervise(deps, opts, m)

	if !waitForStartCount(procs, 1) {
		t.Fatal("tailscale-funnel shim never started")
	}
	wantEnv := []string{"TS_AUTHKEY="}
	if got := procs.lastCall().env; len(got) != 1 || got[0] != wantEnv[0] {
		t.Errorf("env = %v, want %v", got, wantEnv)
	}
}

func TestSuperviseRestartsWithEscalatingBackoff(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	procs := newFakeProcesses()
	deps := baseTestDeps(clock, log)
	deps.StartProcess = procs.Start
	deps.Wait = procs.Wait

	stop := make(chan struct{})
	cfg := validConfig()
	opts := baseTestOptions(cfg, stop)
	m := resolveMode(cfg, opts.Baked, opts.Hostname)

	done := make(chan struct{})
	go func() { supervise(deps, opts, m); close(done) }()

	wantDelays := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	for i, delay := range wantDelays {
		if !waitForStartCount(procs, i+1) {
			t.Fatalf("start %d never happened", i+1)
		}
		procs.exit(i+1, 1, nil) // crashes immediately: never stable
		if !waitForPending(clock, 1) {
			t.Fatalf("backoff timer %d never registered", i+1)
		}
		clock.Advance(delay)
	}

	if !waitForStartCount(procs, len(wantDelays)+1) {
		t.Fatal("supervise did not restart after the final scripted backoff")
	}

	close(stop)
	procs.exit(len(wantDelays)+1, 0, nil)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervise did not return after Stop was closed")
	}
}

func TestSuperviseResetsBackoffAfterAStableRun(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	procs := newFakeProcesses()
	deps := baseTestDeps(clock, log)
	deps.StartProcess = procs.Start
	deps.Wait = procs.Wait

	stop := make(chan struct{})
	cfg := validConfig()
	opts := baseTestOptions(cfg, stop)
	m := resolveMode(cfg, opts.Baked, opts.Hostname)

	done := make(chan struct{})
	go func() { supervise(deps, opts, m); close(done) }()

	// Two immediate crashes escalate the backoff to 1s, then 2s.
	if !waitForStartCount(procs, 1) {
		t.Fatal("start 1 never happened")
	}
	procs.exit(1, 1, nil)
	if !waitForPending(clock, 1) {
		t.Fatal("backoff timer 1 never registered")
	}
	clock.Advance(1 * time.Second)

	if !waitForStartCount(procs, 2) {
		t.Fatal("start 2 never happened")
	}
	procs.exit(2, 1, nil)
	if !waitForPending(clock, 1) {
		t.Fatal("backoff timer 2 never registered")
	}
	clock.Advance(2 * time.Second)

	// Run 3 is stable (>= StableAfter): advance the clock before delivering
	// its exit, so runOnce measures a long enough run to reset the backoff.
	if !waitForStartCount(procs, 3) {
		t.Fatal("start 3 never happened")
	}
	clock.Advance(StableAfter)
	procs.exit(3, 0, nil)
	if !waitForPending(clock, 1) {
		t.Fatal("backoff timer 3 never registered")
	}

	// If the backoff had NOT reset, the next delay would be 4s: advancing
	// only 1s must already be enough to trigger the restart.
	clock.Advance(1 * time.Second)
	if !waitForStartCount(procs, 4) {
		t.Fatal("supervise did not restart at the reset (1s) delay after a stable run")
	}

	close(stop)
	procs.exit(4, 0, nil)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervise did not return after Stop was closed")
	}
}

func TestSuperviseRegatesNetworkUpBeforeEachRestart(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	procs := newFakeProcesses()
	up := &flag{}
	up.set(true)
	deps := baseTestDeps(clock, log)
	deps.StartProcess = procs.Start
	deps.Wait = procs.Wait
	deps.NetworkUp = func() (bool, error) { return up.get(), nil }

	stop := make(chan struct{})
	defer close(stop)
	cfg := validConfig()
	opts := baseTestOptions(cfg, stop)
	opts.NetworkUpPollInterval = time.Second
	m := resolveMode(cfg, opts.Baked, opts.Hostname)

	go func() { supervise(deps, opts, m) }()

	if !waitForStartCount(procs, 1) {
		t.Fatal("start 1 never happened")
	}
	up.set(false) // network drops while the shim is "running"
	procs.exit(1, 1, nil)

	// The next restart must park on the network-up gate, not burn a
	// backoff sleep: no backoff timer should appear.
	if !waitForPending(clock, 1) {
		t.Fatal("tailscale-funnel never registered the network-up re-gate timer")
	}
	if got := procs.startCount(); got != 1 {
		t.Fatalf("started %d times while the network was down, want 1", got)
	}

	up.set(true)
	clock.Advance(opts.NetworkUpPollInterval)

	if !waitForStartCount(procs, 2) {
		t.Fatal("tailscale-funnel never restarted once the network came back up")
	}
}

// TestAuthkeyNeverAppearsInAnyLogOutput scans every log line produced across
// every resolveMode failure mode, plus a full runOnce (start, relayed
// stdout, exit) log sequence, for the raw authkey — it may never appear in
// anything gosd-init logs (it travels only via TS_AUTHKEY in the child's
// environment, which this package never logs at all — see runOnce).
func TestAuthkeyNeverAppearsInAnyLogOutput(t *testing.T) {
	var allLines []string

	configs := []gosdtoml.IngressTailscaleFunnel{
		{},
		{Authkey: testAuthkey},
		{Authkey: testAuthkey, Port: 8080},
		{Authkey: testAuthkey, Port: 8080, FunnelPort: 9999},
		{Port: -1},
		{Port: 70000},
		validConfig(),
	}
	for _, cfg := range configs {
		for _, baked := range []bool{false, true} {
			if m := resolveMode(cfg, baked, testDeviceHostname); m.log != "" {
				allLines = append(allLines, m.log)
			}
		}
	}

	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	cfg := validConfig()
	m := resolveMode(cfg, true, testDeviceHostname)
	if !m.run {
		t.Fatalf("test setup: resolveMode did not run: %q", m.log)
	}
	deps := Deps{
		StartProcess: func(path string, args, env []string, stdout, stderr io.Writer) (int, error) {
			_, _ = stdout.Write([]byte("tsnet: connected to control plane\n"))
			_, _ = stderr.Write([]byte("tsnet: some warning\n"))
			return 4242, nil
		},
		Wait:  func(int) (int, error) { return 0, nil },
		Clock: clock,
		Log:   log.Printf,
	}
	opts := baseTestOptions(cfg, nil)
	runOnce(deps, opts, m, childbackoff.NewBackoff(time.Second, 30*time.Second))
	allLines = append(allLines, log.snapshot()...)

	for _, line := range allLines {
		if strings.Contains(line, testAuthkey) {
			t.Errorf("log line leaked the authkey: %q", line)
		}
	}
}
