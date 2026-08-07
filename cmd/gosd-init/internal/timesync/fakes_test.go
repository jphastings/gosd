package timesync

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// fakeClock mirrors netup's own fakeClock (duplicated rather than shared,
// per the convention this repo already follows for per-package test
// fakes): a manually-advanced Now/After pair so the retry/backoff and
// resync timers can be driven deterministically.
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

// waitForPending polls (using real, short sleeps) until clock has at
// least n timers registered, so a test can safely call Advance without
// racing the goroutine under test that's about to call Clock.After.
func waitForPending(clock *fakeClock, n int) bool {
	deadline := time.Now().Add(2 * time.Second)
	for clock.numPending() < n {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return true
}

// ntpResult is one scripted outcome for fakeNTPClient.Query. Most tests
// only care about the corrected time (or an error) and get a
// well-formed SNTPSample built from t for free; the handful exercising
// SNTP response validation (gosd-dqps) set sample to script a specific
// malformed response instead.
type ntpResult struct {
	t      time.Time
	err    error
	sample *SNTPSample
}

// toSample returns the SNTPSample this result reports: the explicit
// override if one was scripted, otherwise a well-formed sample built
// from t (a valid stratum, no leap warning, and t itself as both the
// corrected time and the wire transmit timestamp).
func (r ntpResult) toSample() SNTPSample {
	if r.sample != nil {
		return *r.sample
	}
	return SNTPSample{Time: r.t, Stratum: 2, TransmitTimestamp: r.t}
}

// fakeNTPClient scripts NTP query outcomes per server, so tests can
// exercise retry/backoff, multi-server fallback, and resync
// deterministically.
type fakeNTPClient struct {
	mu      sync.Mutex
	results map[string][]ntpResult
	calls   map[string]int
}

func newFakeNTPClient() *fakeNTPClient {
	return &fakeNTPClient{results: map[string][]ntpResult{}, calls: map[string]int{}}
}

// script sets the sequential results returned for server: the first Query
// call gets results[0], the second results[1], and so on, repeating the
// last entry once exhausted.
func (f *fakeNTPClient) script(server string, results ...ntpResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[server] = results
}

func (f *fakeNTPClient) Query(server string) (SNTPSample, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	i := f.calls[server]
	f.calls[server] = i + 1

	rs := f.results[server]
	if len(rs) == 0 {
		return SNTPSample{}, fmt.Errorf("fakeNTPClient: no result scripted for %s", server)
	}
	if i >= len(rs) {
		i = len(rs) - 1
	}
	if rs[i].err != nil {
		return SNTPSample{}, rs[i].err
	}
	return rs[i].toSample(), nil
}

func (f *fakeNTPClient) callCount(server string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[server]
}

// fakeSystemClock records every time Set was called with.
type fakeSystemClock struct {
	mu  sync.Mutex
	set []time.Time
	err error
}

func (f *fakeSystemClock) Set(t time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set = append(f.set, t)
	return f.err
}

func (f *fakeSystemClock) sets() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]time.Time, len(f.set))
	copy(out, f.set)
	return out
}

// fakeRTC records every time Set was called with, and can be scripted to
// fail — mirroring fakeSystemClock's shape (see its doc): rtcWriteback's
// job (log a write failure at most once, never block the sync loop) is
// symmetric to how stepClock already handles a failed System.Set.
type fakeRTC struct {
	mu  sync.Mutex
	set []time.Time
	err error
}

func (f *fakeRTC) Set(t time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set = append(f.set, t)
	return f.err
}

func (f *fakeRTC) sets() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]time.Time, len(f.set))
	copy(out, f.set)
	return out
}

// jumpingClock is a fakeClock whose Now() actually jumps when paired
// jumpingSystemClock.Set is called — modeling a real wall-clock step,
// the way production's real Clock (plain time.Now()) and SystemClock
// (settimeofday) are tied together by the OS, but the plain
// fakeClock/fakeSystemClock pair above deliberately isn't (see Clock's
// doc comment). guard.go's own doc calls this out as the blind spot that
// let gosd-dqps's field failure mode go unmodeled: every test using the
// decoupled pair passes regardless of whether stepGuard's math would
// have tolerated Now() actually moving at the moment a sync lands, since
// it never does in those tests. Guard/anchor tests that need to rule
// that blind spot out use this pair instead.
type jumpingClock struct {
	*fakeClock
}

func newJumpingClock(start time.Time) *jumpingClock {
	return &jumpingClock{fakeClock: newFakeClock(start)}
}

// jumpTo instantly moves the clock's Now() to t without firing any
// pending timer early — a real settimeofday step doesn't touch
// CLOCK_MONOTONIC, so pending After() timers (armed off the monotonic
// side in production) are unaffected by it either.
func (c *jumpingClock) jumpTo(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

// jumpingSystemClock records Set calls exactly like fakeSystemClock, and
// additionally jumps its paired jumpingClock's Now() to the set value —
// see jumpingClock's doc.
type jumpingSystemClock struct {
	fakeSystemClock
	clock *jumpingClock
}

func (s *jumpingSystemClock) Set(t time.Time) error {
	if err := s.fakeSystemClock.Set(t); err != nil {
		return err
	}
	s.clock.jumpTo(t)
	return nil
}

// flag is a thread-safe boolean, used to script deps.NetworkUp's result
// changing mid-test (the marker file appearing after gosd-init starts
// polling for it).
type flag struct {
	mu sync.Mutex
	v  bool
}

func (f *flag) set(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.v = v
}

func (f *flag) get() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.v
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
	return l.count(substr) > 0
}

// count returns how many logged lines contain substr, so a test can
// synchronize on ("wait until this has happened twice") rather than just
// ("has this happened at all").
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
