package repocheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// changesetNotePackage is the one package whose change files may use the `note`
// bump type: knope.toml declares `extra_changelog_sections` under
// `[packages.gosd]` and nowhere else.
const changesetNotePackage = "gosd"

// TestChangeFilesAreValid asserts every change file in the tree says what its
// author meant it to say. It complements .github/workflows/change-file-check.yml
// rather than replacing it: that workflow's question ("does this PR's diff add a
// change file?") is inherently base-ref-scoped and stays a workflow, while this
// one asks whether the change files sitting in the tree are valid at all.
//
// The frontmatter is deliberately NOT parsed as YAML. yaml.Unmarshal maps
// `"npm/gosd": patch` and `npm/gosd: patch` onto the same Go map key, so a YAML
// round-trip is structurally blind to exactly the quoting knope ignores — the
// silent failure this test exists to catch. The check is lexical, on raw lines.
func TestChangeFilesAreValid(t *testing.T) {
	packages := knopePackageKeys(t)

	files, err := filepath.Glob(filepath.Join("..", "..", ".changeset", "*.md"))
	if err != nil {
		t.Fatalf("globbing .changeset: %v", err)
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading %s: %v", file, err)
			}
			for _, problem := range changeFileProblems(string(content), packages) {
				t.Errorf("%s %s", file, problem)
			}
		})
	}
}

// TestChangeFileProblems keeps the check honest while .changeset is empty, which
// it is for most of a release cycle: without it a typo in the patterns below
// would leave every change file passing vacuously.
func TestChangeFileProblems(t *testing.T) {
	packages := map[string]bool{changesetNotePackage: true, "npm/gosd": true}
	valid := "---\ngosd: minor\nnpm/gosd: patch\n---\n\n#### A short title\n\nProse.\n"

	tests := []struct {
		name    string
		content string
		want    string // substring of the expected problem; empty means no problem
	}{
		{name: "valid", content: valid},
		{name: "note on gosd", content: strings.Replace(valid, "gosd: minor", "gosd: note", 1)},
		{name: "quoted key", content: strings.Replace(valid, "npm/gosd: patch", `"npm/gosd": patch`, 1), want: "no package's release at all"},
		{name: "unknown package", content: strings.Replace(valid, "npm/gosd: patch", "gsod: patch", 1), want: "knope.toml does not declare"},
		{name: "unknown bump type", content: strings.Replace(valid, "gosd: minor", "gosd: breaking", 1), want: "is not `<package>:"},
		{name: "trailing whitespace", content: strings.Replace(valid, "gosd: minor", "gosd: minor ", 1), want: "is not `<package>:"},
		{name: "note on another package", content: strings.Replace(valid, "npm/gosd: patch", "npm/gosd: note", 1), want: "`note` bump type"},
		{name: "no packages", content: "---\n---\n\n#### A short title\n", want: "declares no package bumps"},
		{name: "no frontmatter", content: "#### A short title\n\nProse.\n", want: "must open with a `---`"},
		{name: "unterminated frontmatter", content: "---\ngosd: minor\n\n#### A short title\n", want: "never closes its frontmatter"},
		{name: "body without a heading", content: strings.Replace(valid, "#### A short title", "A short title", 1), want: "prescribes a `#### ` heading"},
		{name: "no body", content: "---\ngosd: minor\n---\n\n", want: "no body below its frontmatter"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			problems := changeFileProblems(test.content, packages)
			joined := strings.Join(problems, "\n")

			switch {
			case test.want == "" && len(problems) > 0:
				t.Errorf("valid change file reported as broken: %s", joined)
			case test.want != "" && !strings.Contains(joined, test.want):
				t.Errorf("problem should mention %q, got: %s", test.want, joined)
			}
		})
	}
}

