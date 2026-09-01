package cloudflared

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a mutex-guarded bytes.Buffer. StartProcess deliberately
// never calls cmd.Wait (that's the reaper's job in production — see
// StartProcess's doc comment), so the child's stdout/stderr can still be
// arriving, written by os/exec's own internal copying goroutine, after
// os.Process.Wait has already returned in this test; a plain bytes.Buffer
// read concurrently with that write would be a data race.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForContent polls (using real, short sleeps) until get() contains
// substr, tolerating the same small gap StartProcess's doc comment
// describes between the child exiting and its output finishing its copy
// into buf.
func waitForContent(get func() string, substr string) bool {
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(get(), substr) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return true
}

// TestStartProcessRunsAndIsolatesEnv exercises the real os/exec-backed
// StartProcess (no fakes): it launches a real child, directs its
// stdout/stderr into buffers, and confirms the child sees exactly the env
// StartProcess was given — never the parent's own environment — which is
// what lets Run's supervise loop rely on runEnv (HOME only) being the
// child's whole environment.
func TestStartProcessRunsAndIsolatesEnv(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on PATH; can't exercise a real child process")
	}

	if err := os.Setenv("GOSD_CLOUDFLARED_TEST_PARENT_ONLY", "leaked-if-inherited"); err != nil {
		t.Fatalf("Setenv: %v", err)
	}
	defer func() { _ = os.Unsetenv("GOSD_CLOUDFLARED_TEST_PARENT_ONLY") }()

	stdout := &syncBuffer{}
	stderr := &syncBuffer{}
	pid, err := StartProcess(
		sh,
		[]string{"-c", `echo "child says: $CHILD_ONLY_VAR"; echo "parent leak: $GOSD_CLOUDFLARED_TEST_PARENT_ONLY"; echo to-stderr 1>&2`},
		[]string{"CHILD_ONLY_VAR=hello-from-start-process"},
		stdout, stderr,
	)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("pid = %d, want > 0", pid)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("FindProcess(%d): %v", pid, err)
	}
	if _, err := proc.Wait(); err != nil {
		t.Fatalf("waiting for the child (pid %d) to exit: %v", pid, err)
	}

	if !waitForContent(stdout.String, "child says: hello-from-start-process") {
		t.Errorf("stdout = %q, want it to contain the env var StartProcess was given", stdout.String())
	}
	if !waitForContent(stdout.String, "parent leak: \n") {
		t.Errorf("stdout = %q, the child saw the parent's own environment; StartProcess must isolate env", stdout.String())
	}
	if !waitForContent(stderr.String, "to-stderr") {
		t.Errorf("stderr = %q, want it to contain the child's stderr output", stderr.String())
	}
}

func TestStartProcessReturnsErrorForMissingBinary(t *testing.T) {
	if _, err := StartProcess("/no/such/binary/gosd-cloudflared-test", nil, nil, &syncBuffer{}, &syncBuffer{}); err == nil {
		t.Fatal("StartProcess succeeded launching a nonexistent binary, want an error")
	}
}

// TestKillTerminatesARealProcess exercises the real, SIGTERM-backed Kill (no
// fakes): a long-sleeping child, sent SIGTERM via Kill, must exit promptly
// rather than run to completion.
func TestKillTerminatesARealProcess(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on PATH; can't exercise a real child process")
	}

	pid, err := StartProcess(sh, []string{"-c", "trap 'exit 0' TERM; sleep 30 & wait"}, nil, &syncBuffer{}, &syncBuffer{})
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}

	if err := Kill(pid); err != nil {
		t.Fatalf("Kill(%d): %v", pid, err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("FindProcess(%d): %v", pid, err)
	}
	done := make(chan struct{})
	go func() { _, _ = proc.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("child did not exit within 5s of Kill; SIGTERM was not delivered")
	}
}

func TestKillReturnsErrorForNoSuchProcess(t *testing.T) {
	// A pid that has already exited and been reaped: os.FindProcess itself
	// always succeeds on Unix, so the failure surfaces from Signal.
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Skipf("could not run a throwaway process to obtain a dead pid: %v", err)
	}
	if err := Kill(cmd.Process.Pid); err == nil {
		t.Fatal("Kill succeeded against an already-reaped pid, want an error")
	}
}
