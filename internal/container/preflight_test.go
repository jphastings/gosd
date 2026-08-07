package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// contextInspectOutput builds the `docker context inspect` JSON shape for a
// context named name whose docker endpoint is host.
func contextInspectOutput(name, host string) string {
	return fmt.Sprintf(`[{"Name":%q,"Endpoints":{"docker":{"Host":%q}}}]`, name, host)
}

// withDaemonUpAndContext returns a fakeExec runFn where `info` always
// succeeds (the daemon is up) and `context inspect` prints contextJSON.
func withDaemonUpAndContext(contextJSON string) func(string, []string, io.Writer, io.Writer) (int, error) {
	return func(_ string, args []string, stdout, _ io.Writer) (int, error) {
		if len(args) > 0 && args[0] == "context" {
			_, _ = stdout.Write([]byte(contextJSON))
			return 0, nil
		}
		return 0, nil
	}
}

// withDaemonUpNoContextSupport simulates a runtime (e.g. Podman without
// context support) whose `info` succeeds but `context inspect` isn't a
// recognized subcommand.
func withDaemonUpNoContextSupport() func(string, []string, io.Writer, io.Writer) (int, error) {
	return func(_ string, args []string, _, stderr io.Writer) (int, error) {
		if len(args) > 0 && args[0] == "context" {
			_, _ = stderr.Write([]byte("Error: unknown command \"context\""))
			return 1, errors.New("exit status 1")
		}
		return 0, nil
	}
}

func TestDetect_LocalUnixSocketContext_Passes(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimeDocker: "/usr/bin/docker"})
	ex.runFn = withDaemonUpAndContext(contextInspectOutput("colima", "unix:///Users/jp/.colima/default/docker.sock"))

	rt, err := detect(context.Background(), testCommand, "", ex)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if rt.Name() != RuntimeDocker {
		t.Fatalf("Name() = %q, want %q", rt.Name(), RuntimeDocker)
	}
}

func TestDetect_NoContextSupport_AssumesLocal(t *testing.T) {
	// Podman builds without `context` support (or any CLI whose
	// `context inspect` fails) must not be blocked - checkLocalEndpoint
	// degrades to "can't tell" rather than failing the build.
	ex := newFakeExec(map[string]string{RuntimePodman: "/usr/bin/podman"})
	ex.runFn = withDaemonUpNoContextSupport()

	rt, err := detect(context.Background(), testCommand, RuntimePodman, ex)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if rt.Name() != RuntimePodman {
		t.Fatalf("Name() = %q, want %q", rt.Name(), RuntimePodman)
	}
}

func TestDetect_RemoteSSHContext_Fails(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimeDocker: "/usr/bin/docker"})
	ex.runFn = withDaemonUpAndContext(contextInspectOutput("build-box", "ssh://jp@build-box.local"))

	_, err := detect(context.Background(), testCommand, "", ex)

	var remoteErr *RemoteContextError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("detect error = %v (%T), want *RemoteContextError", err, err)
	}
	if remoteErr.Context != "build-box" {
		t.Errorf("Context = %q, want %q", remoteErr.Context, "build-box")
	}
	for _, want := range []string{testCommand, "build-box", "ssh://jp@build-box.local", "docs/custom-kernels.md", "docs/externals.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message %q missing %q", err.Error(), want)
		}
	}
}

func TestDetect_RemoteTCPContext_Fails(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimeDocker: "/usr/bin/docker"})
	ex.runFn = withDaemonUpAndContext(contextInspectOutput("remote-daemon", "tcp://10.0.0.5:2375"))

	_, err := detect(context.Background(), testCommand, "", ex)

	var remoteErr *RemoteContextError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("detect error = %v (%T), want *RemoteContextError", err, err)
	}
}