// knopePackageKeys reads the package names knope accepts out of knope.toml's
// [packages.*] table headers rather than hardcoding them (no TOML dependency:
// the repo has none, and this doesn't justify one). Deriving them also documents
// the trap — TOML *requires* the header `[packages."npm/gosd"]` to be quoted,
// while a change file's key for that same package must not be.
func knopePackageKeys(t *testing.T) map[string]bool {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "knope.toml"))
	if err != nil {
		t.Fatalf("reading knope.toml: %v", err)
	}

	header := regexp.MustCompile(`(?m)^\[packages\.(?:"([^"]+)"|([A-Za-z0-9_-]+))\]`)
	keys := make(map[string]bool)
	for _, match := range header.FindAllStringSubmatch(string(data), -1) {
		key := match[1]
		if key == "" {
			key = match[2]
		}
		keys[key] = true
	}

	if !keys[changesetNotePackage] {
		t.Fatalf("knope.toml declares no [packages.%s] (found %s); either the package was renamed or this test's header pattern no longer matches it", changesetNotePackage, knopePackageList(keys))
	}
	return keys
}

var (
	changeFileEntry = regexp.MustCompile(`^([A-Za-z0-9/_.-]+): +(major|minor|patch|note)$`)
	// changeFileEntry's key group excludes quote characters, so a quoted key is
	// simply a line that does not match. changeFileQuotedKey recognises that
	// case afterwards, to explain what knope will do with it.
	changeFileQuotedKey = regexp.MustCompile(`^\s*["'][^"']+["']\s*:`)
)

func malformedEntryProblem(line int, text string) string {
	if changeFileQuotedKey.MatchString(text) {
		return fmt.Sprintf("line %d, %q, quotes its package key. That is valid YAML, so knope parses the change file without complaint and then contributes it to no package's release at all — no version bump, no changelog entry, no error. Write the key unquoted (see docs/releasing.md).", line, text)
	}
	return fmt.Sprintf("line %d, %q, is not `<package>: <major|minor|patch|note>` (see docs/releasing.md)", line, text)
}

// changeFileProblems returns everything wrong with one change file's content,
// phrased for someone about to push it, and nothing if it is valid.
func changeFileProblems(content string, packages map[string]bool) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if lines[0] != "---" {
		return []string{"must open with a `---` frontmatter fence on line 1 (see docs/releasing.md)"}
	}
	end := 0
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end == 0 {
		return []string{"never closes its frontmatter with a `---` line (see docs/releasing.md)"}
	}

	var problems []string
	bumps := 0
	for i, line := range lines[1:end] {
		if strings.TrimSpace(line) == "" {
			continue
		}

		match := changeFileEntry.FindStringSubmatch(line)
		if match == nil {
			problems = append(problems, malformedEntryProblem(i+2, line))
			continue
		}

		key, bump := match[1], match[2]
		if !packages[key] {
			problems = append(problems, fmt.Sprintf("line %d names package %q, which knope.toml does not declare; the valid keys are %s", i+2, key, knopePackageList(packages)))
		}
		if bump == "note" && key != changesetNotePackage {
			problems = append(problems, fmt.Sprintf("line %d gives %q the `note` bump type, which only %q accepts (knope.toml declares extra_changelog_sections under [packages.%s] alone)", i+2, key, changesetNotePackage, changesetNotePackage))
		}
		bumps++
	}
	if bumps == 0 && len(problems) == 0 {
		problems = append(problems, fmt.Sprintf("declares no package bumps, so it would release nothing; list at least one of %s (see docs/releasing.md)", knopePackageList(packages)))
	}

	return append(problems, changeFileBodyProblems(lines[end+1:])...)
}

func changeFileBodyProblems(body []string) []string {
	for _, line := range body {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "#### ") {
			return []string{fmt.Sprintf("starts its body with %q; docs/releasing.md prescribes a `#### ` heading, which becomes the release note's title in CHANGELOG.md", line)}
		}
		return nil
	}
	return []string{"has no body below its frontmatter; the heading and prose there are the release note"}
}

func knopePackageList(keys map[string]bool) string {
	names := make([]string, 0, len(keys))
	for key := range keys {
		names = append(names, key)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}
