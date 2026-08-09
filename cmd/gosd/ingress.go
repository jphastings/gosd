package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/cacerts"
	"github.com/jphastings/gosd/internal/cloudflaredpin"
	"github.com/jphastings/gosd/internal/fetch"
	"github.com/jphastings/gosd/internal/staticelf"
)

// ingressCloudflaredValue is cloudflared's --ingress value (epic gosd-virc,
// v1: a locally-managed Cloudflare Tunnel only).
const ingressCloudflaredValue = "cloudflared"

// ingressCloudflaredDest is where gosd build/run --ingress cloudflared
// embeds the cloudflared binary inside the initramfs.
const ingressCloudflaredDest = "/bin/cloudflared"

// ingressTailscaleFunnelValue is the tsnet Funnel shim's --ingress value
// (epic gosd-65uy), matching gosdtoml's [ingress.tailscale-funnel] table
// name exactly.
const ingressTailscaleFunnelValue = "tailscale-funnel"

// ingressTailscaleFunnelDest is where gosd build/run --ingress
// tailscale-funnel embeds the per-arch-compiled cmd/gosd-tsfunnel shim
// inside the initramfs (bean gosd-kzd3).
const ingressTailscaleFunnelDest = "/bin/gosd-tsfunnel"

// ingressAgent is one entry in gosd's registry of --ingress values it
// understands: the name accepted on the command line, the board rule it
// needs (which GOARCHes it can run on, with the exact refusal wording for
// the ones it can't - see validateIngress), and the initramfs dest(s) it
// reserves from --with-external (see reservedExternalDests). Adding a
// second ingress agent (epic gosd-65uy) means adding one row here, not
// touching parse, validate, or collision-check logic; that agent's own
// resolve/open functions (mirroring resolveIngressCloudflared/
// openIngressCloudflaredForBoard, or - for tailscale-funnel, which is
// per-arch COMPILED rather than fetched - compileForBoards' third binary
// pass, see archbuild.go) are wired into cmd/gosd's build/run commands by
// its own bean, the same way cloudflared's are today.
type ingressAgent struct {
	name string

	// capableGOARCH reports whether this agent can run on goarch. ok=false
	// is paired with the exact refusal reason to show the operator.
	capableGOARCH func(goarch string) (ok bool, reason string)

	// reservedDests are the initramfs paths this agent reserves from
	// --with-external, paired with the description shown in a collision
	// error (see reservedExternalDests).
	reservedDests map[string]string

	// requiresDataPartition, when non-empty, is the reason this agent needs
	// a writable data partition: validateIngressDataPartition refuses a
	// build that selects this agent with no --data-size (dataSizeBytes==0
	// and dataExpand false). Empty means the agent has no such requirement
	// (cloudflared's tunnel credentials are stateless from gosd's point of
	// view - see gosdtoml.IngressCloudflared).
	requiresDataPartition string
}

// cloudflaredAgent is gosd's original --ingress value (epic gosd-virc, v1).
var cloudflaredAgent = ingressAgent{
	name: ingressCloudflaredValue,
	capableGOARCH: func(goarch string) (bool, string) {
		if _, ok := cloudflaredpin.ByGOARCH[goarch]; ok {
			return true, ""
		}
		reason := fmt.Sprintf("no cloudflared build is pinned for GOARCH=%s", goarch)
		if goarch == "arm" {
			reason = "cloudflared's official arm release is built for GOARM=7 and faults with \"illegal instruction\" on this board's armv6 CPU"
		}
		return false, reason
	},
	// /bin/cloudflared and the CA bundle's path are reserved unconditionally
	// (bean gosd-g4km), not just on builds that actually pass --ingress
	// cloudflared: the CA bundle ships in EVERY image regardless (bean
	// gosd-kzgq), and reserving cloudflared's dest eagerly means adding
	// --ingress to an existing --with-external build can never
	// retroactively break it by surprise.
	reservedDests: map[string]string{
		ingressCloudflaredDest: "gosd's --ingress cloudflared binary",
		cacerts.InitramfsPath:  "gosd's baked CA certificate bundle",
	},
}

// tailscaleFunnelAgent is the second --ingress value (epic gosd-65uy):
// gosd's own tsnet-based Funnel shim, compiled per-arch from gosd's own
// source (bean gosd-kzd3, internal/build's CrossCompileTsfunnel) rather than
// downloaded, so - unlike cloudflared, which is arm64-only because
// upstream's arm release targets GOARM=7 - EVERY board's GOARCH is capable:
// the epic's Funnel facts confirmed GOARM=6 self-compiles and runs, covering
// pi-zero-w too.
var tailscaleFunnelAgent = ingressAgent{
	name: ingressTailscaleFunnelValue,
	capableGOARCH: func(_ string) (bool, string) {
		return true, ""
	},
	reservedDests: map[string]string{
		ingressTailscaleFunnelDest: "gosd's --ingress tailscale-funnel shim binary",
	},
	// The shim's tailnet node identity lives in tsnet's state dir under
	// /data (epic gosd-65uy decision 3): losing it means a new node
	// identity and a new public URL on every reboot, so a build with no
	// data partition at all can never work correctly - refusing it at
	// build time is strictly better than shipping a device that silently
	// re-registers itself forever.
	requiresDataPartition: "tailscale-funnel stores its tailnet identity on the data partition",
}

