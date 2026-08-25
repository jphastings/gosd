package main

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/jphastings/gosd/internal/gitversion"
)

// resolveAppVersion turns a git:-scheme --app-version (or gosd-build.toml
// [app] version) into a concrete version string from the app repository's
// tags. Any other value passes through untouched — gosd resolves a git:
// source but still never interprets the resulting version.
func resolveAppVersion(raw, pkgPath string) (string, error) {
	if !gitversion.IsGitSource(raw) {
		return raw, nil
	}
	if !filesystemPathLike(pkgPath) {
		return "", fmt.Errorf(
			"--app-version %s resolves from the app repository's tags, which needs the app named by a local path (\".\", \"./cmd/myapp\", or absolute) rather than the import path %q; build from a checkout of the app instead",
			raw, pkgPath)
	}
	dir, err := filepath.Abs(pkgPath)
	if err != nil {
		return "", fmt.Errorf("--app-version %s: resolving %q to an absolute path failed: %w", raw, pkgPath, err)
	}
	return gitversion.Resolve(raw, dir)
}

// ldflagsAppVersionTokenPattern matches --ldflags's one supported template
// token, {{.AppVersion}}, tolerating internal whitespace ({{ .AppVersion }}
// works the same as {{.AppVersion}}) the way a real Go template would -
// though this is a literal regex match, not text/template execution: no
// other field names, pipelines, or functions are supported.
var ldflagsAppVersionTokenPattern = regexp.MustCompile(`\{\{\s*\.AppVersion\s*\}\}`)

// templateTokenPattern matches any {{...}}-shaped substring in --ldflags,
// so an unsupported or typo'd token (e.g. {{.GitCommit}}, {{.AppVerson}})
// is refused rather than passed to go build as a literal, silently-wrong
// -ldflags value.
var templateTokenPattern = regexp.MustCompile(`\{\{[^}]*\}\}`)

// resolveLDFlagsTemplate substitutes every {{.AppVersion}} in ldflags with
// appVersion - already fully resolved by resolveAppVersion, including any
// git: resolution - so --ldflags="-X main.version={{.AppVersion}}" stamps
// the same version --app-version resolved into the compiled binary,
// without the caller having to resolve it a second time. ldflags with no
// template token is returned unchanged, even when appVersion is empty. Any
// {{...}}-shaped substring that isn't {{.AppVersion}} is refused, and so is
// {{.AppVersion}} itself when appVersion is empty - either would otherwise
// reach `go build` as a literal, silently-wrong -ldflags value.
func resolveLDFlagsTemplate(ldflags, appVersion string) (string, error) {
	matches := templateTokenPattern.FindAllString(ldflags, -1)
	if len(matches) == 0 {
		return ldflags, nil
	}
	for _, m := range matches {
		if !ldflagsAppVersionTokenPattern.MatchString(m) {
			return "", fmt.Errorf(
				"--ldflags contains %q, which is not a recognized template token; {{.AppVersion}} is the only one --ldflags supports",
				m)
		}
	}
	if appVersion == "" {
		return "", fmt.Errorf(
			"--ldflags references {{.AppVersion}} but no --app-version (or gosd-build.toml's app.version) was given a value to substitute; pass --app-version, or remove {{.AppVersion}} from --ldflags")
	}
	// ReplaceAllLiteralString, not ReplaceAllString: appVersion may itself
	// contain regexp replacement metacharacters and must never have them
	// interpreted.
	return ldflagsAppVersionTokenPattern.ReplaceAllLiteralString(ldflags, appVersion), nil
}
