package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jphastings/gosd/internal/extbuild"
	"github.com/jphastings/gosd/internal/kernelbuild"
)

// isolateCacheEnv points every cache-location helper (os.UserCacheDir,
// os.UserHomeDir, $XDG_STATE_HOME) at a fresh temp dir, so these tests never
// touch a real developer's actual gosd caches.
func isolateCacheEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".state"))
}

// populateAllCacheLocations writes one file into every cache location (the
// four download caches plus the two durable build-state dirs), returning
// each location's directory so tests can assert on them afterwards.
func populateAllCacheLocations(t *testing.T) map[string]string {
	t.Helper()
	dirs := make(map[string]string)
	for _, loc := range cacheLocations() {
		dir, err := loc.dir()
		if err != nil {
			t.Fatalf("%s: %v", loc.name, err)
		}
		mkdirAllT(t, dir)
		writeFileT(t, filepath.Join(dir, "fixture"), "cached content")
		dirs[loc.name] = dir
	}
	return dirs
}

func TestCacheDirPrintsEveryLocation(t *testing.T) {
	isolateCacheEnv(t)

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"cache", "dir"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd cache dir: %v", err)
	}

	for _, loc := range cacheLocations() {
		dir, err := loc.dir()
		if err != nil {
			t.Fatalf("%s: %v", loc.name, err)
		}
		if !bytes.Contains(out.Bytes(), []byte(dir)) {
			t.Errorf("gosd cache dir output missing %s's path %s:\n%s", loc.name, dir, out.String())
		}
	}
}

func TestCacheSizeReportsWhatWasWritten(t *testing.T) {
	isolateCacheEnv(t)
	populateAllCacheLocations(t)

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"cache", "size"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd cache size: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("total")) {
		t.Errorf("gosd cache size output missing a total line:\n%s", out.String())
	}
}

// TestCacheCleanLeavesBuildStateAloneByDefault is the point of gosd-2jwa's
// --builds split: a plain `gosd cache clean` must never destroy the
// durable build-kernel/build-external cache, since each of its entries
// costs 20-75 minutes of container build time to reproduce (bean gosd-9o73).
func TestCacheCleanLeavesBuildStateAloneByDefault(t *testing.T) {
	isolateCacheEnv(t)
	dirs := populateAllCacheLocations(t)
	cacheCleanBuilds = false
	t.Cleanup(func() { cacheCleanBuilds = false })

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"cache", "clean"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd cache clean: %v", err)
	}

	kernelDir, err := kernelbuild.BuildRoot()
	if err != nil {
		t.Fatal(err)
	}
	extDir, err := extbuild.BuildRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, expensive := range []string{kernelDir, extDir} {
		if _, err := os.Stat(filepath.Join(expensive, "fixture")); err != nil {
			t.Errorf("gosd cache clean (no --builds) removed expensive build state under %s: %v", expensive, err)
		}
	}

	for _, name := range []string{"board artifacts", "CA certificate bundle", "ingress binaries (cloudflared)", "kernel firmware (gosd-kernel.toml [[firmware]])"} {
		if _, err := os.Stat(dirs[name]); !os.IsNotExist(err) {
			t.Errorf("gosd cache clean left %s (%s) in place: stat err = %v", name, dirs[name], err)
		}
	}
}

// TestCacheCleanWithBuildsRemovesEverything is --builds's whole point: an
// explicit opt-in to also pay back the expensive build-kernel/build-external
// state.
func TestCacheCleanWithBuildsRemovesEverything(t *testing.T) {
	isolateCacheEnv(t)
	populateAllCacheLocations(t)
	cacheCleanBuilds = false
	t.Cleanup(func() { cacheCleanBuilds = false })

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"cache", "clean", "--builds"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd cache clean --builds: %v", err)
	}

	for _, loc := range cacheLocations() {
		dir, err := loc.dir()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("gosd cache clean --builds left %s (%s) in place: stat err = %v", loc.name, dir, err)
		}
	}
}
