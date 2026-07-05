package mdnsresponder

import (
	"fmt"
	"strings"
	"sync"
)

// fakeServer records how many times Close was called, so tests can assert
// Run tears down the previous responder before starting its replacement.
type fakeServer struct {
	mu     sync.Mutex
	closes int
}

func (s *fakeServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closes++
	return nil
}

func (s *fakeServer) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closes
}

// serverResult is one scripted NewServer outcome.
type serverResult struct {
	srv *fakeServer
	err error
}

// fakeNewServer scripts NewServerFunc outcomes per call (in order,
// repeating the last one once exhausted), and records every hostname it was
// called with.
type fakeNewServer struct {
	mu      sync.Mutex
	results []serverResult
	calls   []string
}

func (f *fakeNewServer) script(results ...serverResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = results
}

func (f *fakeNewServer) NewServer(hostname string) (Server, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	i := len(f.calls)
	f.calls = append(f.calls, hostname)

	if len(f.results) == 0 {
		return nil, fmt.Errorf("fakeNewServer: no result scripted for call %d", i)
	}
	if i >= len(f.results) {
		i = len(f.results) - 1
	}
	r := f.results[i]
	if r.err != nil {
		// Return a literal nil Server, not a nil *fakeServer boxed into
		// the interface (which would be a non-nil interface value) —
		// callers that check "err != nil" first never look at this, but
		// keeping it a true nil avoids the classic Go footgun for anyone
		// who later does look.
		return nil, r.err
	}
	return r.srv, nil
}

func (f *fakeNewServer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
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

func (l *testLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.lines))
	copy(out, l.lines)
	return out
}
