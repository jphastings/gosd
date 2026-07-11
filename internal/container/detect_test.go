package container

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func healthyDaemon(_ string, _ []string, _, _ io.Writer) (int, error) {
	return 0, nil
}

func TestDetect_ExplicitPreferenceHonored(t *testing.T) {
	ex := newFakeExec(map[string]string{
		RuntimeDocker: "/usr/bin/docker",
		RuntimePodman: "/usr/bin/podman",
	})
	ex.runFn = healthyDaemon

	rt, err := detect(context.Background(), RuntimePodman, ex)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if rt.Name() != RuntimePodman {
		t.Fatalf("Name() = %q, want %q", rt.Name(), RuntimePodman)
	}
	if ex.calledWith("/usr/bin/docker") {
		t.Fatal("explicit preference for podman should never have consulted docker")
	}
}

func TestDetect_AutoDetectPrefersDockerOverPodman(t *testing.T) {
	ex := newFakeExec(map[string]string{
		RuntimeDocker: "/usr/bin/docker",
		RuntimePodman: "/usr/bin/podman",
	})
	ex.runFn = healthyDaemon

	rt, err := detect(context.Background(), "", ex)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if rt.Name() != RuntimeDocker {
		t.Fatalf("Name() = %q, want %q", rt.Name(), RuntimeDocker)
	}
}

func TestDetect_PodmanPickedWhenDockerAbsent(t *testing.T) {
	ex := newFakeExec(map[string]string{
		RuntimePodman: "/usr/bin/podman",
	})
	ex.runFn = healthyDaemon

	rt, err := detect(context.Background(), "", ex)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if rt.Name() != RuntimePodman {
		t.Fatalf("Name() = %q, want %q", rt.Name(), RuntimePodman)
	}
}

func TestDetect_NotInstalled_AutoDetect(t *testing.T) {
	ex := newFakeExec(nil)

	_, err := detect(context.Background(), "", ex)

	var notInstalled *NotInstalledError
	if !errors.As(err, &notInstalled) {
		t.Fatalf("detect error = %v (%T), want *NotInstalledError", err, err)
	}
	if notInstalled.Preferred != "" {
		t.Fatalf("Preferred = %q, want empty for auto-detect", notInstalled.Preferred)
	}
	for _, want := range []string{"Docker", "Podman", "gosd build-kernel"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message %q missing %q", err.Error(), want)
		}
	}
}

func TestDetect_NotInstalled_ExplicitPreference(t *testing.T) {
	ex := newFakeExec(map[string]string{
		RuntimePodman: "/usr/bin/podman",
	})

	_, err := detect(context.Background(), RuntimeDocker, ex)

	var notInstalled *NotInstalledError
	if !errors.As(err, &notInstalled) {
		t.Fatalf("detect error = %v (%T), want *NotInstalledError", err, err)
	}
	if notInstalled.Preferred != RuntimeDocker {
		t.Fatalf("Preferred = %q, want %q", notInstalled.Preferred, RuntimeDocker)
	}
	if strings.Contains(err.Error(), "Podman") {
		t.Errorf("explicit docker preference error shouldn't mention Podman: %q", err.Error())
	}
}

func TestDetect_DaemonDown(t *testing.T) {
	ex := newFakeExec(map[string]string{
		RuntimeDocker: "/usr/bin/docker",
	})
	ex.runFn = func(_ string, _ []string, _, _ io.Writer) (int, error) {
		return 1, errDaemonUnreachable
	}

	_, err := detect(context.Background(), "", ex)

	var daemonDown *DaemonDownError
	if !errors.As(err, &daemonDown) {
		t.Fatalf("detect error = %v (%T), want *DaemonDownError", err, err)
	}
	if daemonDown.Runtime != RuntimeDocker {
		t.Fatalf("Runtime = %q, want %q", daemonDown.Runtime, RuntimeDocker)
	}
	if !errors.Is(err, errDaemonUnreachable) {
		t.Error("DaemonDownError should unwrap to the underlying liveness-check error")
	}
	if !strings.Contains(err.Error(), "start") {
		t.Errorf("daemon-down error should hint how to start it: %q", err.Error())
	}
}

func TestDetect_DaemonDownAndNotInstalled_AreDistinctTypes(t *testing.T) {
	// A caller (e.g. deciding whether to print an install link vs a
	// "start your daemon" hint) must be able to tell these apart.
	notInstalledErr := &NotInstalledError{}
	daemonDownErr := &DaemonDownError{Runtime: RuntimeDocker}

	var asNotInstalled *NotInstalledError
	if errors.As(error(daemonDownErr), &asNotInstalled) {
		t.Fatal("*DaemonDownError must not satisfy errors.As(*NotInstalledError)")
	}
	var asDaemonDown *DaemonDownError
	if errors.As(error(notInstalledErr), &asDaemonDown) {
		t.Fatal("*NotInstalledError must not satisfy errors.As(*DaemonDownError)")
	}
}
