// Package ingressurl defines where each ingress agent records the public URL
// it serves the app at, and reads it back for the public ingress package —
// the shared contract between writers and reader, so they cannot drift (the
// internal/readymark and internal/wifictl precedent).
//
// Each agent writes its own file, named after its `--ingress` value, because
// the flag is repeatable and both tunnels can run at once; separate files
// stop one overwriting the other. Content is the bare URL. A reader that
// finds a file missing, empty or unparseable treats it as "not yet", so
// writers use a plain WriteFile with no temp-and-rename step.
package ingressurl

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jphastings/gosd/internal/faultdrop"
)

// Dir is the tmpfs directory the URL files live in, under the rest of
// gosd-init's runtime state.
const Dir = faultdrop.Dir + "/ingress"

// CloudflaredPath and TailscaleFunnelPath are where each agent records its
// URL, named after the matching `gosd build --ingress` value.
const (
	CloudflaredPath     = Dir + "/cloudflared"
	TailscaleFunnelPath = Dir + "/tailscale-funnel"
)

// Read returns the URL an ingress agent has recorded under dir, preferring
// cloudflared over tailscale-funnel: the order is deterministic, and a
// custom domain is the more "real" address. ok is false while no agent has
// recorded a usable URL yet.
func Read(dir string) (u *url.URL, ok bool) {
	for _, name := range []string{filepath.Base(CloudflaredPath), filepath.Base(TailscaleFunnelPath)} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		u, err := url.Parse(strings.TrimSpace(string(b)))
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		return u, true
	}
	return nil, false
}
