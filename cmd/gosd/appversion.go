package main

import (
	"fmt"
	"path/filepath"

	"github.com/jphastings/gosd/internal/gitversion"
)

// resolveAppVersion turns a git:-scheme --app-version (or gosd-build.toml
// [app] version) into a concrete version string from the app repository's
// tags. Any other value passes through untouched — gosd resolves a git:
// source but still never interprets the resulting version.
func resolveAppVersion(raw, pkgPath string) (string, error) {
	if !gitversion.IsGitSource(raw) {
		return raw, nil
	}
	if !filesystemPathLike(pkgPath) {
		return "", fmt.Errorf(
			"--app-version %s resolves from the app repository's tags, which needs the app named by a local path (\".\", \"./cmd/myapp\", or absolute) rather than the import path %q; build from a checkout of the app instead",
			raw, pkgPath)
	}
	dir, err := filepath.Abs(pkgPath)
	if err != nil {
		return "", fmt.Errorf("--app-version %s: resolving %q to an absolute path failed: %w", raw, pkgPath, err)
	}
	return gitversion.Resolve(raw, dir)
}
