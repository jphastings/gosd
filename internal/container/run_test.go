package container

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func testSpec() RunSpec {
	return RunSpec{
		Image: KernelBuildImage,
		Env: map[string]string{
			"ARCH":    "arm64",
			"CC":      "aarch64-linux-gnu-gcc",
			"KBUILD_": "1",
		},
		Mounts: []Mount{
			{HostPath: "/host/src", ContainerPath: "/src", ReadOnly: true},
			{HostPath: "/host/out", ContainerPath: "/out"},
		},
		WorkDir: "/src",
		Cmd:     []string{"make", "-j8", "Image"},
	}
}

func wantArgs() []string {
	return []string{
		"run", "--rm",
		"-e", "ARCH=arm64",
		"-e", "CC=aarch64-linux-gnu-gcc",
		"-e", "KBUILD_=1",
		"-v", "/host/src:/src:ro",
		"-v", "/host/out:/out",
		"-w", "/src",
		KernelBuildImage,
		"make", "-j8", "Image",
	}
}

func TestBuildRunArgs(t *testing.T) {
	got := buildRunArgs(testSpec())
	want := wantArgs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRunArgs() =\n%v\nwant\n%v", got, want)
	}
}

func TestRuntime_Run_AssemblesArgsForDocker(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimeDocker: "/usr/bin/docker"})
	ex.runFn = func(_ string, _ []string, _, _ io.Writer) (int, error) { return 0, nil }
	rt := &Runtime{name: RuntimeDocker, binary: "/usr/bin/docker", exec: ex}

	if err := rt.Run(context.Background(), testSpec()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	call := ex.lastCall()
	if call.path != "/usr/bin/docker" {
		t.Fatalf("path = %q, want /usr/bin/docker", call.path)
	}
	if !reflect.DeepEqual(call.args, wantArgs()) {
		t.Fatalf("args =\n%v\nwant\n%v", call.args, wantArgs())
	}
}

func TestRuntime_Run_AssemblesArgsForPodman(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimePodman: "/usr/bin/podman"})
	ex.runFn = func(_ string, _ []string, _, _ io.Writer) (int, error) { return 0, nil }
	rt := &Runtime{name: RuntimePodman, binary: "/usr/bin/podman", exec: ex}

	if err := rt.Run(context.Background(), testSpec()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	call := ex.lastCall()
	if call.path != "/usr/bin/podman" {
		t.Fatalf("path = %q, want /usr/bin/podman", call.path)
	}
	// Podman's CLI is docker-compatible for these flags, so the
	// argument list is identical to docker's - only the binary differs.
	if !reflect.DeepEqual(call.args, wantArgs()) {
		t.Fatalf("args =\n%v\nwant\n%v", call.args, wantArgs())
	}
}

func TestRuntime_Run_FailureIncludesExitCodeAndStderrTail(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimeDocker: "/usr/bin/docker"})
	ex.runFn = func(_ string, _ []string, _, stderr io.Writer) (int, error) {
		_, _ = stderr.Write([]byte("make: *** [Image] Error 2\n"))
		return 2, errors.New("exit status 2")
	}
	rt := &Runtime{name: RuntimeDocker, binary: "/usr/bin/docker", exec: ex}

	err := rt.Run(context.Background(), testSpec())

	var runFailed *RunFailedError
	if !errors.As(err, &runFailed) {
		t.Fatalf("err = %v (%T), want *RunFailedError", err, err)
	}
	if runFailed.ExitCode != 2 {
		t.Errorf("ExitCode = %d, want 2", runFailed.ExitCode)
	}
	if !strings.Contains(runFailed.StderrTail, "make: *** [Image] Error 2") {
		t.Errorf("StderrTail = %q, missing captured stderr", runFailed.StderrTail)
	}
	if !strings.Contains(err.Error(), "exit code 2") {
		t.Errorf("error message %q missing exit code", err.Error())
	}
}

func TestRuntime_Run_LaunchFailureIsNotARunFailedError(t *testing.T) {
	// exitCode == -1 means the process never started (e.g. the binary
	// vanished between LookPath and Run) - that's not a build failure
	// with a meaningful exit code, so it shouldn't be reported as one.
	ex := newFakeExec(map[string]string{RuntimeDocker: "/usr/bin/docker"})
	launchErr := errors.New("fork/exec: no such file or directory")
	ex.runFn = func(_ string, _ []string, _, _ io.Writer) (int, error) { return -1, launchErr }
	rt := &Runtime{name: RuntimeDocker, binary: "/usr/bin/docker", exec: ex}

	err := rt.Run(context.Background(), testSpec())

	var runFailed *RunFailedError
	if errors.As(err, &runFailed) {
		t.Fatalf("launch failure surfaced as *RunFailedError: %v", err)
	}
	if !errors.Is(err, launchErr) {
		t.Errorf("err = %v, want it to wrap %v", err, launchErr)
	}
}

// syncWriter is an io.Writer that signals a channel after every Write, so a
// test can observe partial output arriving before the producing call
// returns - proving Runtime.Run streams rather than buffers.
type syncWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	wroteFor chan struct{}
}

func newSyncWriter() *syncWriter {
	return &syncWriter{wroteFor: make(chan struct{}, 16)}
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.buf.Write(p)
	w.mu.Unlock()
	w.wroteFor <- struct{}{}
	return n, err
}

func (w *syncWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func TestRuntime_Run_StreamsOutputLive(t *testing.T) {
	stdout := newSyncWriter()
	proceed := make(chan struct{})
	done := make(chan struct{})

	ex := newFakeExec(map[string]string{RuntimeDocker: "/usr/bin/docker"})
	ex.runFn = func(_ string, _ []string, stdout, _ io.Writer) (int, error) {
		_, _ = stdout.Write([]byte("compiling foo.c\n"))
		<-proceed // held open until the test observes the first chunk
		_, _ = stdout.Write([]byte("compiling bar.c\n"))
		return 0, nil
	}
	rt := &Runtime{name: RuntimeDocker, binary: "/usr/bin/docker", exec: ex}

	go func() {
		_ = rt.Run(context.Background(), RunSpec{Image: KernelBuildImage, Stdout: stdout})
		close(done)
	}()

	<-stdout.wroteFor // first chunk has arrived
	if got := stdout.String(); got != "compiling foo.c\n" {
		t.Fatalf("after first write, stdout = %q, want only the first chunk", got)
	}
	select {
	case <-done:
		t.Fatal("Run returned before the second chunk was written - output was buffered, not streamed")
	default:
	}

	close(proceed)
	<-stdout.wroteFor
	<-done
	if got := stdout.String(); got != "compiling foo.c\ncompiling bar.c\n" {
		t.Fatalf("final stdout = %q, want both chunks", got)
	}
}
