// Package cloudflaredpin pins the upstream cloudflared release that `gosd
// build --ingress cloudflared` bakes into an image (epic gosd-virc): a
// locally-managed Cloudflare Tunnel client that lets gosd-init expose one
// declared HTTP service to the public internet with zero app code. Like
// every other third-party binary blob GoSD ships (Pi GPU firmware, WiFi
// firmware, Rockchip rkbin), the binary is never re-hosted - only its
// upstream URL and SHA-256 are pinned here, downloaded and verified at
// build time via internal/fetch.
//
// ByGOARCH is deliberately narrow, and IS the capability table: cloudflared
// gosd build --ingress supports. cloudflared's official arm release is
// built for GOARM=7 and faults with "illegal instruction" on pi-zero-w's
// armv6 CPU (upstream issues cloudflare/cloudflared#1136 and #1162), and
// the GOARM level can't be recovered from an ELF header alone (bean
// gosd-aur4) to catch this some other way - so "arm" has no entry at all,
// and cmd/gosd/ingress.go's validation treats a missing GOARCH entry as
// "unsupported," never attempting a fetch for it. Adding a GOARCH here is
// what makes gosd build --ingress start supporting boards of that
// architecture; see CLAUDE.md's "Target" locked decision for the fleet's
// full GOARCH/GOARM vocabulary.
//
// Bump procedure:
//  1. Pick the new release tag from
//     https://github.com/cloudflare/cloudflared/releases.
//  2. For each GOARCH already listed below, find that release's
//     cloudflared-linux-<goarch> asset's SHA-256 in the release body's
//     "SHA256 Checksums" block (`gh release view <tag> -R
//     cloudflare/cloudflared`).
//  3. VERIFY it yourself before trusting it - the release body is
//     upstream-authored text, not a signed artifact: download the asset
//     (`gh release download <tag> -R cloudflare/cloudflared -p
//     cloudflared-linux-<goarch>`) and `shasum -a 256` it; it must match
//     the release-body digest exactly. Record the download command and the
//     matching digest in the bump's bean/PR.
//  4. Update Version and every entry's URL/SHA256 together. Name never
//     changes across a bump - it's both the --artifacts-dir well-known
//     override file name and the local cache key prefix, not tied to a
//     version.
package cloudflaredpin

import "github.com/jphastings/gosd/internal/fetch"

// Version is the pinned cloudflared release tag.
const Version = "2026.7.3"

// Artifact pins one GOARCH's downloadable cloudflared binary: where to
// fetch it (embedded fetch.File, carrying the SHA-256 every fetch verifies
// against), and Name.
type Artifact struct {
	fetch.File
	// Name is both the --artifacts-dir well-known override file name gosd
	// build checks before falling back to fetching URL, and the basename
	// used when caching the fetched file.
	Name string
}

// ByGOARCH maps a Go GOARCH to its pinned cloudflared binary at Version -
// see the package doc comment for why only "arm64" has an entry, and for
// the bump procedure.
//
// Verified for Version: cloudflared-linux-arm64 is a statically linked
// (no PT_INTERP) ELF 64-bit ARM aarch64 executable, ~35MiB; its SHA-256 was
// read from the GitHub release body and re-derived independently from a
// fresh download of the same asset, and the two matched (recorded in bean
// gosd-g4km).
var ByGOARCH = map[string]Artifact{
	"arm64": {
		File: fetch.File{
			URL:    "https://github.com/cloudflare/cloudflared/releases/download/" + Version + "/cloudflared-linux-arm64",
			SHA256: "65259e652a7bea08bf5df603233ab22b8bf3116af8df9f9206209af6a1b955c0",
		},
		Name: "cloudflared-linux-arm64",
	},
}
