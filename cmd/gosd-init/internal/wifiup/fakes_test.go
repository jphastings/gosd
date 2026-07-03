package wifiup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/netup"
)

var errBoom = errors.New("boom")

// fakeClock mirrors netup's own fakeClock (duplicated rather than shared,
// per the convention this repo already follows for per-package test
// fakes): a manually-advanced Now/After pair so the retry/backoff and
// reconnect timers can be driven deterministically.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time

	pending []*fakeTimer
}

type fakeTimer struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan time.Time, 1)
	at := c.now.Add(d)
	if !at.After(c.now) {
		ch <- at
		return ch
	}
	c.pending = append(c.pending, &fakeTimer{at: at, ch: ch})
	return ch
}

func (c *fakeClock) numPending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

// Advance moves the clock forward by d, firing every pending timer whose
// deadline has now been reached.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	var remaining, fire []*fakeTimer
	for _, t := range c.pending {
		if !t.at.After(now) {
			fire = append(fire, t)
		} else {
			remaining = append(remaining, t)
		}
	}
	c.pending = remaining
	c.mu.Unlock()

	for _, t := range fire {
		t.ch <- t.at
	}
}

// waitForPending polls until clock has at least n timers registered, so a
// test can safely call Advance without racing the goroutine under test
// that's about to call Clock.After.
func waitForPending(deadline time.Duration, clock *fakeClock, n int) bool {
	end := time.Now().Add(deadline)
	for clock.numPending() < n {
		if time.Now().After(end) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return true
}

// testLog collects log lines for assertions instead of printing them.
type testLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *testLog) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *testLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.lines))
	copy(out, l.lines)
	return out
}

func (l *testLog) contains(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.lines {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

// counter is a thread-safe call counter.
type counter struct {
	mu sync.Mutex
	n  int
}

func (c *counter) inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func (c *counter) load() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// fakeWifiClient scripts nl80211 outcomes so wifiup's association and
// reconnect state machine can be exercised deterministically.
type fakeWifiClient struct {
	mu sync.Mutex

	// interfacesResults is polled in order (repeating the last entry)
	// each time Interfaces is called, so tests can simulate the wlan
	// interface not existing yet at boot.
	interfacesResults [][]Interface
	interfacesCalls   int

	connectErr      error
	connectCalls    []string // ssids
	connectPSKErr   error
	connectPSKCalls []connectPSKCall

	// associatedResults is polled in order (repeating the last entry)
	// each time Associated is called.
	associatedResults []bool
	associatedCalls   int

	disconnectCalls int
}

type connectPSKCall struct {
	ssid string
	psk  [32]byte
}

func (f *fakeWifiClient) Interfaces() ([]Interface, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.interfacesCalls
	f.interfacesCalls++
	if len(f.interfacesResults) == 0 {
		return nil, nil
	}
	if i >= len(f.interfacesResults) {
		i = len(f.interfacesResults) - 1
	}
	return f.interfacesResults[i], nil
}

func (f *fakeWifiClient) Connect(_ Interface, ssid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connectCalls = append(f.connectCalls, ssid)
	return f.connectErr
}

func (f *fakeWifiClient) ConnectPSK(_ Interface, ssid string, psk [32]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connectPSKCalls = append(f.connectPSKCalls, connectPSKCall{ssid: ssid, psk: psk})
	return f.connectPSKErr
}

func (f *fakeWifiClient) Disconnect(Interface) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disconnectCalls++
	return nil
}

func (f *fakeWifiClient) Associated(Interface) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.associatedCalls
	f.associatedCalls++
	if len(f.associatedResults) == 0 {
		return true, nil
	}
	if i >= len(f.associatedResults) {
		i = len(f.associatedResults) - 1
	}
	return f.associatedResults[i], nil
}

func (f *fakeWifiClient) associatedCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.associatedCalls
}

func (f *fakeWifiClient) connectCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.connectCalls)
}

func (f *fakeWifiClient) connectPSKCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.connectPSKCalls)
}

func (f *fakeWifiClient) lastConnectPSK() (connectPSKCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.connectPSKCalls) == 0 {
		return connectPSKCall{}, false
	}
	return f.connectPSKCalls[len(f.connectPSKCalls)-1], true
}

