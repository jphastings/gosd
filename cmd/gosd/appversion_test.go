package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// gitFixtureRepo makes a one-commit, one-tag repository with go-git, so no
// git binary is needed (the same hermeticity internal/gitversion's own
// tests rely on).
func gitFixtureRepo(t *testing.T, tag string) string {
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
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("main.go"); err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "fixture", Email: "fixture@example.com", When: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	hash, err := wt.Commit("initial", &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTag(tag, hash, &git.CreateTagOptions{Tagger: sig, Message: tag}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestResolveAppVersion(t *testing.T) {
	t.Run("literal values pass through uninterpreted", func(t *testing.T) {
		for _, raw := range []string{"", "1.4.2", "v2-rc1 with spaces"} {
			got, err := resolveAppVersion(raw, "github.com/you/app")
			if err != nil || got != raw {
				t.Errorf("resolveAppVersion(%q) = %q, %v; want it untouched", raw, got, err)
			}
		}
	})

	t.Run("a git: source refuses an import path actionably", func(t *testing.T) {
		_, err := resolveAppVersion("git:v*", "github.com/you/app")
		if err == nil || !strings.Contains(err.Error(), "local path") {
			t.Errorf("error = %v, want it to ask for a local path to the app", err)
		}
	})

	t.Run("a git: source resolves against the app directory", func(t *testing.T) {
		dir := gitFixtureRepo(t, "v2.5.0")
		got, err := resolveAppVersion("git:v*.*.*", dir)
		if err != nil || got != "2.5.0" {
			t.Errorf("resolveAppVersion = %q, %v; want 2.5.0", got, err)
		}
	})
}

func TestResolveLDFlagsTemplate(t *testing.T) {
	t.Run("no token passes ldflags through unchanged, even with no app version", func(t *testing.T) {
		for _, ldflags := range []string{"", "-s -w", "-X main.foo=bar"} {
			got, err := resolveLDFlagsTemplate(ldflags, "")
			if err != nil || got != ldflags {
				t.Errorf("resolveLDFlagsTemplate(%q, \"\") = %q, %v; want it untouched", ldflags, got, err)
			}
		}
	})

	t.Run("substitutes the token with the resolved app version", func(t *testing.T) {
		got, err := resolveLDFlagsTemplate("-X main.version={{.AppVersion}}", "1.4.2")
		if err != nil || got != "-X main.version=1.4.2" {
			t.Errorf("resolveLDFlagsTemplate = %q, %v; want the token substituted", got, err)
		}
	})

	t.Run("tolerates internal whitespace in the token", func(t *testing.T) {
		got, err := resolveLDFlagsTemplate("-X main.version={{ .AppVersion }}", "1.4.2")
		if err != nil || got != "-X main.version=1.4.2" {
			t.Errorf("resolveLDFlagsTemplate = %q, %v; want the whitespace-tolerant token substituted", got, err)
		}
	})

	t.Run("substitutes every occurrence", func(t *testing.T) {
		got, err := resolveLDFlagsTemplate("-X main.a={{.AppVersion}} -X main.b={{.AppVersion}}", "1.4.2")
		if err != nil || got != "-X main.a=1.4.2 -X main.b=1.4.2" {
			t.Errorf("resolveLDFlagsTemplate = %q, %v; want both occurrences substituted", got, err)
		}
	})

	t.Run("refuses the token when no app version was given", func(t *testing.T) {
		_, err := resolveLDFlagsTemplate("-X main.version={{.AppVersion}}", "")
		if err == nil || !strings.Contains(err.Error(), "--app-version") {
			t.Errorf("error = %v, want it to name --app-version", err)
		}
	})

	t.Run("refuses an unsupported template token", func(t *testing.T) {
		_, err := resolveLDFlagsTemplate("-X main.commit={{.GitCommit}}", "1.4.2")
		if err == nil || !strings.Contains(err.Error(), "{{.GitCommit}}") {
			t.Errorf("error = %v, want it to name the unsupported token", err)
		}
	})

	t.Run("refuses an unsupported token even alongside the supported one", func(t *testing.T) {
		_, err := resolveLDFlagsTemplate("-X main.version={{.AppVersion}} -X main.commit={{.GitCommit}}", "1.4.2")
		if err == nil || !strings.Contains(err.Error(), "{{.GitCommit}}") {
			t.Errorf("error = %v, want it to name the unsupported token even though {{.AppVersion}} is also present", err)
		}
	})
}