func TestDetect_DockerHostEnvSSH_FailsWithoutConsultingContext(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimeDocker: "/usr/bin/docker"})
	ex.env = map[string]string{"DOCKER_HOST": "ssh://jp@build-box.local"}
	ex.runFn = func(_ string, args []string, _, _ io.Writer) (int, error) {
		if len(args) > 0 && args[0] == "context" {
			t.Fatal("DOCKER_HOST should short-circuit before context inspect is ever run")
		}
		return 0, nil
	}

	_, err := detect(context.Background(), testCommand, "", ex)

	var remoteErr *RemoteContextError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("detect error = %v (%T), want *RemoteContextError", err, err)
	}
	if remoteErr.EnvVar != "DOCKER_HOST" {
		t.Errorf("EnvVar = %q, want DOCKER_HOST", remoteErr.EnvVar)
	}
	if !strings.Contains(err.Error(), "unset DOCKER_HOST") {
		t.Errorf("error message %q should hint unsetting DOCKER_HOST", err.Error())
	}
}

func TestDetect_DockerHostEnvLoopbackTCP_Passes(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimeDocker: "/usr/bin/docker"})
	ex.env = map[string]string{"DOCKER_HOST": "tcp://127.0.0.1:2375"}
	ex.runFn = healthyDaemon

	if _, err := detect(context.Background(), testCommand, "", ex); err != nil {
		t.Fatalf("detect: %v", err)
	}
}

func TestDetect_PodmanUsesContainerHostNotDockerHost(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimePodman: "/usr/bin/podman"})
	// A stray DOCKER_HOST from an unrelated docker setup must not affect
	// podman: only CONTAINER_HOST does.
	ex.env = map[string]string{"DOCKER_HOST": "ssh://jp@build-box.local"}
	ex.runFn = healthyDaemon

	if _, err := detect(context.Background(), testCommand, RuntimePodman, ex); err != nil {
		t.Fatalf("detect: %v", err)
	}
}

func TestDetect_PodmanContainerHostSSH_Fails(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimePodman: "/usr/bin/podman"})
	ex.env = map[string]string{"CONTAINER_HOST": "ssh://jp@build-box.local"}
	ex.runFn = healthyDaemon

	_, err := detect(context.Background(), testCommand, RuntimePodman, ex)

	var remoteErr *RemoteContextError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("detect error = %v (%T), want *RemoteContextError", err, err)
	}
	if remoteErr.EnvVar != "CONTAINER_HOST" {
		t.Errorf("EnvVar = %q, want CONTAINER_HOST", remoteErr.EnvVar)
	}
}

func TestDetect_MalformedContextOutput_AssumesLocal(t *testing.T) {
	ex := newFakeExec(map[string]string{RuntimeDocker: "/usr/bin/docker"})
	ex.runFn = withDaemonUpAndContext("not valid json")

	if _, err := detect(context.Background(), testCommand, "", ex); err != nil {
		t.Fatalf("detect: %v", err)
	}
}

func TestClassifyEndpoint(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		remote bool
	}{
		{"unix socket", "unix:///var/run/docker.sock", false},
		{"colima unix socket", "unix:///Users/jp/.colima/default/docker.sock", false},
		{"windows npipe", `npipe:////./pipe/docker_engine`, false},
		{"ssh", "ssh://jp@build-box.local", true},
		{"tcp loopback", "tcp://127.0.0.1:2375", false},
		{"tcp IPv6 loopback", "tcp://[::1]:2375", false},
		{"tcp localhost hostname", "tcp://localhost:2375", false},
		{"tcp remote IP", "tcp://10.0.0.5:2375", true},
		{"tcp remote hostname", "tcp://build-box.local:2375", true},
		{"https remote", "https://build-box.example.com:2376", true},
		{"empty string", "", false},
		{"no scheme", "build-box:2375", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, remote := classifyEndpoint(tt.host)
			if remote != tt.remote {
				t.Errorf("classifyEndpoint(%q) remote = %v, want %v", tt.host, remote, tt.remote)
			}
		})
	}
}

func TestRemoteContextError_MessageNamesFixForContextSource(t *testing.T) {
	err := &RemoteContextError{
		Command:  "gosd build-external",
		Runtime:  RuntimeDocker,
		Context:  "build-box",
		Endpoint: "ssh://jp@build-box.local",
		Reason:   "an SSH connection",
	}
	msg := err.Error()
	for _, want := range []string{"gosd build-external", "build-box", "docker context use default", "docs/custom-kernels.md", "docs/externals.md"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}
