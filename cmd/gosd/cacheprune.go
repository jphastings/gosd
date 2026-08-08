package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/artifacts"
	"github.com/jphastings/gosd/internal/cacerts"
	"github.com/jphastings/gosd/internal/cloudflaredpin"
)

// artifactVersionDirPattern matches the "vX.Y.Z" directory name layout
// internal/artifacts.EnsureBoard lays its per-release cache out in
// (cacheDir/<version>/<board>/..., cacheDir/<version>/manifest.json). Only
// entries matching this pattern are ever candidates for removal by
// pruneArtifactCache - anything else (an unrecognised file, a directory from
// some future/older layout) is left strictly alone.
var artifactVersionDirPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// fetchCacheEntryPattern matches the "<sha256>-<name>" naming convention
// internal/fetch.ToDir's callers here (resolveCACerts,
// resolveIngressCloudflared) use for cached pinned-URL downloads.
var fetchCacheEntryPattern = regexp.MustCompile(`^[0-9a-f]{64}-.+$`)

// isFetchCacheEntry reports whether name looks like a finished
// fetch.ToDir-cached file. It deliberately excludes ToDir's own in-progress
// "<name>.part-*" temp files (see fetch.ToDir), so a prune can never race a
// concurrent gosd invocation that is mid-download in the same cache
// directory: a temp file is left alone regardless of keep, exactly like any
// other unrecognised entry.
func isFetchCacheEntry(name string) bool {
	return fetchCacheEntryPattern.MatchString(name) && !strings.Contains(name, ".part-")
}

// pruneCacheToCurrent removes every top-level entry in dir whose name
// isManaged reports true for but that isn't listed in keep, so the
// directory ends up holding exactly the current version's/pin's cache
// entries. Entries isManaged reports false for are left alone entirely -
// this function never removes a file or directory it doesn't recognise the
// shape of. A missing dir is a silent no-op (nothing cached yet to prune).
//
// It is best-effort: a failure removing one entry is collected but does not
// stop the rest from being attempted; the returned error (if any) joins
// every failure. Callers treat the result as advisory (log and continue),
// never as a reason to fail an otherwise-successful build - see
// pruneDownloadCaches.
func pruneCacheToCurrent(dir string, keep []string, isManaged func(name string) bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading cache directory %s: %w", dir, err)
	}

	keepSet := make(map[string]bool, len(keep))
	for _, k := range keep {
		keepSet[k] = true
	}

	var errs []error
	for _, entry := range entries {
		name := entry.Name()
		if keepSet[name] || !isManaged(name) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			errs = append(errs, fmt.Errorf("removing superseded cache entry %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// pruneArtifactCache removes every sibling vX.Y.Z directory under gosd's
// board-artifact cache dir (artifactCacheDir) other than the currently
// pinned internal/artifacts.Version, so re-running gosd at a new version
// doesn't grow the cache forever (bean gosd-gdro): each release's tree is
// tens of MiB, and nothing else ever revisits an old version's directory
// once gosd moves on to a new one.
func pruneArtifactCache(cacheDir string) error {
	return pruneCacheToCurrent(cacheDir, []string{artifacts.Version}, artifactVersionDirPattern.MatchString)
}

// pruneCACertsCache keeps only the file matching the currently pinned CA
// bundle (cacerts.Pin) in dir, removing any file left over from a previous
// pin bump.
func pruneCACertsCache(dir string) error {
	name := cacerts.Pin.SHA256 + "-" + cacerts.ArtifactName
	return pruneCacheToCurrent(dir, []string{name}, isFetchCacheEntry)
}

// pruneIngressCache keeps only the files matching cloudflaredpin.ByGOARCH's
// currently pinned binaries in dir, removing any left over from a previous
// cloudflared pin bump. Every GOARCH currently pinned in code is kept, not
// just the GOARCHes this particular invocation happened to resolve - a
// build restricted to one board/arch shouldn't force a re-download of
// another arch's still-current binary on the next build.
func pruneIngressCache(dir string) error {
	keep := make([]string, 0, len(cloudflaredpin.ByGOARCH))
	for _, art := range cloudflaredpin.ByGOARCH {
		keep = append(keep, art.SHA256+"-"+art.Name)
	}
	return pruneCacheToCurrent(dir, keep, isFetchCacheEntry)
}

// pruneDownloadCaches best-effort-prunes gosd's own pinned-download cache
// directories (artifactCacheDir, caCertsCacheDir, ingressCacheDir) down to
// exactly the current version's/pin's entries, once a build or run has
// successfully resolved them (bean gosd-gdro's core, everyday-user fix):
// left unmanaged, every gosd release adds another artifact tree that's
// never revisited, and cacerts/cloudflared pin bumps leave the old file
// behind too - `os.UserCacheDir()/gosd` was measured to hold over 100MiB on
// one machine, all but the current version's ~65MiB dead weight.
//
// It's skipped entirely when artifactsDir is non-empty: a --artifacts-dir
// build may not have touched the download cache at all for some or all of
// its artifacts (boards.ResolveArtifacts and its --ingress/cacerts
// equivalents check --artifacts-dir per file before ever falling back to a
// fetch), so gosd can't tell whether the cache actually reflects a
// successful resolve of the current pins - pruning it anyway could discard
// entries a concurrent or later --artifacts-dir-less invocation still
// needs, for no benefit to a path that isn't supposed to touch this cache
// in the first place.
//
// Failures are logged to cmd's stderr and otherwise ignored: pruning is a
// disk-hygiene courtesy that runs after the build/run it's attached to has
// already succeeded, never a reason to report that success as a failure.
func pruneDownloadCaches(cmd *cobra.Command, artifactsDir string) {
	if artifactsDir != "" {
		return
	}

	if dir, err := artifactCacheDir(); err == nil {
		if err := pruneArtifactCache(dir); err != nil {
			cmd.PrintErrf("gosd: pruning superseded cached artifacts under %s failed (continuing): %v\n", dir, err)
		}
	}
	if dir, err := caCertsCacheDir(); err == nil {
		if err := pruneCACertsCache(dir); err != nil {
			cmd.PrintErrf("gosd: pruning superseded cached CA bundles under %s failed (continuing): %v\n", dir, err)
		}
	}
	if dir, err := ingressCacheDir(); err == nil {
		if err := pruneIngressCache(dir); err != nil {
			cmd.PrintErrf("gosd: pruning superseded cached ingress binaries under %s failed (continuing): %v\n", dir, err)
		}
	}
}
