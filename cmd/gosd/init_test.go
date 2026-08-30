package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"

	"github.com/jphastings/gosd/internal/buildconfig"
	"github.com/jphastings/gosd/internal/naming"
)

// initTestModule writes the minimal go.mod gosd init's main-package
// detection needs: build.IsMainPackage shells out to `go list`, which
// refuses to run outside a module.
func initTestModule(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module inittest\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGoFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitRepoNoTags is a git repository with zero tags, distinct from a
// directory that isn't a git repository at all - both must leave
// detectVersionSource false, but they're different failure paths inside it.
func gitRepoNoTags(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, false); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runGosdInit chdirs to dir and runs `gosd init` with the given extra args,
// returning stdout and the command's error.
func runGosdInit(t *testing.T, dir string, args ...string) (stdout string, err error) {
	t.Helper()
	t.Chdir(dir)
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"init"}, args...))
	err = cmd.Execute()
	return out.String(), err
}

// readInitConfig reads and parses the gosd-build.toml gosd init wrote in
// dir, failing the test if it doesn't parse - every rendered branch must
// satisfy this round-trip.
func readInitConfig(t *testing.T, dir string) buildconfig.Config {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, defaultBuildConfigFile))
	if err != nil {
		t.Fatalf("reading %s: %v", defaultBuildConfigFile, err)
	}
	cfg, err := buildconfig.Parse(data)
	if err != nil {
		t.Fatalf("gosd init wrote a %s that failed to parse: %v\n%s", defaultBuildConfigFile, err, data)
	}
	return cfg
}

func assertNoFileWritten(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, defaultBuildConfigFile)); !os.IsNotExist(err) {
		t.Errorf("%s exists after a refused gosd init, stat err = %v", defaultBuildConfigFile, err)
	}
}

// Case 1: an explicit arg naming a real package main directory writes a
// live main line, also exercising the stdout message (case 13).
func TestInitExplicitMainPackage(t *testing.T) {
	dir := t.TempDir()
	initTestModule(t, dir)
	writeGoFile(t, filepath.Join(dir, "app", "main.go"), "package main\n\nfunc main() {}\n")

	stdout, err := runGosdInit(t, dir, "./app")
	if err != nil {
		t.Fatalf("gosd init ./app failed: %v", err)
	}

	cfg := readInitConfig(t, dir)
	if cfg.App.Main != "./app" {
		t.Errorf("App.Main = %q, want %q", cfg.App.Main, "./app")
	}
	if !strings.Contains(stdout, "gosd-build.toml") || !strings.Contains(stdout, "gosd build") {
		t.Errorf("stdout = %q, want it to mention gosd-build.toml and gosd build", stdout)
	}
}

