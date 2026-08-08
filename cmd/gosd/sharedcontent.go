package main

import (
	"context"
	"io"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/cacerts"
)

// sharedContent holds the resolved location of every initramfs content item
// that ships in EVERY image regardless of board or which command built it
// (`gosd build` or `gosd run`) - the Mozilla CA bundle (bean gosd-kzgq),
// always present, plus the cloudflared binary (bean gosd-g4km), present
// only when --ingress cloudflared was passed. resolveSharedContent computes
// it once per invocation; openSharedContent opens a fresh set of readers
// from it for each board's pipeline.Options, mirroring the
// resolve-once/open-per-board split every other extra-content source here
// (kernel firmware, --with-external) already uses - pipeline.Assemble
// closes every reader it's handed, so a multi-board build needs its own set
// per board.
//
// runBuild and runRun both build their pipeline.Options.ExtraFiles/
// ExtraExecutables through this one path (bean gosd-x3j5) rather than each
// resolving/opening this content by hand, so content added here reaches
// both commands by construction - the CA bundle itself nearly shipped
// build-only before a review pass caught it and added the same wiring to
// run.go separately, which is exactly the failure mode this path exists to
// prevent for --ingress too (see buildrun_parity_integration_test.go).
// Fields that genuinely differ between the two commands (Board, Config's
// WiFi/UsbGadget/Env/ConsoleBaud, sizing, --with-external's own
// ExtraExecutables) stay explicit at runBuild's and runRun's own call
// sites, not here.
//
// The tailscale-funnel shim (bean gosd-kzd3) is deliberately NOT here: it's
// compiled per-arch by compileForBoards (archbuild.go) alongside the app and
// gosd-init, rather than resolved from a pinned/local artifact like the CA
// bundle and cloudflared are, so build.go and run.go each open its compiled
// binary straight from compileForBoards' result. The build+run parity this
// type exists to protect still holds for it: both commands call
// compileForBoards, so the same wiring mistake this type prevents for
// fetched/cached content can't arise for compiled content either - see
// buildrun_parity_integration_test.go, which asserts both agents' content
// either way.
type sharedContent struct {
	caCertsPath string

	// ingressCloudflaredPaths maps GOARCH -> the locally resolved
	// cloudflared binary path (see resolveIngressCloudflared), nil unless
	// --ingress cloudflared was passed. Keyed by GOARCH, not board,
	// because the binary is architecture-specific, not board-specific -
	// mirroring compileForBoards' per-arch compile dedupe.
	ingressCloudflaredPaths map[string]string
}

// resolveSharedContent resolves every sharedContent path once per gosd
// invocation, checking artifactsDir for a local override before falling
// back to a pinned-URL fetch into its own cache directory - the same rule
// every other artifact source here follows. ingressGOARCHesNeeded is empty
// unless --ingress cloudflared was passed, in which case it's the distinct
// GOARCHes among the boards that need it (see ingressGOARCHes) - an empty
// slice means the cloudflared binary is never resolved and the network is
// never touched for it, which is what proves --ingress is genuinely opt-in.
func resolveSharedContent(ctx context.Context, artifactsDir string, ingressGOARCHesNeeded []string) (sharedContent, error) {
	caCertsCache, err := caCertsCacheDir()
	if err != nil {
		return sharedContent{}, err
	}
	caCertsPath, err := resolveCACerts(ctx, artifactsDir, caCertsCache)
	if err != nil {
		return sharedContent{}, err
	}

	var ingressPaths map[string]string
	if len(ingressGOARCHesNeeded) > 0 {
		ingressCache, err := ingressCacheDir()
		if err != nil {
			return sharedContent{}, err
		}
		ingressPaths, err = resolveIngressCloudflared(ctx, artifactsDir, ingressCache, ingressGOARCHesNeeded)
		if err != nil {
			return sharedContent{}, err
		}
	}

	return sharedContent{caCertsPath: caCertsPath, ingressCloudflaredPaths: ingressPaths}, nil
}

// openSharedContent opens a fresh pipeline.Options.ExtraFiles map (the CA
// bundle) and, when ingress was resolved, a fresh ExtraExecutables map (the
// cloudflared binary, pre-flighted against b.Arch()) from c's resolved
// paths, suitable for one board's build. Call it once per board:
// pipeline.Assemble closes every reader it's handed once that board's build
// is done, so a multi-board build needs a fresh set each time, exactly like
// openKernelFirmware and openExternalsForBoard. extraExecutables is nil
// when c.ingressCloudflaredPaths is nil (--ingress wasn't passed).
func openSharedContent(c sharedContent, b boards.Board) (extraFiles, extraExecutables map[string]io.Reader, err error) {
	caCerts, err := openCACertsForBoard(c.caCertsPath)
	if err != nil {
		return nil, nil, err
	}

	if c.ingressCloudflaredPaths != nil {
		cf, err := openIngressCloudflaredForBoard(c.ingressCloudflaredPaths, b)
		if err != nil {
			if closer, ok := caCerts.(io.Closer); ok {
				_ = closer.Close()
			}
			return nil, nil, err
		}
		extraExecutables = map[string]io.Reader{ingressCloudflaredDest: cf}
	}

	return map[string]io.Reader{cacerts.InitramfsPath: caCerts}, extraExecutables, nil
}
