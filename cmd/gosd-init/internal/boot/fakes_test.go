package boot

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/redact"
)

// fakeMounter lets tests script Mount/Unmount outcomes and inspect what was
// attempted, without touching any real filesystem.
type fakeMounter struct {
	mu       sync.Mutex
	calls    []mountCall
	unmounts []string
	// fn, if set, determines the result of each Mount call; by default
	// every mount succeeds.
	fn func(call mountCall) error
	// unmountFn, if set, determines the result of each Unmount call; by
	// default every unmount succeeds.
	unmountFn func(target string) error
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

func (m *fakeMounter) Unmount(target string) error {
	m.mu.Lock()
	m.unmounts = append(m.unmounts, target)
	fn := m.unmountFn
	m.mu.Unlock()

	if fn != nil {
		return fn(target)
	}
	return nil
}

func (m *fakeMounter) unmountsFor(target string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, t := range m.unmounts {
		if t == target {
			n++
		}
	}
	return n
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

// fakeRebooter records not just what was called but the order it happened
// in (calls), so a test can assert gosd-fs34's fix directly: FlushConsole
// happened before Reboot/Halt, not merely that both happened somewhere.
type fakeRebooter struct {
	mu        sync.Mutex
	syncCalls int
	rebooted  bool
	halted    bool
	calls     []string
}

func (r *fakeRebooter) Sync() {
	r.mu.Lock()
	r.syncCalls++
	r.calls = append(r.calls, "Sync")
	r.mu.Unlock()
}

func (r *fakeRebooter) FlushConsole() {
	r.mu.Lock()
	r.calls = append(r.calls, "FlushConsole")
	r.mu.Unlock()
}

func (r *fakeRebooter) Reboot() {
	r.mu.Lock()
	r.rebooted = true
	r.calls = append(r.calls, "Reboot")
	r.mu.Unlock()
}

func (r *fakeRebooter) Halt() {
	r.mu.Lock()
	r.halted = true
	r.calls = append(r.calls, "Halt")
	r.mu.Unlock()
}

// callOrder returns the sequence of Rebooter methods invoked, so a test can
// assert one happened before another rather than merely that both happened.
func (r *fakeRebooter) callOrder() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

// assertBefore fails the test unless both first and second appear in order,
// and first's earliest occurrence precedes second's — the gosd-fs34
// ordering guarantee (the console flush happens before the halt/reboot that
// could otherwise cut it off) needs exactly this, not merely proof that both
// eventually happened.
func assertBefore(t *testing.T, order []string, first, second string) {
	t.Helper()
	fi, si := -1, -1
	for i, call := range order {
		if call == first && fi == -1 {
			fi = i
		}
		if call == second && si == -1 {
			si = i
		}
	}
	if fi == -1 {
		t.Errorf("%s was never called (order=%v)", first, order)
		return
	}
	if si == -1 {
		t.Errorf("%s was never called (order=%v)", second, order)
		return
	}
	if fi > si {
		t.Errorf("%s was called after %s, want it first so a halt/reboot can't cut it off (order=%v)", first, second, order)
	}
}

// fakeStatusLED records every state transition Run asks for, in order, so
// tests can assert both that the right ones happened and that a failed one
// never stops Run.
type fakeStatusLED struct {
	mu                               sync.Mutex
	calls                            []string
	bootingErr, runningErr, fatalErr error
}

func (f *fakeStatusLED) Booting() error { return f.call("Booting", f.bootingErr) }
func (f *fakeStatusLED) Running() error { return f.call("Running", f.runningErr) }
func (f *fakeStatusLED) Fatal() error   { return f.call("Fatal", f.fatalErr) }

func (f *fakeStatusLED) call(name string, err error) error {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.mu.Unlock()
	return err
}

func (f *fakeStatusLED) callList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// fakeReaper always reports an immediate, clean exit (status 0, unsignaled).
type fakeReaper struct{}

func (fakeReaper) Wait(pid int) (ExitStatus, error) { return ExitStatus{}, nil }

// funcReaper adapts a plain function to the Reaper interface, for tests that
// need the app to stay "running" until something else has happened.
type funcReaper func(pid int) (ExitStatus, error)

func (f funcReaper) Wait(pid int) (ExitStatus, error) { return f(pid) }

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

// fakeFaultReport stands in for the boot partition a crash report is written
// to: it records what was written and deleted, and pretends the files it has
// been given are present at the partition's root.
type fakeFaultReport struct {
	mu sync.Mutex
	// present is what the card already carries, by file name.
	present map[string]bool
	// writes holds every rendered report, in order.
	writes []string
	// removed holds every set of names deletion was asked for, in order —
	// including, crucially, the fact that it wasn't asked at all.
	removed [][]string
	// writeErr, if set, fails every write.
	writeErr error

	uptime      time.Duration
	uptimeKnown bool
	clockSynced bool
	deviceModel string
	bootCount   int
	// registeredSecrets is read fresh on every RegisteredSecrets() call,
	// so a test can change it between two record() calls to prove a
	// registration made "moments before a crash" is picked up.
	registeredSecrets []redact.Rule
}

func (f *fakeFaultReport) deps() FaultReportDeps {
	return FaultReportDeps{
		Write: func(body string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.writeErr != nil {
				return f.writeErr
			}
			f.writes = append(f.writes, body)
			f.markPresent(true, "LAST_FATAL_ERROR.md")
			return nil
		},
		Exists: func(name string) bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.present[name]
		},
		Remove: func(names []string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.removed = append(f.removed, names)
			f.markPresent(false, names...)
			return nil
		},
		DeviceModel: func() string { return f.deviceModel },
		Uptime:      func() (time.Duration, bool) { return f.uptime, f.uptimeKnown },
		ClockSynced: func() bool { return f.clockSynced },
		CountBoot: func() (int, bool) {
			if f.bootCount == 0 {
				return 0, false
			}
			return f.bootCount, true
		},
		RegisteredSecrets: func() []redact.Rule {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.registeredSecrets
		},
	}
}

// setRegisteredSecrets updates what RegisteredSecrets() returns from here
// on, letting a test change it between two record() calls.
func (f *fakeFaultReport) setRegisteredSecrets(rules []redact.Rule) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registeredSecrets = rules
}

// markPresent updates what the card carries. Callers must hold f.mu.
func (f *fakeFaultReport) markPresent(present bool, names ...string) {
	if f.present == nil {
		f.present = map[string]bool{}
	}
	for _, name := range names {
		f.present[name] = present
	}
}

// written returns the most recent report, or "" if nothing was written.
func (f *fakeFaultReport) written() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) == 0 {
		return ""
	}
	return f.writes[len(f.writes)-1]
}

func (f *fakeFaultReport) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

func (f *fakeFaultReport) removals() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string(nil), f.removed...)
}
