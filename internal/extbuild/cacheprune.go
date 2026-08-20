package extbuild

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
// defaultBuildRoot accumulates (bean gosd-9o73), mirroring
// kernelbuild.keepBuildCacheEntries - see that constant's doc comment for
// the full rationale (the two packages are deliberate siblings, not a
// shared abstraction; see this package's doc comment).
const keepBuildCacheEntries = 8

// cacheKeyPattern matches cacheKey's output shape (a hex sha256), mirroring
// kernelbuild.cacheKeyPattern.
var cacheKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// touchAndPruneCache marks currentKey as just-used and best-effort-prunes
// cacheRoot down to the keepBuildCacheEntries most recently used entries,
// mirroring kernelbuild.touchAndPruneCache - see that function's doc comment
// for the full rationale.
func touchAndPruneCache(cacheRoot, currentKey string, stderr io.Writer) {
	now := time.Now()
	_ = os.Chtimes(filepath.Join(cacheRoot, currentKey), now, now)

	if err := pruneStaleCacheEntries(cacheRoot, currentKey, keepBuildCacheEntries); err != nil && stderr != nil {
		_, _ = fmt.Fprintf(stderr, "gosd build-external: pruning superseded build-cache entries under %s failed (continuing): %v\n", cacheRoot, err)
	}
}

// pruneStaleCacheEntries removes every cacheKeyPattern-shaped entry in
// cacheRoot except currentKey and the keepN entries with the most recent
// modification time, mirroring kernelbuild.pruneStaleCacheEntries.
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
