package boot

import (
	"io"
	"sync"
	"time"
)

// fakeMounter lets tests script Mount outcomes and inspect what was
// attempted, without touching any real filesystem.
type fakeMounter struct {
	mu    sync.Mutex
	calls []mountCall
	// fn, if set, determines the result of each Mount call; by default
	// every mount succeeds.
	fn func(call mountCall) error
}

type mountCall struct {
	source, target, fstype string
	flags                  uintptr
	data                   string
}

func (m *fakeMounter) Mount(source, target, fstype string, flags uintptr, data string) error {
	call := mountCall{source: source, target: target, fstype: fstype, flags: flags, data: data}

	m.mu.Lock()
	m.calls = append(m.calls, call)
	fn := m.fn
	m.mu.Unlock()

	if fn != nil {
		return fn(call)
	}
	return nil
}

func (m *fakeMounter) callsFor(target string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.calls {
		if c.target == target {
			n++
		}
	}
	return n
}

// recordedCalls returns every mount call attempted against target, in order.
func (m *fakeMounter) recordedCalls(target string) []mountCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []mountCall
	for _, c := range m.calls {
		if c.target == target {
			out = append(out, c)
		}
	}
	return out
}

// fakeClock provides a manually-advanced Now/Sleep pair, so tests
// involving retry timeouts run instantly and deterministically.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type fakeHostname struct {
	mu  sync.Mutex
	set []string
	err error
}

func (h *fakeHostname) SetHostname(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.set = append(h.set, name)
	return h.err
}

type fakeRebooter struct {
	mu        sync.Mutex
	syncCalls int
	rebooted  bool
}

func (r *fakeRebooter) Sync() {
	r.mu.Lock()
	r.syncCalls++
	r.mu.Unlock()
}

func (r *fakeRebooter) Reboot() {
	r.mu.Lock()
	r.rebooted = true
	r.mu.Unlock()
}

// fakeReaper always reports an immediate, successful exit.
type fakeReaper struct{}

func (fakeReaper) Wait(pid int) (int, error) { return 0, nil }

// funcAppStarter adapts a plain function to the AppStarter interface, for
// tests that need custom start behavior (like stopping supervision after N
// restarts) beyond what fakeAppStarter offers.
type funcAppStarter func(path string, env []string, stdout, stderr io.Writer) (int, error)

func (f funcAppStarter) Start(path string, env []string, stdout, stderr io.Writer) (int, error) {
	return f(path, env, stdout, stderr)
}

// nopWriteCloser adapts an io.Writer (like a *bytes.Buffer) to
// io.WriteCloser for tests that need to open a fake console.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
