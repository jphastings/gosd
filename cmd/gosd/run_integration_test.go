package main

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeQemuBinary writes an executable named qemu-system-aarch64 into a temp
// directory that records the arguments it was invoked with to argsFile and
// exits immediately, then points PATH at that directory for the rest of
// the test. It lets `gosd run`'s build-and-boot pipeline be exercised end
// to end without needing a real qemu-system-aarch64 installed or paying
// for an actual boot.
func fakeQemuBinary(t *testing.T, argsFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake qemu binary is a shell script; gosd's supported CLI hosts are macOS and Linux")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "qemu-system-aarch64")
	contents := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"" + argsFile + "\"\n"
	if err := os.WriteFile(script, []byte(contents), 0o755); err != nil {
		t.Fatalf("writing fake qemu-system-aarch64: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// disableNetwork fails the test the instant a build makes a real network
// request, matching the guard the build integration tests use for
// --artifacts-dir builds.
func disableNetwork(t *testing.T) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir run", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
}

// TestRunFailsActionablyWhenQemuIsNotInstalled is the acceptance test for
// gosd-wnsj's "fail fast with an actionable error" requirement: with no
// qemu-system-aarch64 anywhere on PATH, `gosd run` must refuse before
// spending any time cross-compiling or assembling an image.
func TestRunFailsActionablyWhenQemuIsNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	cmd := newRootCmd()
	cmd.SetArgs([]string{"run", "../../examples/hello"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd run with no qemu-system-aarch64 on PATH succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "qemu-system-aarch64") {
		t.Errorf("error = %q, want it to name the missing binary", err.Error())
	}
}

// TestRunBuildsAQemuVirtImageAndInvokesQemu is gosd-wnsj's core acceptance
// test: `gosd run` cross-compiles the app, assembles a qemu-virt image
// from fake artifacts (no network), and hands it to qemu-system-aarch64
// with the port/memory flags translated into the expected invocation. A
// fake qemu-system-aarch64 stands in for the real one so this stays a fast,
// hermetic `go test` rather than an actual multi-second boot.
func TestRunBuildsAQemuVirtImageAndInvokesQemu(t *testing.T) {
	disableNetwork(t)

	argsFile := filepath.Join(t.TempDir(), "qemu-args.txt")
	fakeQemuBinary(t, argsFile)

	var stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"run", "../../examples/hello",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--hostname", "run-integration-test",
		"--port", "9191",
		"--memory", "256",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd run failed: %v", err)
	}

	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("fake qemu-system-aarch64 was never invoked: %v", err)
	}
	argsLine := string(got)
	for _, want := range []string{"-M virt", "-m 256", "hostfwd=tcp::9191-:80", "hello-qemu-virt.img"} {
		if !strings.Contains(argsLine, want) {
			t.Errorf("qemu invocation = %q, want it to contain %q", argsLine, want)
		}
	}

	if strings.Contains(stderr.String(), "kept build artifacts") {
		t.Errorf("stderr = %q, should not mention kept build artifacts without --keep", stderr.String())
	}
}

// TestRunKeepPreservesTheBuiltImage confirms --keep's documented behavior:
// the temp directory holding the built image survives after qemu exits,
// and gosd run prints its path so a developer can find it.
func TestRunKeepPreservesTheBuiltImage(t *testing.T) {
	disableNetwork(t)

	argsFile := filepath.Join(t.TempDir(), "qemu-args.txt")
	fakeQemuBinary(t, argsFile)

	var stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"run", "../../examples/hello",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--keep",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd run --keep failed: %v", err)
	}

	kept := extractKeptPath(t, stderr.String())
	defer func() { _ = os.RemoveAll(kept) }()

	if info, err := os.Stat(kept); err != nil || !info.IsDir() {
		t.Fatalf("gosd run --keep reported %q as kept, but it's not a directory: %v", kept, err)
	}
	if _, err := os.Stat(filepath.Join(kept, "hello-qemu-virt.img")); err != nil {
		t.Errorf("kept directory is missing the built image: %v", err)
	}
}

func extractKeptPath(t *testing.T, stderr string) string {
	t.Helper()
	const marker = "kept build artifacts at "
	idx := strings.Index(stderr, marker)
	if idx == -1 {
		t.Fatalf("stderr %q doesn't mention kept build artifacts", stderr)
	}
	rest := stderr[idx+len(marker):]
	return strings.TrimSpace(strings.SplitN(rest, "\n", 2)[0])
}
