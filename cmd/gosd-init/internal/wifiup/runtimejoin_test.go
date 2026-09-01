package wifiup

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/wifictl"
)

// runtimeJoinTestDeps builds a Deps for the runtime-join tests around
// clock, which callers drive with advanceUntil to confirm an association
// (see runUntilDisconnect/watchAssociation's associationPollPeriod wait):
// left un-advanced, a successfully joined association simply parks, which
// is exactly what lets one request after another be driven deterministically
// without a stray reconnect in between.
func runtimeJoinTestDeps(clock *fakeClock, wifi *fakeWifiClient, creds CredentialSource, log *testLog) (Deps, *recorder) {
	rec := &recorder{}
	deps := Deps{
		Wifi:        wifi,
		Credentials: creds,
		Links:       newFakeLinks(),
		DHCP:        &fakeDHCP{requestResults: []requestResult{{err: errBoom}}},
		Clock:       clock,
		NewBackoff:  backoffFactory(time.Second, 10*time.Second),
		WriteResolvConf: func([]net.IP) error {
			return nil
		},
		MarkNetworkUp:  func(string) error { return nil },
		ClearNetworkUp: func(string) error { return nil },
		RegisterSecret: rec.registerSecret,
		Persist:        rec.persist,
		RestartIngress: rec.restartIngress,
		Log:            log.Printf,
	}
	return deps, rec
}

// recorder captures what the Deps callbacks the reconciler drives were
// called with, for assertions.
type recorder struct {
	mu sync.Mutex

	secrets   []secretCall
	persists  []persistCall
	restartsN int
}

type secretCall struct{ secret, label string }
type persistCall struct{ ssid, passphrase string }

func (r *recorder) registerSecret(secret, label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secrets = append(r.secrets, secretCall{secret, label})
}

func (r *recorder) persist(ssid, passphrase string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.persists = append(r.persists, persistCall{ssid, passphrase})
	return nil
}

func (r *recorder) restartIngress() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restartsN++
}

func (r *recorder) restarts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.restartsN
}

func (r *recorder) persistCalls() []persistCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]persistCall, len(r.persists))
	copy(out, r.persists)
	return out
}

func (r *recorder) secretCalls() []secretCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]secretCall, len(r.secrets))
	copy(out, r.secrets)
	return out
}

