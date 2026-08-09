package build

import (
	"fmt"
	"path/filepath"

	"github.com/jphastings/gosd/internal/boards"
)

// gosdTsfunnelRelPkg is where cmd/gosd-tsfunnel lives relative to the root
// of whichever copy of the gosd module we end up building it from (rungs 1
// and 2 of the ladder - see CrossCompileGosdInit's docstring, which this
// mirrors).
const gosdTsfunnelRelPkg = "./cmd/gosd-tsfunnel"

// tsfunnelSrcDirName is cmd/gosd-tsfunnel's own leaf directory name, used to
// derive its location from a --gosd-init-src override (see
// CrossCompileGosdInit's docstring): that flag is documented as pointing at
// gosd-init's own leaf package directory, not a checkout root, so
// CrossCompileTsfunnel swaps the last path element rather than reusing
// overrideDir as-is. Every real gosd source tree ships both binaries
// side-by-side under cmd/ - a full checkout, or a package manager's bundled
// copy (gosd-bfhd's nix hook copies the whole tree, not just cmd/gosd-init)
// - so this holds for every overrideDir a developer would actually pass.
const tsfunnelSrcDirName = "gosd-tsfunnel"

// tsfunnelLDFlags is gosd-65uy decision 2's "-ldflags=-s -w": the shim is
// stripped (no symbol table, no DWARF) since, unlike gosd-init, its content
// never needs to be inspected in the field and every byte counts against
// the boot partition's budget - measured at ~23% smaller in gosd-4fve's bean.
const tsfunnelLDFlags = "-s -w"

// tsfunnelOpts is the crossCompileOpts every CrossCompileTsfunnel rung uses.
// The shim is stripped (tsfunnelLDFlags, -ldflags="-s -w") but is NOT
// feature-trimmed: gosd-65uy decision 2's ts_omit_* tag set is deliberately
// gone.
//
// One of those ~74 tags broke tsnet's control-plane registration. The
// trimmed shim fell into a keyless interactive login (StartLoginInteractive
// -> doLogin(regen=true)) that the coordination server answered with 404, so
// the device could never join the tailnet and Funnel never opened. This was
// root-caused on real hardware (bean gosd-h46e): on the SAME board, key and
// network, an un-trimmed tsnet binary registers and serves Funnel end to end,
// while the trimmed shim 404s within ~30ms; it never reproduced off-bench
// because every other test built an un-trimmed binary (`go build`), and only
// `gosd build` applied the trim. Rather than bisect which tag is load-bearing
// for registration, the shim now ships full tsnet — ~30MB of RAM-resident
// initramfs, well worth guaranteed registration.
//
// The epic's "no interactive surface" property no longer rests on compiling
// Tailscale SSH out (ts_omit_ssh). It holds because the shim only ever calls
// ListenFunnel and never enables tsnet's SSH server: the SSH code may be
// present but is unreachable (never advertised, and the tailnet ACL doesn't
// grant it).
var tsfunnelOpts = crossCompileOpts{ldflags: tsfunnelLDFlags}

// CrossCompileTsfunnel locates cmd/gosd-tsfunnel's source (gosd's own tsnet
// Funnel shim, epic gosd-65uy decision 1: compiled FROM GOSD SOURCE, the
// main module, never a nested one) and cross-compiles it to outputPath with
// -ldflags="-s -w" (decision 2) and no feature-trim tags (see tsfunnelOpts
// and gosd-h46e for why the ts_omit_* set was dropped). It follows
// the exact same 3-rung ladder as CrossCompileGosdInit - dev checkout,
// module cache, --gosd-init-src override - since gosd-tsfunnel ships
// alongside gosd-init in every real gosd source tree; see
// CrossCompileGosdInit's docstring for the two detected rungs, and
// tsfunnelSrcDirName's docstring for how the override rung's directory is
// derived from overrideDir rather than reused as-is.
func CrossCompileTsfunnel(outputPath, overrideDir string, arch boards.Arch) error {
	if overrideDir != "" {
		dir := filepath.Join(filepath.Dir(overrideDir), tsfunnelSrcDirName)
		return crossCompileInDir(dir, ".", outputPath, arch,
			fmt.Sprintf("--gosd-init-src %s (gosd-tsfunnel sibling %s)", overrideDir, dir), tsfunnelOpts)
	}

	if dir, ok := devCheckoutDir(); ok {
		return crossCompileInDir(dir, gosdTsfunnelRelPkg, outputPath, arch,
			fmt.Sprintf("local checkout at %s", dir), tsfunnelOpts)
	}

	dir, err := moduleCacheDir()
	if err != nil {
		return err
	}
	return crossCompileInDir(dir, gosdTsfunnelRelPkg, outputPath, arch,
		fmt.Sprintf("module cache at %s", dir), tsfunnelOpts)
}