// Case 2: an explicit arg naming a non-main package is refused, and writes
// nothing.
func TestInitExplicitNonMainPackage(t *testing.T) {
	dir := t.TempDir()
	initTestModule(t, dir)
	writeGoFile(t, filepath.Join(dir, "lib", "lib.go"), "package lib\n")

	_, err := runGosdInit(t, dir, "./lib")
	if err == nil {
		t.Fatal("gosd init ./lib: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "package main") {
		t.Errorf("error %q does not say what's required", err)
	}
	assertNoFileWritten(t, dir)
}

// Case 3: a flag-shaped arg is refused by validatePkgPath before anything
// is written. The shared regression table in pkgpath_test.go covers the
// -toolexec payload specifically; this checks the write side too.
func TestInitExplicitFlagShapedArgIsRejected(t *testing.T) {
	dir := t.TempDir()

	_, err := runGosdInit(t, dir, "--", "-toolexec=/tmp/x")
	if err == nil {
		t.Fatal("gosd init -- -toolexec=/tmp/x: want an error, got nil")
	}
	assertNoFileWritten(t, dir)
}

// Case 4: with no arg, a package-main working directory writes main = ".".
func TestInitDetectsCwdMainPackage(t *testing.T) {
	dir := t.TempDir()
	initTestModule(t, dir)
	writeGoFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")

	if _, err := runGosdInit(t, dir); err != nil {
		t.Fatalf("gosd init failed: %v", err)
	}

	cfg := readInitConfig(t, dir)
	if cfg.App.Main != "." {
		t.Errorf("App.Main = %q, want \".\"", cfg.App.Main)
	}
}

// Case 5: with no arg, exactly one cmd/<name> main package is detected.
func TestInitDetectsSingleCmdSubdirectory(t *testing.T) {
	dir := t.TempDir()
	initTestModule(t, dir)
	writeGoFile(t, filepath.Join(dir, "cmd", "myapp", "main.go"), "package main\n\nfunc main() {}\n")

	if _, err := runGosdInit(t, dir); err != nil {
		t.Fatalf("gosd init failed: %v", err)
	}

	cfg := readInitConfig(t, dir)
	if cfg.App.Main != "./cmd/myapp" {
		t.Errorf("App.Main = %q, want \"./cmd/myapp\"", cfg.App.Main)
	}
}

// Case 6: zero main candidates under cmd/ - missing, empty, or present with
// only non-main packages - all leave main undetected.
func TestInitLeavesMainCommentedWithNoCandidates(t *testing.T) {
	t.Run("cmd directory missing", func(t *testing.T) {
		dir := t.TempDir()
		initTestModule(t, dir)

		if _, err := runGosdInit(t, dir); err != nil {
			t.Fatalf("gosd init failed: %v", err)
		}
		if cfg := readInitConfig(t, dir); cfg.App.Main != "" {
			t.Errorf("App.Main = %q, want it left undetected", cfg.App.Main)
		}
	})

	t.Run("cmd directory empty", func(t *testing.T) {
		dir := t.TempDir()
		initTestModule(t, dir)
		if err := os.Mkdir(filepath.Join(dir, "cmd"), 0o755); err != nil {
			t.Fatal(err)
		}

		if _, err := runGosdInit(t, dir); err != nil {
			t.Fatalf("gosd init failed: %v", err)
		}
		if cfg := readInitConfig(t, dir); cfg.App.Main != "" {
			t.Errorf("App.Main = %q, want it left undetected", cfg.App.Main)
		}
	})

	t.Run("cmd directory has no main candidates", func(t *testing.T) {
		dir := t.TempDir()
		initTestModule(t, dir)
		writeGoFile(t, filepath.Join(dir, "cmd", "util", "util.go"), "package util\n")

		if _, err := runGosdInit(t, dir); err != nil {
			t.Fatalf("gosd init failed: %v", err)
		}
		if cfg := readInitConfig(t, dir); cfg.App.Main != "" {
			t.Errorf("App.Main = %q, want it left undetected", cfg.App.Main)
		}
	})
}

// Case 7: two or more main candidates under cmd/ are ambiguous, so main
// stays undetected rather than guessing.
func TestInitLeavesMainCommentedWithAmbiguousCandidates(t *testing.T) {
	dir := t.TempDir()
	initTestModule(t, dir)
	writeGoFile(t, filepath.Join(dir, "cmd", "foo", "main.go"), "package main\n\nfunc main() {}\n")
	writeGoFile(t, filepath.Join(dir, "cmd", "bar", "main.go"), "package main\n\nfunc main() {}\n")

	if _, err := runGosdInit(t, dir); err != nil {
		t.Fatalf("gosd init failed: %v", err)
	}

	if cfg := readInitConfig(t, dir); cfg.App.Main != "" {
		t.Errorf("App.Main = %q, want it left undetected (ambiguous)", cfg.App.Main)
	}
}

// Case 8: a repository with at least one tag defaults version to the git:
// source.
func TestInitDetectsVersionFromTaggedRepo(t *testing.T) {
	dir := gitFixtureRepo(t, "v1.0.0")

	if _, err := runGosdInit(t, dir); err != nil {
		t.Fatalf("gosd init failed: %v", err)
	}

	cfg := readInitConfig(t, dir)
	if cfg.App.Version != "git:v*.*.*" {
		t.Errorf("App.Version = %q, want \"git:v*.*.*\"", cfg.App.Version)
	}
}

// Case 9: a tag-less git repo and a plain non-git directory both leave
// version undetected.
func TestInitLeavesVersionCommentedWithoutTags(t *testing.T) {
	t.Run("git repo with no tags", func(t *testing.T) {
		dir := gitRepoNoTags(t)
		if _, err := runGosdInit(t, dir); err != nil {
			t.Fatalf("gosd init failed: %v", err)
		}
		if cfg := readInitConfig(t, dir); cfg.IsSet("app.version") {
			t.Errorf("App.Version = %q, want it left undetected", cfg.App.Version)
		}
	})

	t.Run("not a git repository", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := runGosdInit(t, dir); err != nil {
			t.Fatalf("gosd init failed: %v", err)
		}
		if cfg := readInitConfig(t, dir); cfg.IsSet("app.version") {
			t.Errorf("App.Version = %q, want it left undetected", cfg.App.Version)
		}
	})
}