// awaitStatus polls dir for a terminal status matching id, failing the test
// if none arrives before the deadline — the same polling shape wifi.Join's
// own awaitOutcome uses, reimplemented here directly against internal/wifictl
// (a top-level package can't reach into wifi's own unexported joiner from
// this internal package, and wifi.Join itself is gated off entirely without
// the `gosd` build tag `go test` never sets — see wifi's own package doc).
func awaitStatus(t *testing.T, dir, id string) wifictl.Status {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if status, ok, err := wifictl.ReadStatus(dir); ok && err == nil && status.ID == id && status.State.Terminal() {
			return status
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no terminal status for request %s within the deadline", id)
	return wifictl.Status{}
}

// awaitJoined confirms the association the way the real association loop
// requires (advancing past one associationPollPeriod, mirroring the rest of
// this package's tests — see advanceUntil), then waits for id's terminal
// status and requires it to be Joined. wantAssociatedCalls is the
// cumulative Connect()/associated-poll call count this confirmation should
// reach, since wifi is shared and long-lived across every request in a
// test. It first waits (real time) for connectCallCount to reach want,
// proving the watcher has already interrupted whatever generation came
// before this one and started a fresh associate() for THIS request — only
// then does it advance the fake clock, so that advance can only ever fire
// the new generation's own pending poll timer. Skipping that wait would
// race: advancing the clock the instant a request is written can instead
// fire a still-parked PRIOR generation's own pending poll timer before the
// watcher has gotten around to interrupting it, satisfying the associated-
// count condition without the new generation ever having started.
func awaitJoined(t *testing.T, clock *fakeClock, wifi *fakeWifiClient, dir, id string, want int) wifictl.Status {
	t.Helper()
	attempts := func() int { return wifi.connectCallCount() + wifi.connectPSKCallCount() }
	deadline := time.Now().Add(2 * time.Second)
	for attempts() < want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if attempts() < want {
		t.Fatalf("connect attempts never reached %d (got %d): the watcher never started this request's association attempt", want, attempts())
	}
	if !advanceUntil(clock, associationPollPeriod, func() bool { return wifi.associatedCallCount() >= want }) {
		t.Fatalf("associatedCalls never reached %d", want)
	}
	status := awaitStatus(t, dir, id)
	if status.State != wifictl.Joined {
		t.Fatalf("status = %+v, want Joined", status)
	}
	return status
}

// TestRuntimeJoinEndToEndThroughTheRealReconciler is the strongest single
// test for this bean: it drives the real wifiup.Run reconciler, through a
// real temp directory, using internal/wifictl exactly as the public wifi
// package's Join does (write a request, poll for its status) — proving the
// full joining -> joined status sequence end to end against production
// logic, not a fake reconciler.
func TestRuntimeJoinEndToEndThroughTheRealReconciler(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}}}
	log := &testLog{}
	deps, rec := runtimeJoinTestDeps(clock, wifi, fakeCredentials{ok: false}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop, RequestDir: dir, RequestPollInterval: time.Millisecond})

	req := wifictl.Request{ID: "req-1", SSID: "home-network", Passphrase: "correct-horse-battery", Persist: true}
	if err := wifictl.WriteRequest(dir, req); err != nil {
		t.Fatal(err)
	}

	// Observe the joining status before the terminal one, proving the
	// two-phase write the bean asks for (not just an eventual answer).
	deadline := time.Now().Add(2 * time.Second)
	sawJoining := false
	for time.Now().Before(deadline) {
		if status, ok, err := wifictl.ReadStatus(dir); ok && err == nil && status.ID == req.ID && status.State == wifictl.Joining {
			sawJoining = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !sawJoining {
		t.Error("never observed a joining status before the terminal one")
	}

	awaitJoined(t, clock, wifi, dir, req.ID, 1)

	if calls := rec.persistCalls(); len(calls) != 1 || calls[0] != (persistCall{"home-network", "correct-horse-battery"}) {
		t.Errorf("persist calls = %v, want exactly one for the requested network", calls)
	}
	if got := rec.restarts(); got != 1 {
		t.Errorf("ingress restarts = %d, want 1", got)
	}
	if calls := rec.secretCalls(); len(calls) != 1 || calls[0].secret != "correct-horse-battery" {
		t.Errorf("registered secrets = %v, want the request's passphrase registered once", calls)
	}
}

func TestRuntimeJoinFailureReasonIsSurfacedVerbatim(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		connectErr:        errBoom,
	}
	log := &testLog{}
	deps, rec := runtimeJoinTestDeps(clock, wifi, fakeCredentials{ok: false}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop, RequestDir: dir, RequestPollInterval: time.Millisecond})

	req := wifictl.Request{ID: "req-1", SSID: "flaky-net"}
	if err := wifictl.WriteRequest(dir, req); err != nil {
		t.Fatal(err)
	}

	status := awaitStatus(t, dir, req.ID)
	if status.State != wifictl.Failed {
		t.Fatalf("status = %+v, want Failed", status)
	}
	if !strings.Contains(status.Error, errBoom.Error()) {
		t.Errorf("status.Error = %q, want it to include the underlying failure %q verbatim", status.Error, errBoom.Error())
	}
	if rec.restarts() != 0 {
		t.Errorf("ingress restarts = %d, want 0 (a failed join must never restart ingress)", rec.restarts())
	}
	if len(rec.persistCalls()) != 0 {
		t.Errorf("persist calls = %v, want none (a failed join is never persisted)", rec.persistCalls())
	}
}

func TestRuntimeJoinPersistsOnlyWhenAskedAndOnlySuccess(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}}}
	log := &testLog{}
	deps, rec := runtimeJoinTestDeps(clock, wifi, fakeCredentials{ok: false}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop, RequestDir: dir, RequestPollInterval: time.Millisecond})

	req := wifictl.Request{ID: "req-1", SSID: "guest-net", Persist: false}
	if err := wifictl.WriteRequest(dir, req); err != nil {
		t.Fatal(err)
	}

	awaitJoined(t, clock, wifi, dir, req.ID, 1)
	if calls := rec.persistCalls(); len(calls) != 0 {
		t.Errorf("persist calls = %v, want none: this request never asked to persist", calls)
	}
}

// TestRuntimeJoinServesARequestEvenWithNoBootCredentials guards epic
// decision 5's restructure: the watcher must run — and answer a request —
// even on a board that had no WiFi credentials configured at boot.
func TestRuntimeJoinServesARequestEvenWithNoBootCredentials(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}}}
	log := &testLog{}
	deps, _ := runtimeJoinTestDeps(clock, wifi, fakeCredentials{ok: false}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop, RequestDir: dir, RequestPollInterval: time.Millisecond})

	waitForLog(t, log, "no WiFi credentials configured")

	req := wifictl.Request{ID: "req-1", SSID: "new-network"}
	if err := wifictl.WriteRequest(dir, req); err != nil {
		t.Fatal(err)
	}

	awaitJoined(t, clock, wifi, dir, req.ID, 1)
}

