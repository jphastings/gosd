---
# gosd-r91i
title: 'gosd init: write a starter gosd-build.toml'
status: in-progress
type: feature
created_at: 2026-08-30T20:55:28Z
updated_at: 2026-08-30T20:55:28Z
---

`gosd-build.toml` (schema in internal/buildconfig/config.go, documented with
a full commented example in docs/build-config.md) has no writer today: a
developer hand-copies the doc's example. `gosd init` writes a starter file
in the working directory, filling in whatever it can confirm and leaving
everything else as commented documentation, same shape as the doc's own
example. Plan reviewed and approved 2026-08-30.

## Locked decisions

- New files: cmd/gosd/init.go (newInitCmd, runInit, template constant +
  strings.NewReplacer rendering), cmd/gosd/initdetect.go (detection
  ladder), cmd/gosd/init_test.go. Plus one-line edits: cmd/gosd/main.go
  (register), cmd/gosd/pkgpath_test.go (add "init" to the flag-shaped-arg
  regression table), docs/build-config.md + README.md (one sentence each).
- Command shape mirrors `gosd build`: `Use: "init [path-to-main-package]"`,
  `Args: cobra.MaximumNArgs(1)`, one flag `--force`/`-f` (refuse to
  overwrite an existing gosd-build.toml without it — reuse the existing
  unexported `defaultBuildConfigFile` const from buildconfigfile.go, don't
  redeclare). Plain `os.WriteFile(..., 0o644)`, no temp+rename (nothing
  reads the file concurrently).
- Positional arg given: validate with the existing `validatePkgPath`
  (build.go) — same gosd-jc24 flag-injection defence. Not confirmed
  `package main` → fail fast, actionable error, write nothing.
- No arg — detection ladder: (1) is cwd itself `package main`? →
  `main = "."`. (2) else, is exactly one directory under `cmd/` `package
  main`? → `main = "./cmd/<name>"`. Zero or 2+ matches is ambiguous — don't
  guess. (3) else, leave `main` as a commented example line; cwd is still
  the basis for label-prefix and the git-tag check. No recursive repo-wide
  scan.
- Reuse the existing "is this package main" check rather than
  reimplementing it: split internal/build/build.go's `requireMainPackage`
  (go list -f '{{.Name}}' -- <pkgPath> under GOOS=linux, the real build
  target) into a shared `mainPackageName(pkgPath) (string, error)` plus two
  callers — the existing `requireMainPackage` (today's error) and a new
  exported `build.IsMainPackage(pkgPath string) bool` (swallows any error
  into false; detection is always best-effort). initdetect.go calls
  `build.IsMainPackage`.
- `[app].version`: new `gitversion.HasAnyTag(dir string) bool` in
  internal/gitversion (reuses `git.PlainOpenWithOptions(dir,
  &git.PlainOpenOptions{DetectDotGit: true})` + `repo.Tags()`, same as
  `Resolve`; false for "not a repo" or "zero tags" alike, never an error).
  Checked against the SAME directory the main-detection ladder settled on,
  not always cwd. True → write `version = "git:v*.*.*"` live. False → leave
  the doc's commented example. Deliberately NOT checking reachability from
  HEAD (that's Resolve's own job) — but MUST NOT default to a bare `git:`
  value without confirming at least one tag exists (would silently plant a
  guaranteed build failure in every fresh, untagged repo).
- `label-prefix`: always written live —
  `naming.LabelPrefix(naming.Sanitize(filepath.Base(dir)))`, exactly
  `deriveAppName`'s own logic, applied to whichever directory the ladder
  settled on (rung 3's cwd fallback still yields a usable basename). A
  short comment above it explains it's derived and is on-disk-layout ABI
  (per docs/build-config.md's own framing), pointing at the doc.
- Template content is docs/build-config.md's own example (lines 48-110),
  NOT copied verbatim: every line other than `main`, `version`, and
  `label-prefix` becomes a commented-out example (prefix `# `) — the doc's
  version has `board`/`output`/`ingress`/`placeholder`/`with-external` and
  every [boot]/[data]/[kernel]/[publish] key live as illustrative values,
  and copying those as-is would silently commit a fresh project to someone
  else's board list and tunnel choice. See the approved plan (below) for
  the exact target template text.
- Template storage: a plain Go backtick string constant in init.go, filled
  via `strings.NewReplacer` on three tokens ({{MAIN}}, {{VERSION}},
  {{LABEL_PREFIX}}) — not go:embed (no benefit for one string read once)
  and not text/template (project already chose plain literal substitution
  over text/template for --ldflags's {{.AppVersion}} token in
  resolveLDFlagsTemplate).
- On success, print one stdout line naming the file and next command, e.g.
  "gosd init: wrote gosd-build.toml — edit the commented options you need
  (see docs/build-config.md), then run 'gosd build'."
- Explicitly cut (YAGNI): recursive main-package search beyond the two
  rungs; reachability-from-HEAD checking for the version default;
  auto-detecting board/ingress/any other key with no reliable signal; an
  interactive/prompted mode, --dry-run, or an output-path flag; atomic
  write.

## Todos

[ ] internal/build: split requireMainPackage into mainPackageName + export
    IsMainPackage; internal/build tests for IsMainPackage
[ ] internal/gitversion: add HasAnyTag + tests (not-a-repo, zero-tag,
    lightweight-tag, annotated-tag) using the package's own newFixtureRepo
[ ] cmd/gosd/initdetect.go: detection ladder (main package, version,
    label-prefix)
[ ] cmd/gosd/init.go: newInitCmd, runInit, template constant + rendering,
    --force handling
[ ] cmd/gosd/main.go: register newInitCmd()
[ ] cmd/gosd/init_test.go: behavioral tests per the plan's verification
    list (parseable output on every branch, explicit-arg valid/invalid,
    flag-shaped-arg rejection, no-arg ladder rungs, tagged/untagged git,
    label-prefix derivation, overwrite refusal/--force, stdout message)
[ ] cmd/gosd/pkgpath_test.go: add "init" to the flag-shaped-arg regression
    table
[ ] docs/build-config.md + README.md: one-sentence pointers to `gosd init`
[ ] Quality gates: go test ./..., go vet ./..., gofmt -l ., golangci-lint
    run ./... and GOOS=linux golangci-lint run ./...
[ ] Changeset for the new user-facing command (or `no release notes` label
    if not warranted)

## Summary of Changes

