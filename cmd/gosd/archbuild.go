package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/build"
)

// readyImportPath is github.com/jphastings/gosd/ready's own import path -
// the package whose mere presence in an app's dependency graph (see
// build.ImportsPackage) is gosd build's signal to set config.json's
// AppSignalsReady (see that package's doc for why importing is the
// declaration, with no separate build flag).
const readyImportPath = "github.com/jphastings/gosd/ready"

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

	// appOpts is exactly what appPath was compiled with - kept so a
	// later, board-scoped `go list -deps` inspection (gosd-42vb's
	// AppSignalsReady detection, see build.ImportsPackage) reuses this
	// board's own Tags rather than recomputing appBuildTags a second time
	// and risking the two falling out of step.
	appOpts build.AppCompileOptions
}

// appBuildTags returns the -tags value compileForBoards compiles b's app
// with: b's own mandatory boards.BuildTags, plus extraTags merged on when
// the caller supplied any (--tags). Factored out so the per-board
// AppSignalsReady inspection (cmd/gosd/build.go, cmd/gosd/run.go) can ask
// "what tags would this board's app compile use" without duplicating the
// merge logic.
func appBuildTags(b boards.Board, extraTags []string) string {
	tags := boards.BuildTags(b)
	if len(extraTags) > 0 {
		tags = strings.Join(append([]string{tags}, extraTags...), ",")
	}
	return tags
}

// compileForBoards cross-compiles, for every board in selected: the app at
// pkgPath once per board (each pass tagged with that board's
// boards.BuildTags, so `//go:build gosd`- and `//go:build gosd_<id>`-gated
// app source compiles for the right board - see gosd-1937 and gosd-cm4b),
// gosd-init once per distinct arch among
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
// ldflags, extraTags, gcflags, asmflags and trimpath are gosd build's
// --ldflags/--tags/--gcflags/--asmflags/--trimpath values (gosd-wjjn),
// forwarded verbatim to every board's app compile via build.AppCompileOptions
// except extraTags, which is merged onto (never replaces) each board's own
// mandatory boards.BuildTags - see parseExtraTags's reserved-namespace
// rejection, which is what keeps a caller's --tags from ever silently
// dropping gosd's board-gating tags.
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
	ldflags string,
	extraTags []string, gcflags, asmflags string, trimpath bool,
	compileApp func(pkgPath, outputPath string, opts build.AppCompileOptions, arch boards.Arch) error,
	compileInit func(outputPath, overrideDir string, arch boards.Arch) error,
	compileTsfunnel func(outputPath, overrideDir string, arch boards.Arch) error,
) (map[string]archBinaries, error) {
	binaries := make(map[string]archBinaries, len(selected))
	initPaths := make(map[string]string, len(selected))
	tsfunnelPaths := make(map[string]string, len(selected))

	for _, b := range selected {
		appBinary := filepath.Join(tempDir, "app-"+b.Name())
		opts := build.AppCompileOptions{
			Tags:     appBuildTags(b, extraTags),
			LDFlags:  ldflags,
			GCFlags:  gcflags,
			ASMFlags: asmflags,
			TrimPath: trimpath,
		}
		if err := compileApp(pkgPath, appBinary, opts, b.Arch()); err != nil {
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

		binaries[b.Name()] = archBinaries{appPath: appBinary, initPath: initBinary, tsfunnelPath: tsfunnelBinary, appOpts: opts}
	}

	return binaries, nil
}
