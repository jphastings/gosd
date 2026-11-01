// Package ingressurl defines where each ingress agent records the public URL
// it serves the app at, and reads it back for the public ingress package —
// the shared contract between writers and reader, so they cannot drift (the
// internal/readymark and internal/wifictl precedent).
//
// Each agent writes its own file, named after its `--ingress` value, because
// the flag is repeatable and both tunnels can run at once; separate files
// stop one overwriting the other. Content is a JSON [Record]: the agent's
// URL, or an "off" reason when gosd-init has decided no tunnel will come up
// this boot. JSON means a torn read is invalid, and a reader that finds a
// file missing, empty or invalid treats it as "not yet", so writers use a
// plain WriteFile with no temp-and-rename step (the internal/wifictl
// precedent).
package ingressurl

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/faultdrop"
)

// Dir is the tmpfs directory the record files live in, under the rest of
// gosd-init's runtime state.
const Dir = faultdrop.Dir + "/ingress"

// CloudflaredPath and TailscaleFunnelPath are where each agent records
// itself, named after the matching `gosd build --ingress` value.
const (
	CloudflaredPath     = Dir + "/cloudflared"
	TailscaleFunnelPath = Dir + "/tailscale-funnel"
)

// Record is what an agent writes: its public URL, or the reason it won't
// report one this boot.
type Record struct {
	URL string `json:"url,omitempty"`
	Off string `json:"off,omitempty"` // non-empty: why this agent won't report a URL this boot
}

// URLRecord encodes a record carrying the agent's public URL.
func URLRecord(u string) []byte { return encode(Record{URL: u}) }

// OffRecord encodes a record saying the agent will not report a URL this
// boot. Write it only where gosd-init's decision is final: an agent that is
// merely retrying (a bad auth key fixed outside the device, say) must leave
// readers waiting.
func OffRecord(reason string) []byte { return encode(Record{Off: reason}) }

func encode(r Record) []byte {
	b, _ := json.Marshal(r) // two string fields cannot fail to marshal
	return b
}

// Read returns the first usable URL an ingress agent has recorded under
// dir, preferring cloudflared over tailscale-funnel: the order is
// deterministic, and a custom domain is the more "real" address.
//
// If no URL is found but every agent's record says it is off, u is nil and
// off holds their reasons in that same order. Anything else — a missing,
// empty or invalid record from any agent — is "not yet": both results are
// nil.
func Read(dir string) (u *url.URL, off []string) {
	allOff := true
	for _, name := range []string{filepath.Base(CloudflaredPath), filepath.Base(TailscaleFunnelPath)} {
		r, ok := readRecord(filepath.Join(dir, name))
		switch {
		case !ok:
			allOff = false
		case r.Off != "":
			off = append(off, r.Off)
		default:
			allOff = false
			if u, err := url.Parse(r.URL); err == nil && u.Scheme != "" && u.Host != "" {
				return u, nil
			}
		}
	}
	if !allOff {
		return nil, nil
	}
	return nil, off
}

func readRecord(path string) (Record, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Record{}, false
	}
	var r Record
	if json.Unmarshal(b, &r) != nil {
		return Record{}, false
	}
	return r, true
}
