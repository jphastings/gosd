package main

import (
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"time"
)

// TestBuildBakesGitResolvedAppVersionIntoConfigJSON is gosd-bggq's
// acceptance test: an app repo whose checked-in gosd-build.toml says
// version = "git:v*.*.*" gets the tag-derived version baked into
// config.json, end to end, with no git binary and no network.
func TestBuildBakesGitResolvedAppVersionIntoConfigJSON(t *testing.T) {
	disableNetwork(t)
	dir := t.TempDir()
	writeTestAppRepo(t, dir, `
board = ["pi-zero-2w"]
output = "versioned.img"
artifacts-dir = "`+absFakeArtifacts(t)+`"

[app]
main = "./app"
version = "git:v*.*.*"
`)

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := wt.AddGlob("."); err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "fixture", Email: "fixture@example.com", When: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	hash, err := wt.Commit("initial", &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTag("v2.5.0", hash, &git.CreateTagOptions{Tagger: sig, Message: "v2.5.0"}); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"build"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(filepath.Join(dir, "versioned.img"), diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()
	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs: %v", err)
	}
	configJSON := string(recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json"))
	if want := `"appVersion":"2.5.0"`; !strings.Contains(configJSON, want) {
		t.Errorf("config.json = %s, want it to carry the git-resolved %s", configJSON, want)
	}
}

// TestBuildLDFlagsSubstitutesGitResolvedAppVersion is the combined
// acceptance test for the motivating scenario behind --ldflags's
// {{.AppVersion}} token: a gosd-build.toml pairing version = "git:v*.*.*"
// with ldflags = "-X main.version={{.AppVersion}}" must resolve the
// version once and carry it into both config.json (like
// TestBuildBakesGitResolvedAppVersionIntoConfigJSON above) and the
// compiled /app binary, with no duplicated resolution logic in the caller.
func TestBuildLDFlagsSubstitutesGitResolvedAppVersion(t *testing.T) {
	disableNetwork(t)
	dir := t.TempDir()
	writeTestAppRepoWithMain(t, dir, "package main\n\nvar version = \"dev\"\n\nfunc main() { println(version) }\n", `
board = ["pi-zero-2w"]
output = "versioned.img"
artifacts-dir = "`+absFakeArtifacts(t)+`"
ldflags = "-X main.version={{.AppVersion}}"

[app]
main = "./app"
version = "git:v*.*.*"
`)

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := wt.AddGlob("."); err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "fixture", Email: "fixture@example.com", When: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	hash, err := wt.Commit("initial", &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTag("v2.5.0", hash, &git.CreateTagOptions{Tagger: sig, Message: "v2.5.0"}); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"build"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	imgPath := filepath.Join(dir, "versioned.img")
	configJSON := string(readInitramfsRecord(t, imgPath, "etc/gosd/config.json"))
	if want := `"appVersion":"2.5.0"`; !strings.Contains(configJSON, want) {
		t.Errorf("config.json = %s, want it to carry the git-resolved %s", configJSON, want)
	}

	app := readAppBinary(t, imgPath)
	if !strings.Contains(string(app), "2.5.0") {
		t.Error("/app does not contain \"2.5.0\"; {{.AppVersion}} did not carry the same git-resolved version into the binary")
	}
}
