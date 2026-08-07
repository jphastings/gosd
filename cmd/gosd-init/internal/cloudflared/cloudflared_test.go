package cloudflared

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/childbackoff"
	"github.com/jphastings/gosd/internal/gosdtoml"
)

func validConfig(t *testing.T) gosdtoml.IngressCloudflared {
	return gosdtoml.IngressCloudflared{Token: mustToken(t), Hostname: "app.example.com", Port: 8080}
}

func TestRunWritesCredentialsAndConfigFilesBeforeStarting(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	files := newFakeFiles()
	deps := baseTestDeps(clock, log)
	deps.MkdirAll = files.MkdirAll
	deps.WriteFile = files.WriteFile

	stop := make(chan struct{})
	defer close(stop)
	opts := baseTestOptions(validConfig(t), stop)

	go Run(deps, opts)

	if !waitForLog(log, "started (pid", 1) {
		t.Fatal("cloudflared never started")
	}

	if got := files.dirsCreated(); len(got) != 1 || got[0] != RuntimeDir {
		t.Fatalf("MkdirAll calls = %v, want exactly [%q]", got, RuntimeDir)
	}

	credentials, ok := files.get(CredentialsPath)
	if !ok {
		t.Fatalf("%s was never written", CredentialsPath)
	}
	wantCredentials := `{"AccountTag":"` + testAccountTag + `","TunnelSecret":"` + testTunnelSecret + `","TunnelID":"` + testTunnelID + `"}`
	if string(credentials) != wantCredentials {
		t.Errorf("%s = %s, want %s", CredentialsPath, credentials, wantCredentials)
	}

	config, ok := files.get(ConfigPath)
	if !ok {
		t.Fatalf("%s was never written", ConfigPath)
	}
	wantConfig := "tunnel: " + testTunnelID + "\n" +
		"credentials-file: /run/gosd/cloudflared/credentials.json\n" +
		"ingress:\n" +
		"  - hostname: app.example.com\n" +
		"    service: http://localhost:8080\n" +
		"  - service: http_status:404\n"
	if string(config) != wantConfig {
		t.Errorf("%s =\n%s\nwant\n%s", ConfigPath, config, wantConfig)
	}
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
		WriteFile:    func(string, []byte, os.FileMode) error { return nil },
		Clock:        clock,
		NewBackoff:   func() *childbackoff.Backoff { return childbackoff.NewBackoff(time.Second, 30*time.Second) },
		Log:          log.Printf,
	}
}

func baseTestOptions(cfg gosdtoml.IngressCloudflared, stop <-chan struct{}) Options {
	return Options{
		BinaryPath:             "/usr/bin/cloudflared",
		Baked:                  true,
		Config:                 cfg,
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
		t.Fatal("NetworkUp checked when cloudflared has nothing to do")
		return false, nil
	}

	opts := baseTestOptions(gosdtoml.IngressCloudflared{}, nil)
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
		t.Fatal("NetworkUp checked when cloudflared has nothing to do")
		return false, nil
	}

	opts := baseTestOptions(gosdtoml.IngressCloudflared{}, nil)

	Run(deps, opts)

	got := log.snapshot()
	if len(got) != 1 {
		t.Fatalf("logged lines = %v, want exactly 1", got)
	}
	if !strings.Contains(got[0], "nothing to do") {
		t.Errorf("logged line = %q, want it to mention there's nothing to do", got[0])
	}
}

func TestRunWaitsForNetworkUpBeforeWritingRuntimeFiles(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	up := &flag{}
	deps := baseTestDeps(clock, log)
	deps.NetworkUp = func() (bool, error) { return up.get(), nil }

	var wrote int
	deps.WriteFile = func(string, []byte, os.FileMode) error { wrote++; return nil }

	stop := make(chan struct{})
	defer close(stop)
	opts := baseTestOptions(validConfig(t), stop)

	go Run(deps, opts)

	if !waitForPending(clock, 1) {
		t.Fatal("cloudflared never registered the network-up poll timer")
	}
	if wrote != 0 {
		t.Fatalf("WriteFile called %d times before network was up, want 0", wrote)
	}

	up.set(true)
	clock.Advance(opts.NetworkUpPollInterval)

	if !waitForLog(log, "started (pid", 1) {
		t.Fatal("cloudflared never started once the network came up")
	}
}

func TestRunStopsWaitingForNetworkUpWhenStopClosed(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	deps := baseTestDeps(clock, log)
	deps.NetworkUp = func() (bool, error) { return false, nil } // never comes up

	stop := make(chan struct{})
	close(stop)
	opts := baseTestOptions(validConfig(t), stop)

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
	opts := baseTestOptions(validConfig(t), stop)
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
		t.Fatal("cloudflared never logged the time-sync timeout warning")
	}
	if !waitForLog(log, "started (pid", 1) {
		t.Fatal("cloudflared never started after the time-sync timeout")
	}
}