// Case 10: label-prefix always matches naming.LabelPrefix(naming.Sanitize(...))
// of the directory gosd init resolved.
func TestInitLabelPrefixMatchesDirectoryName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "My Cool App")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := runGosdInit(t, dir); err != nil {
		t.Fatalf("gosd init failed: %v", err)
	}

	cfg := readInitConfig(t, dir)
	want := naming.LabelPrefix(naming.Sanitize(filepath.Base(dir)))
	if cfg.LabelPrefix != want {
		t.Errorf("LabelPrefix = %q, want %q", cfg.LabelPrefix, want)
	}
}

// Case 11: an existing gosd-build.toml is left untouched without --force.
func TestInitRefusesToOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	existing := "board = [\"pi-zero-2w\"]\n"
	if err := os.WriteFile(filepath.Join(dir, defaultBuildConfigFile), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runGosdInit(t, dir)
	if err == nil {
		t.Fatal("gosd init over an existing gosd-build.toml: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error %q does not name --force as the fix", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, defaultBuildConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != existing {
		t.Errorf("gosd-build.toml changed after a refused init: %q", got)
	}
}

// Case 12: --force overwrites an existing gosd-build.toml with freshly
// rendered content.
func TestInitForceOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	existing := "board = [\"pi-zero-2w\"]\n"
	if err := os.WriteFile(filepath.Join(dir, defaultBuildConfigFile), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runGosdInit(t, dir, "--force"); err != nil {
		t.Fatalf("gosd init --force failed: %v", err)
	}

	cfg := readInitConfig(t, dir)
	if cfg.IsSet("board") {
		t.Error("gosd-build.toml still carries the old board key; --force did not overwrite it")
	}

	got, err := os.ReadFile(filepath.Join(dir, defaultBuildConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == existing {
		t.Error("gosd-build.toml was not overwritten by --force")
	}
}

// commentedKeyLineRe matches a commented "# key = value" line in the
// rendered template, whether it sits directly under a header comment (a
// single space after #, e.g. "# board = [...]") or indented under the
// "Also honoured" block ("#   usb-gadget = true").
var commentedKeyLineRe = regexp.MustCompile(`^#\s+([a-z0-9-]+) = `)

// TestInitTemplateCommentedLinesUncommentCleanly guards against the exact
// bug class found in review: a commented key line that's misplaced (e.g.
// under the wrong [section]) or misspelled parses fine as a comment, so
// nothing catches it until a developer uncomments it. This renders the most
// comment-dense case (main and version both undetected), then uncomments
// each commented key line one at a time - leaving every other line exactly
// as rendered - and asserts the result still parses.
func TestInitTemplateCommentedLinesUncommentCleanly(t *testing.T) {
	rendered := renderInitTemplate("", false, "myapp")
	lines := strings.Split(rendered, "\n")

	tested := 0
	for i, line := range lines {
		m := commentedKeyLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		tested++

		uncommented := slices.Clone(lines)
		uncommented[i] = strings.TrimPrefix(line, "#")
		candidate := strings.Join(uncommented, "\n")

		if _, err := buildconfig.Parse([]byte(candidate)); err != nil {
			t.Errorf("uncommenting key %q (line %q) failed to parse: %v", m[1], line, err)
		}
	}

	if tested == 0 {
		t.Fatal("no commented key lines matched in the rendered template; commentedKeyLineRe or the template's shape changed")
	}
}

// TestInitRenderedTemplateAlwaysParses is the round-trip requirement the
// brief calls out explicitly: every combination of main/version
// detected-or-not must parse cleanly through buildconfig.Parse, not just
// the combinations the detection-ladder tests above happen to exercise.
func TestInitRenderedTemplateAlwaysParses(t *testing.T) {
	for _, mainDetected := range []bool{true, false} {
		for _, versionDetected := range []bool{true, false} {
			pkgPath := ""
			if mainDetected {
				pkgPath = "./cmd/myapp"
			}
			rendered := renderInitTemplate(pkgPath, versionDetected, "myapp")
			if _, err := buildconfig.Parse([]byte(rendered)); err != nil {
				t.Errorf("main detected=%v, version detected=%v: rendered gosd-build.toml failed to parse: %v\n%s",
					mainDetected, versionDetected, err, rendered)
			}
			if !strings.Contains(rendered, "{{.AppVersion}}") {
				t.Error("rendered template lost the literal {{.AppVersion}} --ldflags example")
			}
		}
	}
}
