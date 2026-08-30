// Package gitversion resolves a "git:" app-version source (bean gosd-bggq)
// into a describe-style version string, in pure Go via go-git — no git
// binary is invoked, matching the project's no-extra-host-tools ethos.
//
// The value after "git:" is a glob matched against the app repository's
// tag names, and resolution follows git describe's semantics rather than
// "highest tag anywhere": the matching tag nearest HEAD's own ancestry
// wins, so building a maintenance branch never picks up a newer tag from
// another branch. An exactly-tagged HEAD yields the tag alone; otherwise
// <tag>-<count>-g<abbrev>, and a dirty worktree appends -dirty (never an
// error). The glob's literal prefix — everything before its first
// wildcard — is stripped from the result, so git:v*.*.* turns tag v1.4.2
// into 1.4.2 with no hardcoded vocabulary about what a version looks like.
package gitversion

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// Scheme prefixes an app-version value that resolves from git tags at
// build time instead of being taken literally.
const Scheme = "git:"

// abbrevLen matches git describe's default object-name abbreviation.
const abbrevLen = 7

// IsGitSource reports whether raw names a git-resolved version source.
func IsGitSource(raw string) bool {
	return strings.HasPrefix(raw, Scheme)
}

// HasAnyTag reports whether the git repository enclosing dir has at least
// one tag of any kind (lightweight or annotated) — the minimal positive
// signal a caller needs to decide whether a "git:" version source is
// worth defaulting to, without running Resolve's own HEAD-reachability/
// nearest-tag search. dir outside a git repository, or a repository with
// zero tags, both report false — never an error, since both simply mean
// "not eligible for a git: default" to the one caller (gosd init) that
// needs this.
func HasAnyTag(dir string) bool {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return false
	}
	iter, err := repo.Tags()
	if err != nil {
		return false
	}
	defer iter.Close()
	found := false
	_ = iter.ForEach(func(*plumbing.Reference) error {
		found = true
		return storer.ErrStop
	})
	return found
}

// Resolve turns a Scheme-prefixed version source into a version string,
// searching the git repository that encloses dir (the app's main package
// directory, walking up — so the value versions the app's own repo however
// gosd was invoked). A bare "git:" matches every tag.
func Resolve(raw, dir string) (string, error) {
	glob := strings.TrimPrefix(raw, Scheme)
	if glob == "" {
		glob = "*"
	}
	if _, err := path.Match(glob, ""); err != nil {
		return "", fmt.Errorf("--app-version %s: %q is not a valid tag pattern (%v); use shell-style wildcards, e.g. git:v*.*.*", raw, glob, err)
	}

	described, err := describe(dir, glob)
	if err != nil {
		return "", err
	}

	version := strings.TrimPrefix(described, literalPrefix(glob))
	if version == "" {
		// A fully literal pattern (git:v1.2.3) strips to nothing: the tag
		// itself is the version the developer asked for.
		version = described
	}
	return version, nil
}

// candidate is one matching tag: where it points, and what a tie-break
// needs to know about it.
type candidate struct {
	name      string
	commit    plumbing.Hash
	annotated bool
	when      int64
	distance  int
}

func describe(dir, glob string) (string, error) {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return "", fmt.Errorf(
				"--app-version git:%s needs the app to live in a git repository, and %s is not inside one; "+
					"build from a checkout of your app's repo, or pass a literal version instead", glob, dir)
		}
		return "", fmt.Errorf("--app-version git:%s: opening the git repository enclosing %s failed: %w", glob, dir, err)
	}

	headRef, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf(
			"--app-version git:%s: the repository enclosing %s has no commits yet; "+
				"commit and tag your app, or pass a literal version instead", glob, dir)
	}
	headCommit, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		return "", fmt.Errorf("--app-version git:%s: reading HEAD commit %s failed: %w", glob, headRef.Hash(), err)
	}

	candidates, totalTags, err := matchingTags(repo, glob)
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", noTagError(repo, glob, totalTags, "no tag matches")
	}

	reachable, err := ancestry(headCommit)
	if err != nil {
		return "", fmt.Errorf("--app-version git:%s: walking history from HEAD failed: %w", glob, err)
	}

	best, err := nearest(repo, candidates, reachable)
	if err != nil {
		return "", err
	}
	if best == nil {
		return "", noTagError(repo, glob, totalTags, "no matching tag is reachable from HEAD")
	}

	version := best.name
	if best.distance > 0 {
		version = fmt.Sprintf("%s-%d-g%s", best.name, best.distance, headRef.Hash().String()[:abbrevLen])
	}

	dirty, err := worktreeDirty(repo)
	if err != nil {
		return "", fmt.Errorf("--app-version git:%s: checking whether the worktree is dirty failed: %w", glob, err)
	}
	if dirty {
		version += "-dirty"
	}
	return version, nil
}

