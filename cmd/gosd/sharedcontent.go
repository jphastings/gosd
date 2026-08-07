package main

import (
	"context"
	"io"

	"github.com/jphastings/gosd/internal/cacerts"
)

// sharedContent holds the resolved location of every initramfs content item
// that ships in EVERY image regardless of board or which command built it
// (`gosd build` or `gosd run`) - today just the Mozilla CA bundle (bean
// gosd-kzgq). resolveSharedContent computes it once per invocation;
// openSharedContent opens a fresh set of readers from it for each board's
// pipeline.Options, mirroring the resolve-once/open-per-board split every
// other extra-content source here (kernel firmware, --with-external)
// already uses - pipeline.Assemble closes every reader it's handed, so a
// multi-board build needs its own set per board.
//
// runBuild and runRun both build their pipeline.Options.ExtraFiles through
// this one path (bean gosd-x3j5) rather than each resolving/opening the CA
// bundle by hand, so content added here reaches both commands by
// construction - the CA bundle itself nearly shipped build-only before a
// review pass caught it and added the same wiring to run.go separately.
// Fields that genuinely differ between the two commands (Board, Config's
// WiFi/UsbGadget/Env/ConsoleBaud, sizing, ExtraFirmware/ExtraExecutables)
// stay explicit at runBuild's and runRun's own call sites, not here.
type sharedContent struct {
	caCertsPath string
}

// resolveSharedContent resolves every sharedContent path once per gosd
// invocation, checking artifactsDir for a local override before falling
// back to a pinned-URL fetch into its own cache directory - the same rule
// every other artifact source here follows.
func resolveSharedContent(ctx context.Context, artifactsDir string) (sharedContent, error) {
	caCertsCache, err := caCertsCacheDir()
	if err != nil {
		return sharedContent{}, err
	}
	caCertsPath, err := resolveCACerts(ctx, artifactsDir, caCertsCache)
	if err != nil {
		return sharedContent{}, err
	}
	return sharedContent{caCertsPath: caCertsPath}, nil
}

// openSharedContent opens a fresh pipeline.Options.ExtraFiles map from c's
// resolved paths, suitable for one board's build. Call it once per board:
// pipeline.Assemble closes every reader it's handed once that board's build
// is done, so a multi-board build needs a fresh set each time, exactly like
// openKernelFirmware and openExternalsForBoard.
func openSharedContent(c sharedContent) (map[string]io.Reader, error) {
	caCerts, err := openCACertsForBoard(c.caCertsPath)
	if err != nil {
		return nil, err
	}
	return map[string]io.Reader{cacerts.InitramfsPath: caCerts}, nil
}
