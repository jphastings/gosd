package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// beanToken matches the *maximal* `gosd-`-prefixed token, so a longer name that
// merely begins like a bean id is seen whole and can be rejected on length.
// The obvious `\bgosd-[a-z0-9]{4}\b` matches "gosd-data" inside
// "gosd-data-established": "-" is a non-word character, so `\b` happily fires
// mid-token, and Go's regexp has no lookahead to forbid a trailing "-".
var beanToken = regexp.MustCompile(`gosd-[a-z0-9]{4}[-a-z0-9]*`)

// beanIDLen is len("gosd-") + the four id characters.
const beanIDLen = 9

// notBeans are tokens that are exactly bean-id shaped but name something else:
// the second binary gosd builds into every image, and the data partition label
// that also prefixes the `gosd-data-established` marker.
var notBeans = map[string]struct{}{
	"gosd-init": {},
	"gosd-data": {},
}

// TestDocsCiteOnlyKnownBeans asserts that every bean id cited in the published
// documentation still resolves to a bean file, so renaming or deleting a bean
// cannot quietly orphan the citation that explains a decision.
//
// Scope is deliberate in two directions. `.beans/` itself is not scanned:
// beans cross-reference each other and archived beans cite ids that were
// superseded or folded elsewhere, which is a different invariant with a
// different noise profile. And there is no reverse check — most beans are
// never cited from the docs, so "every bean is referenced" is not a property
// this repo has or wants.
func TestDocsCiteOnlyKnownBeans(t *testing.T) {
	known := knownBeanIDs(t)

	for _, doc := range append(repoDocFiles(t), "CLAUDE.md") {
		data, err := os.ReadFile(filepath.Join("../..", doc))
		if err != nil {
			t.Fatalf("reading %s: %v", doc, err)
		}
		for n, line := range strings.Split(string(data), "\n") {
			for _, tok := range beanToken.FindAllString(line, -1) {
				if len(tok) != beanIDLen {
					continue
				}
				if _, skip := notBeans[tok]; skip {
					continue
				}
				if _, ok := known[tok]; !ok {
					t.Errorf("%s:%d cites bean %s, which has no file under .beans/ or .beans/archive/ — cite the bean that replaced it, or drop the reference", doc, n+1, tok)
				}
			}
		}
	}
}

// knownBeanIDs reads the ids off the bean filenames, each of which is
// `<id>--<slug>.md`. `.beans` is dot-prefixed so the go tool ignores it as a
// package directory, but it is an ordinary directory to os.ReadDir.
func knownBeanIDs(t *testing.T) map[string]struct{} {
	t.Helper()

	ids := make(map[string]struct{})
	for _, dir := range []string{".beans", ".beans/archive"} {
		entries, err := os.ReadDir(filepath.Join("../..", dir))
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, e := range entries {
			id, _, ok := strings.Cut(e.Name(), "--")
			if !ok {
				continue
			}
			ids[id] = struct{}{}
		}
	}
	if len(ids) == 0 {
		t.Fatal("no bean files found under .beans/ — the check would pass vacuously")
	}
	return ids
}

// repoDocFiles lists the repository's published markdown documentation as
// repo-relative slash paths: everything under docs/ plus the two root
// documents. CLAUDE.md is not included; callers that want it add it, because
// it is an agent-facing index rather than published prose.
func repoDocFiles(t *testing.T) []string {
	t.Helper()

	files := []string{"README.md", "COMPATIBILITY.md"}
	err := filepath.WalkDir(filepath.Join("../..", "docs"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(p) != ".md" {
			return nil
		}
		rel, err := filepath.Rel("../..", p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walking docs/: %v", err)
	}
	sort.Strings(files)
	return files
}