// ingressAgents is gosd's registry of every --ingress value it understands.
var ingressAgents = []ingressAgent{cloudflaredAgent, tailscaleFunnelAgent}

// findIngressAgent looks up name in ingressAgents.
func findIngressAgent(name string) (ingressAgent, bool) {
	for _, a := range ingressAgents {
		if a.name == name {
			return a, true
		}
	}
	return ingressAgent{}, false
}

// ingressAgentNames lists every valid --ingress value, in registry order,
// for an unknown-value error message.
func ingressAgentNames() []string {
	names := make([]string, len(ingressAgents))
	for i, a := range ingressAgents {
		names[i] = a.name
	}
	return names
}

// ingressSelection is the registry-shaped result of parsing --ingress: which
// known agents were requested. Each agent gets its own named field, rather
// than a generic set, because the rest of gosd (resolveSharedContent/
// compileForBoards, pipeline.Options.IngressCloudflared/
// IngressTailscaleFunnel) needs to address a specific agent's content - a
// third agent's own field would land here alongside these, in the bean that
// wires it all the way through.
type ingressSelection struct {
	Cloudflared     bool
	TailscaleFunnel bool
}

// has reports whether name was selected, keyed by the same names
// ingressAgents registers - see validateIngress, which loops over the
// registry rather than this struct's fields directly.
func (s ingressSelection) has(name string) bool {
	switch name {
	case ingressCloudflaredValue:
		return s.Cloudflared
	case ingressTailscaleFunnelValue:
		return s.TailscaleFunnel
	default:
		return false
	}
}

// parseIngressFlags validates the repeatable --ingress flag's values
// against gosd's registry of known ingress agents (ingressAgents), fail-fast
// before any cross-compilation starts. An unknown value's error lists every
// valid agent name; repeating a known value more than once is harmless
// (idempotent).
func parseIngressFlags(flags []string) (ingressSelection, error) {
	var sel ingressSelection
	for _, v := range flags {
		agent, ok := findIngressAgent(v)
		if !ok {
			return ingressSelection{}, fmt.Errorf("--ingress %q is invalid; valid values are: %s", v, strings.Join(ingressAgentNames(), ", "))
		}
		switch agent.name {
		case ingressCloudflaredValue:
			sel.Cloudflared = true
		case ingressTailscaleFunnelValue:
			sel.TailscaleFunnel = true
		}
	}
	return sel, nil
}

// validateIngressDataPartition fails fast when a selected --ingress agent
// needs a writable data partition (see
// ingressAgent.requiresDataPartition) but the build has none: dataSizeBytes
// is 0 and dataExpand is false. Without this check, `gosd build --ingress
// tailscale-funnel` with no --data-size would boot a device that loses its
// tsnet state - and therefore its public URL - on every single reboot,
// discovered only in the field, long after the image shipped. A no-op when
// dataSizeBytes>0, dataExpand is true, or no selected agent requires a data
// partition; --data-size=expand satisfies the requirement even though the
// image itself carries no partition (dataexpand creates one before
// StartNetworking, per epic gosd-65uy decision 3).
func validateIngressDataPartition(sel ingressSelection, dataSizeBytes int64, dataExpand bool) error {
	if dataSizeBytes > 0 || dataExpand {
		return nil
	}
	for _, agent := range ingressAgents {
		if !sel.has(agent.name) || agent.requiresDataPartition == "" {
			continue
		}
		return fmt.Errorf("--ingress %s failed: %s; pass --data-size (e.g. --data-size=64MiB or --data-size=expand)", agent.name, agent.requiresDataPartition)
	}
	return nil
}

// validateIngress fails fast when a selected --ingress agent has no pinned
// binary for a board's GOARCH (see ingressAgent.capableGOARCH) - without
// this check, gosd build --ingress cloudflared --board pi-zero-w would
// either fail deep inside the resolve/fetch step or, worse, ship a binary
// that faults with "illegal instruction" the moment it runs. Each selected
// agent is validated independently against selected, mirroring
// validateUsbGadget's shape: name every incapable board's reason, name every
// capable board, and suggest --board= to narrow the build. A no-op when sel
// selects no agent, or every selected board's GOARCH is capable.
func validateIngress(selected []boards.Board, sel ingressSelection) error {
	for _, agent := range ingressAgents {
		if !sel.has(agent.name) {
			continue
		}
		if err := validateIngressAgent(selected, agent); err != nil {
			return err
		}
	}
	return nil
}