// fakeCredentials returns a fixed Credentials/ok/err triple.
type fakeCredentials struct {
	creds Credentials
	ok    bool
	err   error
}

func (f fakeCredentials) Credentials() (Credentials, bool, error) {
	return f.creds, f.ok, f.err
}

// fakeLinks records SetUp/AddAddr/ReplaceDefaultRoute calls; Watch is
// never used by wifiup so it's not implemented here.
type fakeLinks struct {
	mu sync.Mutex

	setUp  []string
	addrs  map[string]net.IPNet
	routes map[string]net.IP
}

func newFakeLinks() *fakeLinks {
	return &fakeLinks{addrs: map[string]net.IPNet{}, routes: map[string]net.IP{}}
}

func (l *fakeLinks) SetUp(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.setUp = append(l.setUp, name)
	return nil
}

func (l *fakeLinks) AddAddr(name string, addr net.IPNet) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.addrs[name] = addr
	return nil
}

func (l *fakeLinks) ReplaceDefaultRoute(name string, gw net.IP) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.routes[name] = gw
	return nil
}

func (l *fakeLinks) Watch(<-chan struct{}) (<-chan netup.LinkEvent, error) {
	return nil, fmt.Errorf("fakeLinks: Watch not used by wifiup")
}

func (l *fakeLinks) sawSetUp(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, n := range l.setUp {
		if n == name {
			return true
		}
	}
	return false
}

func (l *fakeLinks) addrFor(name string) (net.IPNet, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.addrs[name]
	return a, ok
}

// fakeDHCP scripts DHCP Request/Renew outcomes; wifiup only depends on
// netup.DHCPClient's shape (see netup.RunDHCP), so this mirrors netup's
// own fakeDHCP but is kept minimal to what these tests exercise.
type fakeDHCP struct {
	mu sync.Mutex

	requestResults []requestResult
	requestCalls   int
}

type requestResult struct {
	lease *netup.Lease
	err   error
}

func (d *fakeDHCP) Request(_ context.Context, _ string) (*netup.Lease, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := d.requestCalls
	d.requestCalls++
	if len(d.requestResults) == 0 {
		return nil, fmt.Errorf("fakeDHCP: no result scripted for call %d", i)
	}
	if i >= len(d.requestResults) {
		i = len(d.requestResults) - 1
	}
	return d.requestResults[i].lease, d.requestResults[i].err
}

func (d *fakeDHCP) Renew(_ context.Context, _ string, lease *netup.Lease) (*netup.Lease, error) {
	return lease, nil
}

func (d *fakeDHCP) requestCallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.requestCalls
}

// backoffFactory returns a NewBackoff func for tests. netup.Backoff's
// jitter source is unexported, so unlike netup's own tests (which are in
// the same package and can zero it out), these tests can't get a fully
// deterministic delay; instead, waitAndAdvancePast advances the fake
// clock by max, which always exceeds any jittered delay Backoff.Next can
// produce and so reliably fires the pending timer regardless of jitter.
func backoffFactory(base, max time.Duration) func() *netup.Backoff {
	return func() *netup.Backoff { return netup.NewBackoff(base, max) }
}

// waitAndAdvancePast waits for a timer to be pending and then advances
// clock by max (see backoffFactory), guaranteeing it fires regardless of
// the actual jittered delay chosen.
func waitAndAdvancePast(clock *fakeClock, max time.Duration) bool {
	if !waitForPending(2*time.Second, clock, 1) {
		return false
	}
	clock.Advance(max)
	return true
}

// advanceUntil repeatedly advances clock by step (e.g.
// associationPollPeriod) until condition is true or the deadline passes.
// Used instead of a single wait-then-advance when another, much longer
// timer (e.g. a DHCP lease renewal wait) may also be pending concurrently
// and would make a pending-timer-count-based wait racy: repeatedly
// advancing by a small step only ever fires the short-period timer,
// leaving a much longer one untouched.
func advanceUntil(clock *fakeClock, step time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			return false
		}
		clock.Advance(step)
		time.Sleep(time.Millisecond)
	}
	return true
}
