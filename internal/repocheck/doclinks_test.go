package repocheck

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// maxDocPathViolations is the ratchet: the number of doc-path references each
// file may name in prose rather than hyperlink behind a descriptive phrase.
// Seeded from the tree as it stood when the check landed, so the convention
// stops getting worse without a mass rewrite first. Lowering an entry as you
// fix the prose around it is welcome; raising one is the failure.
//
// A file absent from this map must have none at all.
var maxDocPathViolations = map[string]int{
	"COMPATIBILITY.md":             1,
	"README.md":                    17,
	"docs/custom-kernels.md":       4,
	"docs/design/ab-updates.md":    6,
	"docs/design/upgrade-path.md":  6,
	"docs/externals.md":            7,
	"docs/flashing.md":             1,
	"docs/image-injection.md":      4,
	"docs/ingress.md":              5,
	"docs/provisioning-formats.md": 7,
	"docs/publishing.md":           11,
	"docs/runtime.md":              13,
	"docs/sound.md":                1,
}

// docPathPattern recognises a token shaped like a path to a markdown document.
// The check is purely lexical on purpose: requiring the path to exist on disk
// false-positives on the generic filenames the docs cite as examples and on
// paths that belong to other repositories entirely.
var docPathPattern = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.-]*(?:/[A-Za-z0-9_.-]+)*\.md$`)

var (
	markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	inlineCode   = regexp.MustCompile("`([^`]+)`")
)

// TestDocPathsAreLinkedNotNamed is the doc-link ratchet. A document hyperlinks
// a descriptive phrase — "[the runtime contract](runtime.md)" — and never
// names a file path in prose, because a path tells a reader nothing about why
// they should follow it and rots invisibly when the file moves.
//
// Two things count as naming a path: an inline-code span that is nothing but a
// markdown path, and a link whose text is a path rather than a phrase. Fenced
// code blocks are exempt — a path inside an example command is the content.
// CLAUDE.md is excluded outright: it is a machine-oriented index for agents,
// where a bare path is the useful form.
func TestDocPathsAreLinkedNotNamed(t *testing.T) {
	docs := repoDocFiles(t)

	scanned := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		scanned[doc] = struct{}{}

		data, err := os.ReadFile(filepath.Join("../..", doc))
		if err != nil {
			t.Fatalf("reading %s: %v", doc, err)
		}
		found := scanDocPaths(string(data))
		allowed := maxDocPathViolations[doc]

		switch {
		case len(found) > allowed:
			t.Errorf("%s names %d doc paths in prose, up from %d — link a descriptive phrase instead, as COMPATIBILITY.md does throughout:", doc, len(found), allowed)
			for _, v := range found {
				t.Errorf("  %s:%d %s: %s", doc, v.line, v.kind, v.text)
			}
		case len(found) < allowed:
			t.Logf("%s is down to %d doc paths named in prose from %d — lower its entry in maxDocPathViolations", doc, len(found), allowed)
		}
	}

	for doc := range maxDocPathViolations {
		if _, ok := scanned[doc]; !ok {
			t.Errorf("maxDocPathViolations still budgets %q, which is no longer a scanned document — drop the entry", doc)
		}
	}
}

type docPathViolation struct {
	line int
	kind string
	text string
}

func scanDocPaths(src string) []docPathViolation {
	var found []docPathViolation
	inFence := false

	for i, line := range strings.Split(src, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		// Replace each link with its own text, so a path named inside otherwise
		// descriptive link text is still seen by the prose scan below. A link
		// whose whole text is a path is counted here and consumed.
		prose := markdownLink.ReplaceAllStringFunc(line, func(m string) string {
			text := markdownLink.FindStringSubmatch(m)[1]
			if isRepoDocPath(unquote(text)) {
				found = append(found, docPathViolation{line: i + 1, kind: "link text is a path", text: m})
				return ""
			}
			return text
		})

		for _, span := range inlineCode.FindAllStringSubmatch(prose, -1) {
			if isRepoDocPath(strings.TrimSpace(span[1])) {
				found = append(found, docPathViolation{line: i + 1, kind: "path named in prose", text: span[0]})
			}
		}
	}

	sort.SliceStable(found, func(a, b int) bool { return found[a].line < found[b].line })
	return found
}

func unquote(s string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "`"))
}

// isRepoDocPath reports whether s looks like a path to a document that prose
// could have linked to instead. The `.explain.md` sidecars and the crash
// report are files on a flashed card, not documents in this repository, so
// naming them is the only thing prose can do.
func isRepoDocPath(s string) bool {
	if !docPathPattern.MatchString(s) {
		return false
	}
	base := path.Base(s)
	return base != "explain.md" && !strings.HasSuffix(base, ".explain.md") && base != "LAST_FATAL_ERROR.md"
}
