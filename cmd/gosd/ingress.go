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
	"github.com/jphastings/gosd/internal/cloudflaredpin"
	"github.com/jphastings/gosd/internal/fetch"
	"github.com/jphastings/gosd/internal/staticelf"
)

// ingressCloudflaredValue is the only --ingress value gosd understands
// today (epic gosd-virc, v1: a locally-managed Cloudflare Tunnel only).
const ingressCloudflaredValue = "cloudflared"

// ingressCloudflaredDest is where gosd build/run --ingress cloudflared
// embeds the cloudflared binary inside the initramfs.
const ingressCloudflaredDest = "/bin/cloudflared"

// parseIngressFlags validates the repeatable --ingress flag's values,
// fail-fast before any cross-compilation starts: the only value gosd
// understands today is "cloudflared". Returns whether cloudflared ingress
// was requested at all; repeating "cloudflared" more than once is harmless
// (idempotent), but any other value is refused outright.
func parseIngressFlags(flags []string) (cloudflared bool, err error) {
	for _, v := range flags {
		if v != ingressCloudflaredValue {
			return false, fmt.Errorf("--ingress %q is invalid; the only supported value is %q", v, ingressCloudflaredValue)
		}
		cloudflared = true
	}
	return cloudflared, nil
}

// validateIngress fails fast when --ingress cloudflared is set and any
// board in selected has no pinned cloudflared binary for its GOARCH (see
// cloudflaredpin.ByGOARCH, the capability table) - without this check, gosd
// build --ingress cloudflared --board pi-zero-w would either fail deep
// inside the resolve/fetch step or, worse, ship a binary that faults with
// "illegal instruction" the moment it runs (cloudflared's official arm
// release is GOARM=7; pi-zero-w is armv6 - see cloudflaredpin's doc
// comment). Mirrors validateUsbGadget's shape: name every incapable
// board's reason, name every capable board, and suggest --board= to narrow
// the build. A no-op when ingress is false or every selected board's
// GOARCH is in cloudflaredpin.ByGOARCH.
func validateIngress(selected []boards.Board, ingress bool) error {
	if !ingress {
		return nil
	}

	var incapable, capable []string
	for _, b := range selected {
		if _, ok := cloudflaredpin.ByGOARCH[b.Arch().GOARCH]; ok {
			capable = append(capable, b.Name())
			continue
		}

		reason := fmt.Sprintf("no cloudflared build is pinned for GOARCH=%s", b.Arch().GOARCH)
		if b.Arch().GOARCH == "arm" {
			reason = "cloudflared's official arm release is built for GOARM=7 and faults with \"illegal instruction\" on this board's armv6 CPU"
		}
		incapable = append(incapable, fmt.Sprintf("%s (%s)", b.Name(), reason))
	}
	if len(incapable) == 0 {
		return nil
	}

	msg := fmt.Sprintf("--ingress cloudflared failed: %s", strings.Join(incapable, "; "))
	if len(capable) > 0 {
		msg += fmt.Sprintf("; other selected boards do support --ingress cloudflared (%s) — try restricting the build with --board=%s",
			strings.Join(capable, ", "), capable[0])
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
