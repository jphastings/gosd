package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/artifacts"
	"github.com/jphastings/gosd/internal/cacerts"
	"github.com/jphastings/gosd/internal/cloudflaredpin"
)

// fakeSHA256 returns a syntactically valid (64 lowercase hex chars), but
// not-a-real-digest, sha256-shaped string for isFetchCacheEntry test
// fixtures - built from b repeated, so distinct calls (e.g. "ab", "cd")
// produce visibly distinct, easy-to-eyeball fake digests.
func fakeSHA256(b string) string {
	return strings.Repeat(b, 32)
}

// dirEntryNames lists dir's immediate entry names, sorted, so tests can
// assert on the exact remaining contents of a pruned cache directory.
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

func mkdirAllT(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
}

func writeFileT(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestPruneCacheToCurrentRemovesOnlySuperseded(t *testing.T) {
	dir := t.TempDir()
	mkdirAllT(t, filepath.Join(dir, "v0.9.0", "pi-zero-2w"))
	mkdirAllT(t, filepath.Join(dir, "v0.10.0", "pi-zero-2w"))
	writeFileT(t, filepath.Join(dir, "v0.9.0", "manifest.json"), "old")
	writeFileT(t, filepath.Join(dir, "v0.10.0", "manifest.json"), "current")

	if err := pruneCacheToCurrent(dir, []string{"v0.10.0"}, artifactVersionDirPattern.MatchString); err != nil {
		t.Fatalf("pruneCacheToCurrent: %v", err)
	}

	got := dirEntryNames(t, dir)
	want := []string{"v0.10.0"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("dir entries after prune = %v, want %v", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "v0.10.0", "manifest.json")); err != nil {
		t.Errorf("current version's content was disturbed: %v", err)
	}
}

func TestPruneCacheToCurrentLeavesUnknownEntriesAlone(t *testing.T) {
	dir := t.TempDir()
	mkdirAllT(t, filepath.Join(dir, "v0.10.0"))
	writeFileT(t, filepath.Join(dir, "README.txt"), "a stray file, not a version dir")
	mkdirAllT(t, filepath.Join(dir, "not-a-version"))

	if err := pruneCacheToCurrent(dir, []string{"v0.10.0"}, artifactVersionDirPattern.MatchString); err != nil {
		t.Fatalf("pruneCacheToCurrent: %v", err)
	}

	got := dirEntryNames(t, dir)
	want := []string{"README.txt", "not-a-version", "v0.10.0"}
	if len(got) != len(want) {
		t.Fatalf("dir entries after prune = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dir entries after prune = %v, want %v", got, want)
			break
		}
	}
}

func TestPruneCacheToCurrentLeavesInProgressDownloadsAlone(t *testing.T) {
	dir := t.TempDir()
	current := fakeSHA256("ab") + "-ca-certificates.crt"
	partial := current + ".part-12345678"
	writeFileT(t, filepath.Join(dir, current), "current pin")
	writeFileT(t, filepath.Join(dir, partial), "someone else's in-flight download")
	writeFileT(t, filepath.Join(dir, fakeSHA256("cd")+"-ca-certificates.crt"), "superseded pin")

	if err := pruneCacheToCurrent(dir, []string{current}, isFetchCacheEntry); err != nil {
		t.Fatalf("pruneCacheToCurrent: %v", err)
	}

	got := dirEntryNames(t, dir)
	want := []string{current, partial}
	if len(got) != len(want) {
		t.Fatalf("dir entries after prune = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dir entries after prune = %v, want %v", got, want)
			break
		}
	}
}

func TestPruneCacheToCurrentIsNoopOnMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	if err := pruneCacheToCurrent(dir, []string{"v0.10.0"}, artifactVersionDirPattern.MatchString); err != nil {
		t.Errorf("pruneCacheToCurrent on a missing dir = %v, want nil", err)
	}
}

func TestPruneArtifactCacheKeepsOnlyCurrentVersion(t *testing.T) {
	dir := t.TempDir()
	mkdirAllT(t, filepath.Join(dir, "v0.1.0", "pi-zero-2w"))
	mkdirAllT(t, filepath.Join(dir, artifacts.Version, "pi-zero-2w"))

	if err := pruneArtifactCache(dir); err != nil {
		t.Fatalf("pruneArtifactCache: %v", err)
	}

	got := dirEntryNames(t, dir)
	if len(got) != 1 || got[0] != artifacts.Version {
		t.Errorf("dir entries after pruneArtifactCache = %v, want [%s]", got, artifacts.Version)
	}
}

func TestPruneCACertsCacheKeepsOnlyCurrentPin(t *testing.T) {
	dir := t.TempDir()
	currentName := cacerts.Pin.SHA256 + "-" + cacerts.ArtifactName
	staleName := fakeSHA256("ee") + "-" + cacerts.ArtifactName
	writeFileT(t, filepath.Join(dir, currentName), "current bundle")
	writeFileT(t, filepath.Join(dir, staleName), "stale bundle")

	if err := pruneCACertsCache(dir); err != nil {
		t.Fatalf("pruneCACertsCache: %v", err)
	}

	got := dirEntryNames(t, dir)
	if len(got) != 1 || got[0] != currentName {
		t.Errorf("dir entries after pruneCACertsCache = %v, want [%s]", got, currentName)
	}
}

func TestPruneIngressCacheKeepsEveryCurrentlyPinnedGOARCH(t *testing.T) {
	dir := t.TempDir()
	var keep []string
	for _, art := range cloudflaredpin.ByGOARCH {
		name := art.SHA256 + "-" + art.Name
		keep = append(keep, name)
		writeFileT(t, filepath.Join(dir, name), "current")
	}
	staleName := fakeSHA256("ff") + "-cloudflared-linux-arm64"
	writeFileT(t, filepath.Join(dir, staleName), "stale")

	if err := pruneIngressCache(dir); err != nil {
		t.Fatalf("pruneIngressCache: %v", err)
	}

	sort.Strings(keep)
	got := dirEntryNames(t, dir)
	if len(got) != len(keep) {
		t.Fatalf("dir entries after pruneIngressCache = %v, want %v", got, keep)
	}
	for i := range keep {
		if got[i] != keep[i] {
			t.Errorf("dir entries after pruneIngressCache = %v, want %v", got, keep)
			break
		}
	}
}

func TestPruneDownloadCachesSkipsWhenArtifactsDirSet(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmpHome, ".cache"))

	cacheDir, err := artifactCacheDir()
	if err != nil {
		t.Fatalf("artifactCacheDir: %v", err)
	}
	staleDir := filepath.Join(cacheDir, "v0.1.0", "pi-zero-2w")
	mkdirAllT(t, staleDir)

	cmd := newRootCmd()
	pruneDownloadCaches(cmd, "/some/artifacts/dir")

	if _, err := os.Stat(staleDir); err != nil {
		t.Errorf("pruneDownloadCaches with a non-empty artifactsDir touched the cache: %v", err)
	}
}

func TestPruneDownloadCachesPrunesWhenArtifactsDirEmpty(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmpHome, ".cache"))

	cacheDir, err := artifactCacheDir()
	if err != nil {
		t.Fatalf("artifactCacheDir: %v", err)
	}
	staleDir := filepath.Join(cacheDir, "v0.1.0", "pi-zero-2w")
	mkdirAllT(t, staleDir)

	cmd := newRootCmd()
	pruneDownloadCaches(cmd, "")

	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Errorf("pruneDownloadCaches(artifactsDir=\"\") left stale version dir in place; stat err = %v", err)
	}
}