// TestRuntimeJoinFailsHonestlyWithNoWiFiInterface guards epic decision 8: a
// board with no WiFi hardware at all (deps.Wifi nil) must still answer a
// request, not leave the caller to time out.
func TestRuntimeJoinFailsHonestlyWithNoWiFiInterface(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	deps, rec := runtimeJoinTestDeps(clock, nil, fakeCredentials{ok: false}, log)
	deps.Wifi = nil

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop, RequestDir: dir, RequestPollInterval: time.Millisecond})

	req := wifictl.Request{ID: "req-1", SSID: "new-network"}
	if err := wifictl.WriteRequest(dir, req); err != nil {
		t.Fatal(err)
	}

	status := awaitStatus(t, dir, req.ID)
	if status.State != wifictl.Failed || !strings.Contains(status.Error, "no WiFi interface") {
		t.Fatalf("status = %+v, want a Failed status naming \"no WiFi interface\"", status)
	}
	if rec.restarts() != 0 {
		t.Errorf("ingress restarts = %d, want 0", rec.restarts())
	}
}

// TestRuntimeJoinSameSSIDStillFiresIngressRestart guards epic decision 4:
// every successful runtime join restarts ingress, even one that rejoins the
// SSID already in use.
func TestRuntimeJoinSameSSIDStillFiresIngressRestart(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		connectErrs:       []error{nil, nil, errBoom},
	}
	log := &testLog{}
	deps, rec := runtimeJoinTestDeps(clock, wifi, fakeCredentials{ok: false}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop, RequestDir: dir, RequestPollInterval: time.Millisecond})

	first := wifictl.Request{ID: "req-1", SSID: "home-net"}
	if err := wifictl.WriteRequest(dir, first); err != nil {
		t.Fatal(err)
	}
	awaitJoined(t, clock, wifi, dir, first.ID, 1)
	if got := rec.restarts(); got != 1 {
		t.Fatalf("ingress restarts after the first join = %d, want 1", got)
	}

	// Same SSID again: still a fresh restart per decision 4.
	second := wifictl.Request{ID: "req-2", SSID: "home-net"}
	if err := wifictl.WriteRequest(dir, second); err != nil {
		t.Fatal(err)
	}
	awaitJoined(t, clock, wifi, dir, second.ID, 2)
	if got := rec.restarts(); got != 2 {
		t.Fatalf("ingress restarts after the same-SSID rejoin = %d, want 2 (epic decision 4)", got)
	}

	// A third, failing request must not add a third restart.
	third := wifictl.Request{ID: "req-3", SSID: "other-net"}
	if err := wifictl.WriteRequest(dir, third); err != nil {
		t.Fatal(err)
	}
	if status := awaitStatus(t, dir, third.ID); status.State != wifictl.Failed {
		t.Fatalf("third status = %+v, want Failed", status)
	}
	if got := rec.restarts(); got != 2 {
		t.Errorf("ingress restarts after a failed join = %d, want still 2", got)
	}
}

// TestRuntimeJoinUnparseableRequestSelfHeals guards the gosd-6cf2 lesson: a
// request.json that doesn't parse gets one failure report, not one per
// poll, and a later valid request still works.
func TestRuntimeJoinUnparseableRequestSelfHeals(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, wifictl.RequestFile), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}}}
	log := &testLog{}
	deps, _ := runtimeJoinTestDeps(clock, wifi, fakeCredentials{ok: false}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop, RequestDir: dir, RequestPollInterval: time.Millisecond})

	deadline := time.Now().Add(2 * time.Second)
	for !log.contains("could not be read") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !log.contains("could not be read") {
		t.Fatalf("log missing parse-failure notice: %v", log.snapshot())
	}
	status, ok, err := wifictl.ReadStatus(dir)
	if err != nil || !ok || status.State != wifictl.Failed {
		t.Fatalf("ReadStatus() = %+v, %v, %v, want a Failed status", status, ok, err)
	}

	// Give the watcher several more polls against the SAME unparseable
	// content, then confirm it only logged the failure once (self-heal:
	// ignored until the bytes change).
	time.Sleep(20 * time.Millisecond)
	if n := log.count("could not be read"); n != 1 {
		t.Errorf("parse-failure logged %d times against unchanged bytes, want exactly 1", n)
	}

	// Now write a real request: the watcher must recover once the bytes
	// actually change.
	req := wifictl.Request{ID: "req-1", SSID: "recovered-net"}
	if err := wifictl.WriteRequest(dir, req); err != nil {
		t.Fatal(err)
	}
	awaitJoined(t, clock, wifi, dir, req.ID, 1)
}