// validateIngressAgent is validateIngress's per-agent check.
func validateIngressAgent(selected []boards.Board, agent ingressAgent) error {
	var incapable, capable []string
	for _, b := range selected {
		ok, reason := agent.capableGOARCH(b.Arch().GOARCH)
		if ok {
			capable = append(capable, b.Name())
			continue
		}
		incapable = append(incapable, fmt.Sprintf("%s (%s)", b.Name(), reason))
	}
	if len(incapable) == 0 {
		return nil
	}

	msg := fmt.Sprintf("--ingress %s failed: %s", agent.name, strings.Join(incapable, "; "))
	if len(capable) > 0 {
		msg += fmt.Sprintf("; other selected boards do support --ingress %s (%s) — try restricting the build with --board=%s",
			agent.name, strings.Join(capable, ", "), capable[0])
	}
	return errors.New(msg)
}

// ingressGOARCHes returns the distinct GOARCH values among selected, sorted
// for determinism, so a multi-board build resolves exactly the cloudflared
// binaries it actually needs - no more, no less - mirroring
// compileForBoards' per-arch compile dedupe (boards.Arch.Key()).
func ingressGOARCHes(selected []boards.Board) []string {
	seen := make(map[string]bool, len(selected))
	goarches := make([]string, 0, len(selected))
	for _, b := range selected {
		goarch := b.Arch().GOARCH
		if seen[goarch] {
			continue
		}
		seen[goarch] = true
		goarches = append(goarches, goarch)
	}
	sort.Strings(goarches)
	return goarches
}

// ingressCacheDir is where gosd build/run --ingress cloudflared's pinned
// binary is cached across builds, kept separate from board artifact caches
// (artifactCacheDir) since it's resolved once per invocation - keyed by
// GOARCH, not board - rather than once per board.
func ingressCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locating a user cache directory for the cloudflared download failed: %w; try passing --artifacts-dir instead", err)
	}
	return filepath.Join(base, "gosd", "ingress"), nil
}

// resolveIngressCloudflared resolves cloudflaredpin.ByGOARCH's pinned
// binary for every GOARCH in goarches (see ingressGOARCHes), once each
// regardless of how many selected boards share that GOARCH.
// artifactsDir/<Name> is checked first for each GOARCH - the
// integration-test seam, the same well-known-name convention every other
// artifact source here uses, applying to a per-GOARCH override too -
// otherwise the pinned URL is fetched into cacheDir via internal/fetch,
// verifying its SHA-256. Every goarch is assumed already validated present
// in cloudflaredpin.ByGOARCH by validateIngress before this is ever called.
func resolveIngressCloudflared(ctx context.Context, artifactsDir, cacheDir string, goarches []string) (map[string]string, error) {
	paths := make(map[string]string, len(goarches))
	for _, goarch := range goarches {
		art := cloudflaredpin.ByGOARCH[goarch]

		if artifactsDir != "" {
			local := filepath.Join(artifactsDir, art.Name)
			if _, err := os.Stat(local); err == nil {
				paths[goarch] = local
				continue
			}
		}

		name := art.SHA256 + "-" + art.Name
		path, err := fetch.ToDir(ctx, nil, art.File, cacheDir, name)
		if err != nil {
			return nil, fmt.Errorf("fetching the cloudflared binary for GOARCH=%s failed: %w; check your network connection, or supply your own %s via --artifacts-dir", goarch, err, art.Name)
		}
		paths[goarch] = path
	}
	return paths, nil
}

// openIngressCloudflaredForBoard opens a fresh reader for b's GOARCH entry
// in paths, and pre-flights it against b.Arch() via staticelf.Verify - this
// applies even to an --artifacts-dir override, following the same
// "never trust unverified bytes into an image" rule --with-external's
// validateStaticELF follows. paths missing b's GOARCH means validateIngress
// was skipped, which is a caller bug, not a user-facing condition.
func openIngressCloudflaredForBoard(paths map[string]string, b boards.Board) (io.Reader, error) {
	path, ok := paths[b.Arch().GOARCH]
	if !ok {
		return nil, fmt.Errorf("internal error: no resolved cloudflared binary for %s's GOARCH=%s; validateIngress should have refused this board earlier", b.Name(), b.Arch().GOARCH)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening cached cloudflared binary at %s: %w", path, err)
	}
	if err := staticelf.Verify(f, f.Name(), b.Arch()); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("the cloudflared binary at %s failed verification for --board %s: %w; if you supplied it via --artifacts-dir, check it matches this board's architecture", path, b.Name(), err)
	}
	return f, nil
}

// openTsfunnelBinary opens a fresh reader for the gosd-tsfunnel shim
// compileForBoards already cross-compiled at tsfunnelPath for this board's
// arch. Unlike openIngressCloudflaredForBoard, no ELF pre-flight is needed:
// this binary was just produced by the same host Go toolchain, targeting
// this exact board's boards.Arch, in this same invocation - not a
// downloaded/cached blob whose provenance --artifacts-dir might have
// substituted.
func openTsfunnelBinary(tsfunnelPath string) (io.Reader, error) {
	f, err := os.Open(tsfunnelPath)
	if err != nil {
		return nil, fmt.Errorf("opening the compiled gosd-tsfunnel binary at %s: %w", tsfunnelPath, err)
	}
	return f, nil
}
