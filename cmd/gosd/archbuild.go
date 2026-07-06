package main

import (
	"fmt"
	"path/filepath"

	"github.com/jphastings/gosd/internal/boards"
)

// archBinaries is one arch's cross-compiled app and gosd-init binaries.
type archBinaries struct {
	appPath  string
	initPath string
}

// compileForArchs cross-compiles the app at pkgPath and gosd-init exactly
// once per distinct arch among archs (keyed by boards.Arch.Key()), writing
// binaries into tempDir and returning each arch's paths keyed the same way.
// Two boards that share an arch - e.g. today's pi-zero-2w and
// radxa-zero-3e, both arm64 - therefore share one compile pass; a
// GOARM=6 board mixed in (the upcoming pi-zero-w) adds exactly one more.
//
// compileApp and compileInit are the seams that make the dedupe itself
// testable without shelling out to the real Go toolchain per arch:
// production callers pass build.CrossCompile and build.CrossCompileGosdInit
// directly (their signatures already match); tests substitute invocation-
// counting fakes.
func compileForArchs(
	archs []boards.Arch,
	tempDir, pkgPath, gosdInitSrc string,
	compileApp func(pkgPath, outputPath string, arch boards.Arch) error,
	compileInit func(outputPath, overrideDir string, arch boards.Arch) error,
) (map[string]archBinaries, error) {
	binaries := make(map[string]archBinaries, len(archs))

	for _, arch := range archs {
		key := arch.Key()
		if _, done := binaries[key]; done {
			continue
		}

		appBinary := filepath.Join(tempDir, "app-"+key)
		if err := compileApp(pkgPath, appBinary, arch); err != nil {
			return nil, fmt.Errorf("cross-compiling %s for %s failed: %w", pkgPath, key, err)
		}

		initBinary := filepath.Join(tempDir, "gosd-init-"+key)
		if err := compileInit(initBinary, gosdInitSrc, arch); err != nil {
			return nil, fmt.Errorf("cross-compiling gosd-init for %s failed: %w", key, err)
		}

		binaries[key] = archBinaries{appPath: appBinary, initPath: initBinary}
	}

	return binaries, nil
}
