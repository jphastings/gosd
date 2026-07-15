package container

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// TestSmoke_DetectAndRun exercises the real Detect + Run path against
// whatever container runtime the machine running this test actually has,
// including a real image pull. It is not part of the fake-driven behavioral
// suite and must never run implicitly: CI's `go test ./...` runs on
// ubuntu-latest images that typically have a live Docker daemon already,
// and this repo's "no build step may require root, Docker, or Linux"
// convention means a container test must not become a silent, flaky
// network dependency of the default test run. It only runs when a
// developer explicitly opts in with GOSD_CONTAINER_SMOKE_TEST=1, and even
// then skips (rather than fails) if no runtime turns out to be available.
func TestSmoke_DetectAndRun(t *testing.T) {
	if os.Getenv("GOSD_CONTAINER_SMOKE_TEST") != "1" {
		t.Skip("set GOSD_CONTAINER_SMOKE_TEST=1 to run the real-daemon smoke test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	rt, err := Detect(ctx, "gosd build-kernel", "")
	if err != nil {
		var notInstalled *NotInstalledError
		var daemonDown *DaemonDownError
		if errors.As(err, &notInstalled) || errors.As(err, &daemonDown) {
			t.Skipf("no live container runtime available: %v", err)
		}
		t.Fatalf("Detect: %v", err)
	}

	var stdout bytes.Buffer
	err = rt.Run(ctx, RunSpec{
		Image:  KernelBuildImage,
		Cmd:    []string{"cat", "/etc/os-release"},
		Stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(stdout.String(), "bookworm") {
		t.Errorf("container output = %q, want it to mention bookworm", stdout.String())
	}
}
