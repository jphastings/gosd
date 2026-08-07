package cloudflared

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// fakeClock mirrors timesync's own fakeClock (duplicated rather than
// shared, per this repo's per-package test-fake convention): a
// manually-advanced Now/After pair so gating and backoff timers can be
// driven deterministically.
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

// waitForPending polls (using real, short sleeps) until clock has at least
// n timers registered, so a test can safely call Advance without racing the
// goroutine under test that's about to call Clock.After.
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

// flag is a thread-safe boolean, used to script deps.NetworkUp/TimeSynced
// results changing mid-test.
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

// waitForLog polls (using real, short sleeps) until log has at least n
// lines containing substr.
func waitForLog(log *testLog, substr string, n int) bool {
	deadline := time.Now().Add(2 * time.Second)
	for log.count(substr) < n {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return true
}

// startCall records one Deps.StartProcess invocation, capturing everything
// it was called with so tests can assert on exact argv/env.
type startCall struct {
	path           string
	args, env      []string
	stdout, stderr io.Writer
}

// fakeProcesses scripts Deps.StartProcess/Deps.Wait: each Start call is
// assigned the next pid (starting at 1), and Wait for that pid blocks on a
// per-pid channel until the test delivers an exit via exit().
type fakeProcesses struct {
	mu       sync.Mutex
	calls    []startCall
	nextPID  int
	exits    map[int]chan waitResult
	startErr error
}

type waitResult struct {
	status int
	err    error
}

func newFakeProcesses() *fakeProcesses {
	return &fakeProcesses{exits: map[int]chan waitResult{}}
}

func (f *fakeProcesses) Start(path string, args, env []string, stdout, stderr io.Writer) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return 0, f.startErr
	}
	f.nextPID++
	pid := f.nextPID
	f.calls = append(f.calls, startCall{path: path, args: append([]string(nil), args...), env: append([]string(nil), env...), stdout: stdout, stderr: stderr})
	f.exits[pid] = make(chan waitResult, 1)
	return pid, nil
}

func (f *fakeProcesses) Wait(pid int) (int, error) {
	f.mu.Lock()
	ch := f.exits[pid]
	f.mu.Unlock()
	r := <-ch
	return r.status, r.err
}

// exit delivers pid's exit status to whatever Wait call is blocked on it.
func (f *fakeProcesses) exit(pid, status int, err error) {
	f.mu.Lock()
	ch := f.exits[pid]
	f.mu.Unlock()
	ch <- waitResult{status: status, err: err}
}

func (f *fakeProcesses) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeProcesses) lastCall() startCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

// waitForStartCount polls (using real, short sleeps) until p has recorded
// at least n Start calls.
func waitForStartCount(p *fakeProcesses, n int) bool {
	deadline := time.Now().Add(2 * time.Second)
	for p.startCount() < n {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return true
}

// fakeFiles records MkdirAll/WriteFile calls instead of touching a real
// filesystem.
type fakeFiles struct {
	mu    sync.Mutex
	dirs  []string
	files map[string][]byte
}

func newFakeFiles() *fakeFiles {
	return &fakeFiles{files: map[string][]byte{}}
}

func (f *fakeFiles) MkdirAll(path string, _ os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dirs = append(f.dirs, path)
	return nil
}

func (f *fakeFiles) WriteFile(path string, data []byte, _ os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := append([]byte(nil), data...)
	f.files[path] = cp
	return nil
}

func (f *fakeFiles) get(path string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.files[path]
	return data, ok
}

func (f *fakeFiles) dirsCreated() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.dirs))
	copy(out, f.dirs)
	return out
}
