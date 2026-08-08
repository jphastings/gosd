package main

import (
	"fmt"
	"path/filepath"

	"github.com/jphastings/gosd/internal/boards"
)

// archBinaries is one board's cross-compiled binaries (the name predates
// gosd-1937's per-board app tagging; initPath/tsfunnelPath may now be shared
// with other boards on the same arch, while appPath never is).
type archBinaries struct {
	appPath  string
	initPath string

	// tsfunnelPath is the per-arch-compiled cmd/gosd-tsfunnel shim (bean
	// gosd-kzd3), empty unless needsTsfunnel was true - i.e. unless
	// --ingress tailscale-funnel was selected.
	tsfunnelPath string
}

// compileForBoards cross-compiles, for every board in selected: the app at
// pkgPath once per board (each pass tagged with that board's
// boards.BuildTag, so `//go:build gosd_<id>`-gated app source compiles for
// the right board - see gosd-1937), gosd-init once per distinct arch among
// selected (keyed by boards.Arch.Key(), unchanged and untagged - see
// gosd-2j6z), and - only when needsTsfunnel is true (--ingress
// tailscale-funnel was selected, bean gosd-kzd3) - the gosd-tsfunnel shim,
// also once per distinct arch, beside gosd-init's own dedupe. Binaries are
// written into tempDir; the result is keyed by board name (b.Name()), with
// boards that share an arch pointing at the same initPath/tsfunnelPath. Two
// boards that share an arch - e.g. today's pi-zero-2w and radxa-zero-3e,
// both arm64 - therefore still share one gosd-init (and, when needed, one
// gosd-tsfunnel) compile pass; a GOARM=6 board mixed in (pi-zero-w) adds
// exactly one more of each.
//
// compileApp, compileInit and compileTsfunnel are the seams that make the
// per-board/per-arch compile counts testable without shelling out to the
// real Go toolchain: production callers pass build.CrossCompile,
// build.CrossCompileGosdInit and build.CrossCompileTsfunnel directly (their
// signatures already match); tests substitute invocation-counting fakes.
func compileForBoards(
	selected []boards.Board,
	tempDir, pkgPath, gosdInitSrc string,
	needsTsfunnel bool,
	compileApp func(pkgPath, outputPath, tags string, arch boards.Arch) error,
	compileInit func(outputPath, overrideDir string, arch boards.Arch) error,
	compileTsfunnel func(outputPath, overrideDir string, arch boards.Arch) error,
) (map[string]archBinaries, error) {
	binaries := make(map[string]archBinaries, len(selected))
	initPaths := make(map[string]string, len(selected))
	tsfunnelPaths := make(map[string]string, len(selected))

	for _, b := range selected {
		appBinary := filepath.Join(tempDir, "app-"+b.Name())
		if err := compileApp(pkgPath, appBinary, boards.BuildTag(b), b.Arch()); err != nil {
			return nil, fmt.Errorf("cross-compiling %s for %s failed: %w", pkgPath, b.Name(), err)
		}

		archKey := b.Arch().Key()
		initBinary, done := initPaths[archKey]
		if !done {
			initBinary = filepath.Join(tempDir, "gosd-init-"+archKey)
			if err := compileInit(initBinary, gosdInitSrc, b.Arch()); err != nil {
				return nil, fmt.Errorf("cross-compiling gosd-init for %s failed: %w", archKey, err)
			}
			initPaths[archKey] = initBinary
		}

		var tsfunnelBinary string
		if needsTsfunnel {
			tsfunnelBinary, done = tsfunnelPaths[archKey]
			if !done {
				tsfunnelBinary = filepath.Join(tempDir, "gosd-tsfunnel-"+archKey)
				if err := compileTsfunnel(tsfunnelBinary, gosdInitSrc, b.Arch()); err != nil {
					return nil, fmt.Errorf("cross-compiling gosd-tsfunnel for %s failed: %w", archKey, err)
				}
				tsfunnelPaths[archKey] = tsfunnelBinary
			}
		}

		binaries[b.Name()] = archBinaries{appPath: appBinary, initPath: initBinary, tsfunnelPath: tsfunnelBinary}
	}

	return binaries, nil
}
