package kernelbuild

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// keepBuildCacheEntries bounds how many content-addressed entries
// defaultBuildRoot accumulates (bean gosd-9o73). That directory is
// deliberately not under os.UserCacheDir (see defaultBuildRoot), so nothing
// else ever reclaims it, and every distinct kernelspec/overlay/image
// combination this checkout has ever built adds another entry - one JP
// measured at 652MiB across kernelbuild and extbuild combined. 8 comfortably
// covers a full board fleet's kernels plus a couple of in-flight overlay
// iterations, without letting the directory grow in proportion to how long
// or how often gosd build-kernel has been run - the parent principle behind
// bean gosd-gdro.
const keepBuildCacheEntries = 8

// cacheKeyPattern matches cacheKey's output shape (a hex sha256), the only
// entries pruneStaleCacheEntries ever considers touching - a work-*/
// build.tmp-* staging directory left over from an interrupted build (see
// runBuild) is a shape it does not recognise, so it is left alone exactly
// like any other unmanaged file.
var cacheKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// touchAndPruneCache marks currentKey as just-used, so an entry that is only
// ever cache-hit (never rebuilt) still counts as recently used rather than
// looking stale, then best-effort-prunes cacheRoot down to the
// keepBuildCacheEntries most recently used entries. currentKey is always
// kept regardless of where it lands in that ordering - the entry this very
// call just resolved is never the one pruneStaleCacheEntries removes.
//
// This runs only after Build has already produced (or found) its result, so
// a failure here is written to stderr (when non-nil) and otherwise ignored:
// disk hygiene is never a reason to fail a build that already succeeded,
// mirroring cmd/gosd's pruneDownloadCaches for the everyday download caches
// (bean gosd-gdro).
func touchAndPruneCache(cacheRoot, currentKey string, stderr io.Writer) {
	now := time.Now()
	_ = os.Chtimes(filepath.Join(cacheRoot, currentKey), now, now)

	if err := pruneStaleCacheEntries(cacheRoot, currentKey, keepBuildCacheEntries); err != nil && stderr != nil {
		_, _ = fmt.Fprintf(stderr, "gosd build-kernel: pruning superseded build-cache entries under %s failed (continuing): %v\n", cacheRoot, err)
	}
}

// pruneStaleCacheEntries removes every cacheKeyPattern-shaped entry in
// cacheRoot except currentKey and the keepN entries with the most recent
// modification time, so the directory never holds more than keepN entries
// plus (if it would otherwise have fallen out of that set) the one this
// build just used. A missing dir is a silent no-op. It is best-effort: one
// entry failing to remove is collected but does not stop the rest from
// being attempted.
func pruneStaleCacheEntries(cacheRoot, currentKey string, keepN int) error {
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading build cache directory %s: %w", cacheRoot, err)
	}

	type cacheEntry struct {
		name    string
		modTime time.Time
	}
	var keys []cacheEntry
	for _, entry := range entries {
		if !entry.IsDir() || !cacheKeyPattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		keys = append(keys, cacheEntry{name: entry.Name(), modTime: info.ModTime()})
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].modTime.After(keys[j].modTime) })

	keep := map[string]bool{currentKey: true}
	for _, k := range keys {
		if keep[k.name] {
			continue
		}
		if len(keep) >= keepN {
			continue
		}
		keep[k.name] = true
	}

	var errs []error
	for _, k := range keys {
		if keep[k.name] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(cacheRoot, k.name)); err != nil {
			errs = append(errs, fmt.Errorf("removing stale build cache entry %s: %w", k.name, err))
		}
	}
	return errors.Join(errs...)
}