// matchingTags collects every tag whose short name matches glob, resolved
// to the commit it (possibly via an annotated tag object) points at, plus
// the repository's total tag count for error messages.
func matchingTags(repo *git.Repository, glob string) ([]candidate, int, error) {
	iter, err := repo.Tags()
	if err != nil {
		return nil, 0, fmt.Errorf("--app-version git:%s: listing tags failed: %w", glob, err)
	}
	defer iter.Close()

	var candidates []candidate
	total := 0
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		total++
		name := ref.Name().Short()
		if ok, _ := path.Match(glob, name); !ok {
			return nil
		}

		if tag, tagErr := repo.TagObject(ref.Hash()); tagErr == nil {
			commit, commitErr := tag.Commit()
			if commitErr != nil {
				return nil // a tag of a tree/blob (or nested tag) can't version a build
			}
			candidates = append(candidates, candidate{name: name, commit: commit.Hash, annotated: true, when: tag.Tagger.When.Unix()})
			return nil
		}

		commit, commitErr := repo.CommitObject(ref.Hash())
		if commitErr != nil {
			return nil
		}
		candidates = append(candidates, candidate{name: name, commit: commit.Hash, when: commit.Committer.When.Unix()})
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("--app-version git:%s: listing tags failed: %w", glob, err)
	}
	return candidates, total, nil
}

// ancestry returns every commit reachable from c, keyed by hash.
func ancestry(c *object.Commit) (map[plumbing.Hash]bool, error) {
	seen := make(map[plumbing.Hash]bool)
	iter := object.NewCommitPreorderIter(c, nil, nil)
	defer iter.Close()
	err := iter.ForEach(func(commit *object.Commit) error {
		seen[commit.Hash] = true
		return nil
	})
	return seen, err
}

// nearest picks the candidate git describe would: among tags whose commit
// is reachable from HEAD, the one with the fewest commits reachable from
// HEAD but not from the tag. Because the tag's commit is an ancestor of
// HEAD, that count is |ancestry(HEAD)| - |ancestry(tag)|. Ties prefer
// annotated tags, then the newest (tagger date, else commit date), then
// the lexically last name — pinned by tests so the choice can't drift.
func nearest(repo *git.Repository, candidates []candidate, reachable map[plumbing.Hash]bool) (*candidate, error) {
	distances := make(map[plumbing.Hash]int)
	var inReach []candidate
	for _, c := range candidates {
		if !reachable[c.commit] {
			continue
		}
		if _, ok := distances[c.commit]; !ok {
			commit, err := repo.CommitObject(c.commit)
			if err != nil {
				return nil, err
			}
			tagReach, err := ancestry(commit)
			if err != nil {
				return nil, err
			}
			distances[c.commit] = len(reachable) - len(tagReach)
		}
		c.distance = distances[c.commit]
		inReach = append(inReach, c)
	}
	if len(inReach) == 0 {
		return nil, nil
	}

	sort.Slice(inReach, func(i, j int) bool {
		a, b := inReach[i], inReach[j]
		if a.distance != b.distance {
			return a.distance < b.distance
		}
		if a.annotated != b.annotated {
			return a.annotated
		}
		if a.when != b.when {
			return a.when > b.when
		}
		return a.name > b.name
	})
	return &inReach[0], nil
}

// worktreeDirty mirrors git describe --dirty's diff-index semantics: a
// tracked file modified in the index or worktree makes the build dirty;
// untracked files do not. A bare repository is never dirty.
func worktreeDirty(repo *git.Repository) (bool, error) {
	wt, err := repo.Worktree()
	if errors.Is(err, git.ErrIsBareRepository) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	status, err := wt.Status()
	if err != nil {
		return false, err
	}
	for _, fs := range status {
		if fs.Staging == git.Untracked && fs.Worktree == git.Untracked {
			continue
		}
		if fs.Staging != git.Unmodified || fs.Worktree != git.Unmodified {
			return true, nil
		}
	}
	return false, nil
}

// noTagError explains an empty result actionably, with the specific
// shallow-clone remedy when that is the likely cause: CI checkouts default
// to depth 1 with no tags, and "no tag matches" would otherwise read as a
// pattern mistake.
func noTagError(repo *git.Repository, glob string, totalTags int, what string) error {
	if shallow, err := repo.Storer.Shallow(); err == nil && len(shallow) > 0 {
		return fmt.Errorf(
			"--app-version git:%s: %s %q, and this is a shallow clone, which usually has no tags or history to search; "+
				"run `git fetch --unshallow --tags --filter=tree:0`, or in GitHub Actions give actions/checkout "+
				"`fetch-depth: 0` with `filter: tree:0` — the filter keeps the fetch small by skipping historical "+
				"file contents, since the commit graph and tags are all a version needs",
			glob, what, glob)
	}
	return fmt.Errorf(
		"--app-version git:%s: %s %q (the repository has %d tags); "+
			"tag a commit reachable from HEAD to match, adjust the pattern, or pass a literal version instead",
		glob, what, glob, totalTags)
}

// literalPrefix returns glob's leading literal text: everything before the
// first wildcard metacharacter. It is what Resolve strips from the tag.
func literalPrefix(glob string) string {
	if i := strings.IndexAny(glob, "*?[\\"); i >= 0 {
		return glob[:i]
	}
	return glob
}
