package main

import (
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/build"
	"github.com/jphastings/gosd/internal/gitversion"
	"github.com/jphastings/gosd/internal/naming"
)

// detectMainPackage returns the best-effort [app].main value gosd init
// should write (empty when nothing could be confirmed) and the directory
// that value — or cwd as a fallback — should be judged by for the
// label-prefix and version defaults too.
//
// Detection: (1) is cwd itself package main? -> ("." , cwd). (2) else, is
// exactly one directory entry directly under cwd/cmd/ package main? ->
// ("./cmd/<name>", cwd/cmd/<name>) -- zero or more than one match is
// ambiguous, don't guess. (3) else ("", cwd) -- main stays undetected but
// cwd still anchors the other two defaults.
func detectMainPackage(cwd string) (pkgPath, dir string) {
	if build.IsMainPackage(cwd) {
		return ".", cwd
	}

	entries, _ := os.ReadDir(filepath.Join(cwd, "cmd"))
	var matches []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if build.IsMainPackage(filepath.Join(cwd, "cmd", entry.Name())) {
			matches = append(matches, entry.Name())
		}
	}
	if len(matches) == 1 {
		return "./cmd/" + matches[0], filepath.Join(cwd, "cmd", matches[0])
	}

	return "", cwd
}

// detectLabelPrefix derives the default label-prefix for a gosd-build.toml
// generated for the app at dir: naming.LabelPrefix(naming.Sanitize(filepath.Base(dir))),
// exactly build.go's deriveAppName followed by naming.LabelPrefix. Always
// returns a usable value (naming.Sanitize never returns empty).
func detectLabelPrefix(dir string) string {
	return naming.LabelPrefix(naming.Sanitize(filepath.Base(dir)))
}

// detectVersionSource reports whether dir's enclosing git repository has
// at least one tag (gitversion.HasAnyTag) -- true means gosd init should
// default [app].version to "git:v*.*.*".
func detectVersionSource(dir string) bool {
	return gitversion.HasAnyTag(dir)
}
