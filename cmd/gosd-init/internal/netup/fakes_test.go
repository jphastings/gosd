package netup

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock provides a manually-advanced Now/After pair, modeled on
// boot's fakeClock but channel-based (rather than a blocking Sleep) since
// netup's state machine needs to select between a timer and other events
// (context cancellation, link-down) rather than sleep sequentially.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time

	// pending are the timers currently waiting to fire, keyed by the
	// absolute time they were scheduled for.
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

// After returns a channel that fires once advance moves the clock's time
// to at least now+d. A zero or negative d fires immediately.
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

// numPending reports how many timers are currently registered and
// waiting to fire. Tests poll this (rather than racing Advance against
// whichever goroutine is about to call After) to know when it's safe to
// advance the clock.
func (c *fakeClock) numPending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

// Advance moves the clock forward by d, firing (in order) every pending
// timer whose deadline has now been reached.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	var remaining []*fakeTimer
	var fire []*fakeTimer
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

// fakeLinks records SetUp/AddAddr/FlushAddrs/ReplaceDefaultRoute calls and
// lets tests drive Watch's event stream manually. AddAddr and FlushAddrs
// model the real netlink semantics they stand in for (see platform_linux.go):
// AddAddr (AddrReplace) replaces an existing address only if it's identical
// (same IP and mask) and otherwise adds alongside it — so an interface can
// accumulate more than one address here, exactly as it can on a real
// netlink.AddrReplace call — and FlushAddrs (AddrList + AddrDel) empties an
// interface's address list entirely, without touching any other interface's.
type fakeLinks struct {
	mu sync.Mutex

	setUp      []string
	addrs      map[string][]net.IPNet
	routes     map[string]net.IP
	flushCalls map[string]int
	events     chan LinkEvent
	watchErr   error
}

func newFakeLinks() *fakeLinks {
	return &fakeLinks{
		addrs:      make(map[string][]net.IPNet),
		routes:     make(map[string]net.IP),
		flushCalls: make(map[string]int),
		events:     make(chan LinkEvent, 16),
	}
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
	for i, existing := range l.addrs[name] {
		if existing.String() == addr.String() {
			l.addrs[name][i] = addr
			return nil
		}
	}
	l.addrs[name] = append(l.addrs[name], addr)
	return nil
}

func (l *fakeLinks) FlushAddrs(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.addrs, name)
	l.flushCalls[name]++
	return nil
}

func (l *fakeLinks) ReplaceDefaultRoute(name string, gw net.IP) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.routes[name] = gw
	return nil
}

func (l *fakeLinks) Watch(stop <-chan struct{}) (<-chan LinkEvent, error) {
	if l.watchErr != nil {
		return nil, l.watchErr
	}
	return l.events, nil
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

// addrFor returns the most recently applied address for name, for tests
// that only care about the latest lease. ok is false once FlushAddrs has
// emptied name's address list (or nothing was ever assigned).
func (l *fakeLinks) addrFor(name string) (net.IPNet, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	addrs := l.addrs[name]
	if len(addrs) == 0 {
		return net.IPNet{}, false
	}
	return addrs[len(addrs)-1], true
}

// addrsFor returns every address currently assigned to name, so tests can
// assert an interface doesn't accumulate stale addresses across a replug
// or a lease change.
func (l *fakeLinks) addrsFor(name string) []net.IPNet {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]net.IPNet, len(l.addrs[name]))
	copy(out, l.addrs[name])
	return out
}

func (l *fakeLinks) flushCountFor(name string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.flushCalls[name]
}

func (l *fakeLinks) routeFor(name string) (net.IP, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	gw, ok := l.routes[name]
	return gw, ok
}

// fakeDHCP scripts DHCP Request/Renew outcomes per call, so tests can
// exercise retry/backoff and renewal/rebind failure paths deterministically.
type fakeDHCP struct {
	mu sync.Mutex

	requestResults []requestResult
	requestCalls   int

	renewFn    func(iface string, lease *Lease, call int) (*Lease, error)
	renewCalls int
}

