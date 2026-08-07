package container

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
)

// hostEnvVar is the environment variable each engine's own CLI resolves
// before consulting its context (Docker: DOCKER_HOST; Podman's remote
// client: CONTAINER_HOST), so checkLocalEndpoint looks there first.
func hostEnvVar(name string) string {
	if name == RuntimePodman {
		return "CONTAINER_HOST"
	}
	return "DOCKER_HOST"
}

// checkLocalEndpoint fails with *RemoteContextError when the runtime's
// effective daemon endpoint is on another machine. gosd build-kernel and
// gosd build-external bind-mount host paths into the container; a remote
// daemon resolves those mounts on ITS OWN filesystem, so the build sees
// empty directories and fails deep inside the container with no hint the
// daemon was ever the problem (the gotcha CLAUDE.md and docs/custom-kernels.md
// / docs/externals.md's "Supported hosts" sections document). A local
// unix-socket context or DOCKER_HOST — including colima's default — always
// passes.
//
// This only classifies a host string; it never dials the network, so it
// stays cheap to run before every build.
func checkLocalEndpoint(ctx context.Context, ex execRunner, command, name, path string) error {
	envVar := hostEnvVar(name)
	if host, ok := ex.LookupEnv(envVar); ok && host != "" {
		if reason, remote := classifyEndpoint(host); remote {
			return &RemoteContextError{Command: command, Runtime: name, EnvVar: envVar, Endpoint: host, Reason: reason}
		}
		return nil
	}

	ctxName, host, ok := currentContextEndpoint(ctx, ex, path)
	if !ok {
		return nil
	}
	if reason, remote := classifyEndpoint(host); remote {
		return &RemoteContextError{Command: command, Runtime: name, Context: ctxName, Endpoint: host, Reason: reason}
	}
	return nil
}

// currentContextEndpoint runs `<path> context inspect` to find the host
// endpoint of the currently active context. It reports ok=false — never an
// error of its own — whenever the output doesn't look like Docker's context
// JSON: e.g. Podman builds without `context` support, or any other
// unrecognized shape. Classification degrades to "can't tell, assume
// local" rather than blocking a build over a probe this preflight doesn't
// understand.
func currentContextEndpoint(ctx context.Context, ex execRunner, path string) (name, host string, ok bool) {
	var stdout bytes.Buffer
	exitCode, err := ex.Run(ctx, path, []string{"context", "inspect"}, &stdout, io.Discard)
	if err != nil || exitCode != 0 {
		return "", "", false
	}

	var contexts []struct {
		Name      string `json:"Name"`
		Endpoints map[string]struct {
			Host string `json:"Host"`
		} `json:"Endpoints"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &contexts); err != nil || len(contexts) == 0 {
		return "", "", false
	}

	c := contexts[0]
	if ep, ok := c.Endpoints["docker"]; ok && ep.Host != "" {
		return c.Name, ep.Host, true
	}
	for _, ep := range c.Endpoints {
		if ep.Host != "" {
			return c.Name, ep.Host, true
		}
	}
	return "", "", false
}

// classifyEndpoint reports whether a docker/podman host endpoint — using
// DOCKER_HOST/CONTAINER_HOST/context syntax: "unix:///…", "npipe://./pipe/…",
// "tcp://host:port", "ssh://[user@]host[:port]" — points at a daemon
// running somewhere other than this machine. An endpoint this function
// doesn't recognize (no scheme, unparseable, or a scheme other than the
// ones above) is classified local: a false positive would block colima's
// default context, which CLAUDE.md requires to keep working, while a false
// negative merely falls back to today's generic failure.
func classifyEndpoint(host string) (reason string, remote bool) {
	u, err := url.Parse(host)
	if err != nil || u.Scheme == "" {
		return "", false
	}

	switch strings.ToLower(u.Scheme) {
	case "unix", "npipe":
		return "", false
	case "ssh":
		return "an SSH connection", true
	case "tcp", "http", "https":
		if isLoopback(u.Hostname()) {
			return "", false
		}
		return "a non-local TCP address", true
	default:
		return "", false
	}
}

func isLoopback(hostname string) bool {
	switch strings.ToLower(hostname) {
	case "", "localhost":
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}

// RemoteContextError means the runtime's active context, or its
// DOCKER_HOST/CONTAINER_HOST override, points at a daemon on another
// machine. See checkLocalEndpoint's doc comment for why that's fatal for
// gosd build-kernel/build-external specifically.
type RemoteContextError struct {
	// Command names the gosd subcommand that needed a container runtime
	// (see NotInstalledError.Command).
	Command string
	Runtime string
	// EnvVar is the environment variable the remote endpoint came from
	// (e.g. "DOCKER_HOST"), or "" if it came from a context instead.
	EnvVar string
	// Context is the active context's name the remote endpoint came
	// from, or "" if it came from EnvVar instead.
	Context string
	// Endpoint is the raw host string that was classified as remote,
	// e.g. "ssh://user@build-box" or "tcp://10.0.0.5:2375".
	Endpoint string
	// Reason is a short human description of why Endpoint is remote,
	// e.g. "an SSH connection".
	Reason string
}

func (e *RemoteContextError) Error() string {
	var source, fix string
	if e.EnvVar != "" {
		source = fmt.Sprintf("$%s", e.EnvVar)
		fix = fmt.Sprintf("unset %s", e.EnvVar)
	} else {
		source = fmt.Sprintf("context %q", e.Context)
		fix = fmt.Sprintf("`%s context use default` (or another local context)", e.Runtime)
	}
	return fmt.Sprintf(
		"%s needs a container daemon on this machine, but %s's %s points at %s, %s; "+
			"%s bind-mounts local paths into the container, and a remote daemon resolves "+
			"them empty — failing confusingly deep inside the build. %s, or run %s on the "+
			"machine hosting that daemon. See docs/custom-kernels.md and docs/externals.md's "+
			"\"Supported hosts\" sections.",
		e.Command, e.Runtime, source, e.Endpoint, e.Reason,
		e.Command, fix, e.Command,
	)
}
