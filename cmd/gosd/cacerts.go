package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/cacerts"
	"github.com/jphastings/gosd/internal/fetch"
)

// caCertsCacheDir is where the pinned Mozilla CA bundle (internal/cacerts)
// is cached across builds, kept separate from board artifact caches
// (artifactCacheDir) since it's resolved once per invocation rather than
// per board.
func caCertsCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locating a user cache directory for the CA bundle download failed: %w; try passing --artifacts-dir with a %s already in it", err, cacerts.ArtifactName)
	}
	return filepath.Join(base, "gosd", "cacerts"), nil
}

// resolveCACerts resolves the Mozilla CA bundle every image ships at
// cacerts.InitramfsPath, once per gosd invocation rather than once per
// board: artifactsDir/cacerts.ArtifactName first, if present (the
// integration-test seam, same well-known-name convention board artifacts
// use), else internal/fetch's pinned URL+sha256 into cacheDir. Returns the
// bundle's local, sha256-verified path.
func resolveCACerts(ctx context.Context, artifactsDir, cacheDir string) (string, error) {
	if artifactsDir != "" {
		local := filepath.Join(artifactsDir, cacerts.ArtifactName)
		if _, err := os.Stat(local); err == nil {
			return local, nil
		}
	}

	name := cacerts.Pin.SHA256 + "-" + cacerts.ArtifactName
	path, err := fetch.ToDir(ctx, nil, cacerts.Pin, cacheDir, name)
	if err != nil {
		return "", fmt.Errorf("fetching the Mozilla CA bundle baked into every image failed: %w; check your network connection, or supply your own %s via --artifacts-dir", err, cacerts.ArtifactName)
	}
	return path, nil
}

// openCACertsForBoard opens a fresh reader for path, so the one CA bundle
// resolveCACerts resolves once for the whole invocation can still be
// embedded independently in each selected board's own initramfs -
// pipeline.Assemble closes every reader it's handed once that board's build
// is done, so each board needs its own *os.File.
func openCACertsForBoard(path string) (io.Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening cached CA bundle at %s: %w", path, err)
	}
	return f, nil
}
