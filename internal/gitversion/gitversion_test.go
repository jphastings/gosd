package gitversion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// fixtureRepo builds test repositories with go-git itself, so the tests
// need no git binary and stay hermetic.
type fixtureRepo struct {
	t    *testing.T
	dir  string
	repo *git.Repository
	wt   *git.Worktree
	n    int
	when time.Time
}

func newFixtureRepo(t *testing.T) *fixtureRepo {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	return &fixtureRepo{t: t, dir: dir, repo: repo, wt: wt, when: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (f *fixtureRepo) sig() *object.Signature {
	f.when = f.when.Add(time.Minute)
	return &object.Signature{Name: "fixture", Email: "fixture@example.com", When: f.when}
}

func (f *fixtureRepo) commit() plumbing.Hash {
	f.t.Helper()
	f.n++
	name := "file" + string(rune('a'+f.n)) + ".txt"
	if err := os.WriteFile(filepath.Join(f.dir, name), []byte("content"), 0o644); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.wt.Add(name); err != nil {
		f.t.Fatal(err)
	}
	hash, err := f.wt.Commit("commit "+name, &git.CommitOptions{Author: f.sig(), Committer: f.sig()})
	if err != nil {
		f.t.Fatal(err)
	}
	return hash
}

func (f *fixtureRepo) lightweightTag(name string, hash plumbing.Hash) {
	f.t.Helper()
	if _, err := f.repo.CreateTag(name, hash, nil); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixtureRepo) annotatedTag(name string, hash plumbing.Hash) {
	f.t.Helper()
	if _, err := f.repo.CreateTag(name, hash, &git.CreateTagOptions{Tagger: f.sig(), Message: name}); err != nil {
		f.t.Fatal(err)
	}
}

func mustResolve(t *testing.T, raw, dir string) string {
	t.Helper()
	got, err := Resolve(raw, dir)
	if err != nil {
		t.Fatalf("Resolve(%q) errored: %v", raw, err)
	}
	return got
}

func TestIsGitSource(t *testing.T) {
	if !IsGitSource("git:v*") || IsGitSource("1.4.2") || IsGitSource("") {
		t.Fatal("IsGitSource must recognize exactly the git: prefix")
	}
}

func TestExactlyTaggedHeadStripsThePatternPrefix(t *testing.T) {
	f := newFixtureRepo(t)
	f.annotatedTag("v1.4.2", f.commit())
	if got := mustResolve(t, "git:v*.*.*", f.dir); got != "1.4.2" {
		t.Errorf("Resolve = %q, want the glob's literal prefix stripped (1.4.2)", got)
	}
}

func TestCommitsPastTheTagGetDescribeSuffix(t *testing.T) {
	f := newFixtureRepo(t)
	f.annotatedTag("v1.4.2", f.commit())
	f.commit()
	head := f.commit()

	got := mustResolve(t, "git:v*.*.*", f.dir)
	want := "1.4.2-2-g" + head.String()[:7]
	if got != want {
		t.Errorf("Resolve = %q, want %q (tag, distance, abbreviated HEAD)", got, want)
	}
}

func TestDirtyWorktreeSuffixesNeverRefuses(t *testing.T) {
	f := newFixtureRepo(t)
	f.annotatedTag("v1.0.0", f.commit())

	// An untracked file is not dirt (git describe --dirty semantics).
	if err := os.WriteFile(filepath.Join(f.dir, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mustResolve(t, "git:v*", f.dir); got != "1.0.0" {
		t.Errorf("Resolve with only an untracked file = %q, want no -dirty suffix", got)
	}

	// Modifying a tracked file is.
	if err := os.WriteFile(filepath.Join(f.dir, "fileb.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mustResolve(t, "git:v*", f.dir); got != "1.0.0-dirty" {
		t.Errorf("Resolve with a modified tracked file = %q, want the -dirty suffix", got)
	}
}

func TestNearestReachableTagWinsNotHighest(t *testing.T) {
	f := newFixtureRepo(t)
	f.commit()
	old := f.commit()
	f.annotatedTag("v1.0.0", old)

	// A newer tag on a side branch must not win from the main line.
	main, err := f.repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.wt.Checkout(&git.CheckoutOptions{Hash: old, Branch: "refs/heads/side", Create: true}); err != nil {
		t.Fatal(err)
	}
	f.annotatedTag("v2.0.0", f.commit())
	if err := f.wt.Checkout(&git.CheckoutOptions{Branch: main.Name()}); err != nil {
		t.Fatal(err)
	}
	head := f.commit()

	got := mustResolve(t, "git:v*", f.dir)
	want := "1.0.0-1-g" + head.String()[:7]
	if got != want {
		t.Errorf("Resolve = %q, want %q: v2.0.0 lives on an unreachable branch and must lose to the reachable v1.0.0", got, want)
	}
}

func TestGlobFiltersTags(t *testing.T) {
	f := newFixtureRepo(t)
	first := f.commit()
	f.annotatedTag("v1.0.0", first)
	f.lightweightTag("nightly-20260101", f.commit())

	if got := mustResolve(t, "git:nightly-*", f.dir); got != "20260101" {
		t.Errorf("Resolve(nightly-*) = %q, want the nightly tag with its prefix stripped", got)
	}
}

func TestTieBreakPrefersAnnotatedOverLightweight(t *testing.T) {
	f := newFixtureRepo(t)
	head := f.commit()
	f.lightweightTag("v1.0.1", head)
	f.annotatedTag("v1.0.0", head)

	if got := mustResolve(t, "git:v*", f.dir); got != "1.0.0" {
		t.Errorf("Resolve = %q, want the annotated tag to beat the lightweight one on the same commit", got)
	}
}

func TestFullyLiteralPatternKeepsTheTag(t *testing.T) {
	f := newFixtureRepo(t)
	f.annotatedTag("v1.0.0", f.commit())
	if got := mustResolve(t, "git:v1.0.0", f.dir); got != "v1.0.0" {
		t.Errorf("Resolve = %q; a wildcard-free pattern strips to nothing, so the tag itself is the version", got)
	}
}

func TestBareGitSchemeMatchesAnyTag(t *testing.T) {
	f := newFixtureRepo(t)
	f.annotatedTag("release-7", f.commit())
	if got := mustResolve(t, "git:", f.dir); got != "release-7" {
		t.Errorf("Resolve(git:) = %q, want any tag to match (release-7)", got)
	}
}

func TestErrorsAreActionable(t *testing.T) {
	t.Run("not a repository", func(t *testing.T) {
		_, err := Resolve("git:v*", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "git repository") {
			t.Errorf("error = %v, want it to explain the app isn't in a git repository", err)
		}
	})

	t.Run("no commits", func(t *testing.T) {
		f := newFixtureRepo(t)
		_, err := Resolve("git:v*", f.dir)
		if err == nil || !strings.Contains(err.Error(), "no commits") {
			t.Errorf("error = %v, want it to say the repository has no commits", err)
		}
	})

	t.Run("no matching tag names the pattern and count", func(t *testing.T) {
		f := newFixtureRepo(t)
		f.annotatedTag("nightly-1", f.commit())
		_, err := Resolve("git:v*", f.dir)
		if err == nil || !strings.Contains(err.Error(), `"v*"`) || !strings.Contains(err.Error(), "1 tags") {
			t.Errorf("error = %v, want the pattern and the existing tag count named", err)
		}
	})

	t.Run("unreachable matching tag is distinguished", func(t *testing.T) {
		f := newFixtureRepo(t)
		first := f.commit()
		main, err := f.repo.Head()
		if err != nil {
			t.Fatal(err)
		}
		if err := f.wt.Checkout(&git.CheckoutOptions{Hash: first, Branch: "refs/heads/side", Create: true}); err != nil {
			t.Fatal(err)
		}
		f.annotatedTag("v9.9.9", f.commit())
		if err := f.wt.Checkout(&git.CheckoutOptions{Branch: main.Name()}); err != nil {
			t.Fatal(err)
		}
		_, err = Resolve("git:v*", f.dir)
		if err == nil || !strings.Contains(err.Error(), "reachable from HEAD") {
			t.Errorf("error = %v, want it to say no matching tag is reachable from HEAD", err)
		}
	})

	t.Run("shallow clone steers to the checkout-depth fix", func(t *testing.T) {
		f := newFixtureRepo(t)
		head := f.commit()
		if err := os.WriteFile(filepath.Join(f.dir, ".git", "shallow"), []byte(head.String()+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Resolve("git:v*", f.dir)
		if err == nil || !strings.Contains(err.Error(), "--unshallow") ||
			!strings.Contains(err.Error(), "fetch-depth: 0") || !strings.Contains(err.Error(), "tree:0") {
			t.Errorf("error = %v, want the unshallow and treeless actions/checkout remedies named", err)
		}
	})

	t.Run("invalid pattern", func(t *testing.T) {
		_, err := Resolve("git:v[", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "not a valid tag pattern") {
			t.Errorf("error = %v, want the malformed pattern refused by name", err)
		}
	})
}

func TestResolveSearchesUpwardFromTheAppDir(t *testing.T) {
	f := newFixtureRepo(t)
	f.annotatedTag("v3.0.0", f.commit())
	sub := filepath.Join(f.dir, "cmd", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := mustResolve(t, "git:v*", sub); got != "3.0.0" {
		t.Errorf("Resolve from a subdirectory = %q, want the enclosing repo's tag found (3.0.0, and untracked dirs are not dirt)", got)
	}
}