func TestRunProceedsImmediatelyWhenAlreadyTimeSynced(t *testing.T) {
	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	deps := baseTestDeps(clock, log)
	deps.TimeSynced = func() (bool, error) { return true, nil }

	stop := make(chan struct{})
	defer close(stop)
	opts := baseTestOptions(validConfig(t), stop)

	go Run(deps, opts)

	if !waitForLog(log, "started (pid", 1) {
		t.Fatal("cloudflared never started")
	}
	if log.contains("time sync did not complete") {
		t.Error("logged a time-sync timeout warning despite already being synced")
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
	opts := baseTestOptions(validConfig(t), stop)

	go Run(deps, opts)

	if !waitForStartCount(procs, 1) {
		t.Fatal("cloudflared never started")
	}
	call := procs.lastCall()

	if call.path != opts.BinaryPath {
		t.Errorf("path = %q, want %q", call.path, opts.BinaryPath)
	}
	wantArgs := []string{"tunnel", "--no-autoupdate", "--loglevel", "warn", "--config", "/run/gosd/cloudflared/config.yml", "run"}
	if len(call.args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", call.args, wantArgs)
	}
	for i := range wantArgs {
		if call.args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %q, want %q", i, call.args[i], wantArgs[i])
		}
	}
	wantEnv := []string{"HOME=/run/gosd/cloudflared"}
	if len(call.env) != len(wantEnv) || call.env[0] != wantEnv[0] {
		t.Errorf("env = %v, want %v", call.env, wantEnv)
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
	opts := baseTestOptions(validConfig(t), stop)

	done := make(chan struct{})
	go func() { supervise(deps, opts); close(done) }()

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
	opts := baseTestOptions(validConfig(t), stop)

	done := make(chan struct{})
	go func() { supervise(deps, opts); close(done) }()

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
	opts := baseTestOptions(validConfig(t), stop)
	opts.NetworkUpPollInterval = time.Second

	go func() { supervise(deps, opts) }()

	if !waitForStartCount(procs, 1) {
		t.Fatal("start 1 never happened")
	}
	up.set(false) // network drops while cloudflared is "running"
	procs.exit(1, 1, nil)

	// The next restart must park on the network-up gate, not burn a
	// backoff sleep: no backoff timer should appear.
	if !waitForPending(clock, 1) {
		t.Fatal("cloudflared never registered the network-up re-gate timer")
	}
	if got := procs.startCount(); got != 1 {
		t.Fatalf("started %d times while the network was down, want 1", got)
	}

	up.set(true)
	clock.Advance(opts.NetworkUpPollInterval)

	if !waitForStartCount(procs, 2) {
		t.Fatal("cloudflared never restarted once the network came back up")
	}
}

// TestTokenNeverAppearsInAnyLogOutput scans every log line produced across
// every resolveMode failure mode, plus a full runOnce (start, relayed
// stdout, exit) log sequence, for the raw secret and the raw encoded
// token — neither may ever appear in anything gosd-init logs (the
// credentials.json file it deliberately writes is a separate, expected
// carrier of the secret, and isn't part of this scan).
func TestTokenNeverAppearsInAnyLogOutput(t *testing.T) {
	token := mustToken(t)
	var allLines []string

	configs := []gosdtoml.IngressCloudflared{
		{},
		{Token: token},
		{Token: token, Hostname: "app.example.com"},
		{Token: token, Port: 8080},
		{Hostname: "app.example.com", Port: 8080},
		{Token: "garbage", Hostname: "app.example.com", Port: 8080},
		{Token: token, Hostname: "not a hostname", Port: 8080},
		{Token: token, Hostname: "app.example.com", Port: 99999},
		validConfig(t),
	}
	for _, cfg := range configs {
		for _, baked := range []bool{false, true} {
			if m := resolveMode(cfg, baked); m.log != "" {
				allLines = append(allLines, m.log)
			}
		}
	}

	log := &testLog{}
	clock := newFakeClock(time.Unix(0, 0))
	m := resolveMode(validConfig(t), true)
	deps := Deps{
		StartProcess: func(path string, args, env []string, stdout, stderr io.Writer) (int, error) {
			_, _ = stdout.Write([]byte("connection registered connIndex=0\n"))
			_, _ = stderr.Write([]byte("some warning from cloudflared\n"))
			return 4242, nil
		},
		Wait:  func(int) (int, error) { return 0, nil },
		Clock: clock,
		Log:   log.Printf,
	}
	opts := baseTestOptions(validConfig(t), nil)
	runOnce(deps, opts, childbackoff.NewBackoff(time.Second, 30*time.Second))
	if !m.run {
		t.Fatalf("test setup: resolveMode did not run: %q", m.log)
	}
	allLines = append(allLines, log.snapshot()...)

	for _, line := range allLines {
		if strings.Contains(line, testTunnelSecret) {
			t.Errorf("log line leaked the tunnel secret: %q", line)
		}
		if strings.Contains(line, token) {
			t.Errorf("log line leaked the raw encoded token: %q", line)
		}
	}
}
