package kernelbuild

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// makeCacheEntry creates a fake content-addressed cache entry directory
// named key under root, with a deterministic modTime so ordering in tests
// is exact rather than dependent on wall-clock write speed.
func makeCacheEntry(t *testing.T, root, key string, modTime time.Time) {
	t.Helper()
	dir := filepath.Join(root, key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.Chtimes(dir, modTime, modTime); err != nil {
		t.Fatalf("Chtimes(%s): %v", dir, err)
	}
}

// fakeCacheKey returns a syntactically valid (64 lowercase hex chars), but
// not-a-real-digest, cache key for test fixtures.
func fakeCacheKey(b string) string {
	s := ""
	for len(s) < 64 {
		s += b
	}
	return s[:64]
}

func dirEntryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestPruneStaleCacheEntriesKeepsOnlyTheMostRecentN(t *testing.T) {
	root := t.TempDir()
	base := time.Now()

	keys := make([]string, 5)
	for i := range keys {
		keys[i] = fakeCacheKey(string(rune('a' + i)))
		// Oldest first: keys[0] is the least recently used.
		makeCacheEntry(t, root, keys[i], base.Add(time.Duration(i)*time.Hour))
	}

	if err := pruneStaleCacheEntries(root, keys[len(keys)-1], 3); err != nil {
		t.Fatalf("pruneStaleCacheEntries: %v", err)
	}

	got := dirEntryNames(t, root)
	want := []string{keys[2], keys[3], keys[4]}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("entries after prune = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entries after prune = %v, want %v", got, want)
			break
		}
	}
}

func TestPruneStaleCacheEntriesNeverRemovesCurrentKey(t *testing.T) {
	root := t.TempDir()
	base := time.Now()

	// The current key is the OLDEST entry - as if it had just been created
	// with a clock that runs behind the others - to prove it survives
	// regardless of recency ordering.
	current := fakeCacheKey("0")
	makeCacheEntry(t, root, current, base)
	for i, b := range []string{"a", "b", "c", "d", "e"} {
		makeCacheEntry(t, root, fakeCacheKey(b), base.Add(time.Duration(i+1)*time.Hour))
	}

	if err := pruneStaleCacheEntries(root, current, 3); err != nil {
		t.Fatalf("pruneStaleCacheEntries: %v", err)
	}

	got := dirEntryNames(t, root)
	found := false
	for _, name := range got {
		if name == current {
			found = true
		}
	}
	if !found {
		t.Errorf("current key %s was pruned; entries = %v", current, got)
	}
	if len(got) != 3 {
		t.Errorf("entries after prune = %v, want 3 total (current key + 2 most recent)", got)
	}
}

func TestPruneStaleCacheEntriesLeavesUnmanagedEntriesAlone(t *testing.T) {
	root := t.TempDir()
	base := time.Now()

	current := fakeCacheKey("a")
	makeCacheEntry(t, root, current, base)
	// work-*/build.tmp-* staging dirs (see runBuild) and any other
	// unrecognised entry must never be touched by a prune.
	for _, name := range []string{"work-123456", "build.tmp-654321", "not-a-cache-key"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	if err := pruneStaleCacheEntries(root, current, 1); err != nil {
		t.Fatalf("pruneStaleCacheEntries: %v", err)
	}

	got := dirEntryNames(t, root)
	want := []string{"build.tmp-654321", current, "not-a-cache-key", "work-123456"}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("entries after prune = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entries after prune = %v, want %v", got, want)
			break
		}
	}
}

func TestPruneStaleCacheEntriesIsNoopOnMissingDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")
	if err := pruneStaleCacheEntries(root, fakeCacheKey("a"), 3); err != nil {
		t.Errorf("pruneStaleCacheEntries on a missing dir = %v, want nil", err)
	}
}

func TestTouchAndPruneCacheUpdatesCurrentKeysModTime(t *testing.T) {
	root := t.TempDir()
	current := fakeCacheKey("a")
	stale := time.Now().Add(-24 * time.Hour)
	makeCacheEntry(t, root, current, stale)

	touchAndPruneCache(root, current, nil)

	info, err := os.Stat(filepath.Join(root, current))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.ModTime().Before(stale.Add(time.Hour)) {
		t.Errorf("touchAndPruneCache did not refresh %s's modtime: got %s, still looks like the old %s", current, info.ModTime(), stale)
	}
}