type requestResult struct {
	lease *Lease
	err   error
}

func (d *fakeDHCP) Request(ctx context.Context, iface string) (*Lease, error) {
	d.mu.Lock()
	i := d.requestCalls
	d.requestCalls++
	var r requestResult
	if i < len(d.requestResults) {
		r = d.requestResults[i]
	} else if len(d.requestResults) > 0 {
		r = d.requestResults[len(d.requestResults)-1]
	} else {
		r = requestResult{err: fmt.Errorf("fakeDHCP: no result scripted for call %d", i)}
	}
	d.mu.Unlock()
	return r.lease, r.err
}

func (d *fakeDHCP) Renew(ctx context.Context, iface string, lease *Lease) (*Lease, error) {
	d.mu.Lock()
	call := d.renewCalls
	d.renewCalls++
	fn := d.renewFn
	d.mu.Unlock()
	if fn == nil {
		return lease, nil
	}
	return fn(iface, lease, call)
}

func (d *fakeDHCP) requestCallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.requestCalls
}

func (d *fakeDHCP) renewCallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.renewCalls
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

// snapshot returns a copy of the log lines recorded so far, safe to read
// (e.g. in a test failure message) even while another goroutine may still
// be logging.
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

// noJitterBackoff returns a Backoff whose Next() is deterministic (no
// randomness), for tests that assert on exact delays.
func noJitterBackoff(base, max time.Duration) *Backoff {
	b := NewBackoff(base, max)
	b.random = func() float64 { return 1 }
	return b
}

// counter is a thread-safe call counter, used by tests that need to know
// how many times a callback fired from a goroutine other than the test's.
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

// leaseLog thread-safely records the leases onLease was called with, from
// whichever goroutine RunDHCP happens to invoke the callback on.
type leaseLog struct {
	mu  sync.Mutex
	got []*Lease
}

func (l *leaseLog) add(lease *Lease) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.got = append(l.got, lease)
}

func (l *leaseLog) snapshot() []*Lease {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*Lease, len(l.got))
	copy(out, l.got)
	return out
}

func (l *leaseLog) len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.got)
}

// leaseBox thread-safely holds a single lease written from one goroutine
// and read from another.
type leaseBox struct {
	mu sync.Mutex
	v  *Lease
}

func (b *leaseBox) set(l *Lease) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.v = l
}

func (b *leaseBox) get() *Lease {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.v
}

// waitForPending polls (using real, short sleeps) until clock has at
// least n timers registered, so a test can safely call Advance without
// racing the goroutine under test that's about to call Clock.After.
func waitForPending(t *testing.T, clock *fakeClock, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for clock.numPending() < n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d pending timer(s), got %d", n, clock.numPending())
		}
		time.Sleep(time.Millisecond)
	}
}

// count reports how many recorded log lines contain substr, so a test can
// assert that a repeating condition is reported without being repeated.
func (l *testLog) count(substr string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, line := range l.lines {
		if strings.Contains(line, substr) {
			n++
		}
	}
	return n
}

// waitForRenews polls (using real, short sleeps) until dhcp has recorded
// at least n Renew calls, the counterpart to waitForPending for tests that
// advance a fake clock and then need the goroutine under test to have
// acted on it.
func waitForRenews(t *testing.T, dhcp *fakeDHCP, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for dhcp.renewCallCount() < n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d Renew call(s), got %d", n, dhcp.renewCallCount())
		}
		time.Sleep(time.Millisecond)
	}
}

// nextDelay returns how much longer the one pending timer has to run, so
// a test can assert on the delay the code under test actually asked the
// clock for — how soon it intends to act, which advancing the clock and
// watching for the effect can only ever answer racily — and then advance
// exactly that far.
func (c *fakeClock) nextDelay(t *testing.T) time.Duration {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) != 1 {
		t.Fatalf("want exactly one pending timer to read a delay from, got %d", len(c.pending))
	}
	return c.pending[0].at.Sub(c.now)
}
