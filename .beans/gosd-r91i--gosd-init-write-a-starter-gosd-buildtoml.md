---
# gosd-r91i
title: 'gosd init: write a starter gosd-build.toml'
status: completed
type: feature
priority: normal
created_at: 2026-08-30T20:55:28Z
updated_at: 2026-08-30T22:26:23Z
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
  else's board list and tunnel choice.
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

[x] internal/build: split requireMainPackage into mainPackageName + export
    IsMainPackage; internal/build tests for IsMainPackage
[x] internal/gitversion: add HasAnyTag + tests (not-a-repo, zero-tag,
    lightweight-tag, annotated-tag) using the package's own newFixtureRepo
[x] cmd/gosd/initdetect.go: detection ladder (main package, version,
    label-prefix)
[x] cmd/gosd/init.go: newInitCmd, runInit, template constant + rendering,
    --force handling
[x] cmd/gosd/main.go: register newInitCmd()
[x] cmd/gosd/init_test.go: behavioral tests per the plan's verification
    list (parseable output on every branch, explicit-arg valid/invalid,
    flag-shaped-arg rejection, no-arg ladder rungs, tagged/untagged git,
    label-prefix derivation, overwrite refusal/--force, stdout message)
[x] cmd/gosd/pkgpath_test.go: add "init" to the flag-shaped-arg regression
    table
[x] docs/build-config.md + README.md: one-sentence pointers to `gosd init`
[x] Quality gates: go test ./..., go vet ./..., gofmt -l ., golangci-lint
    run ./... and GOOS=linux golangci-lint run ./...
[x] Changeset for the new user-facing command (or `no release notes` label
    if not warranted)

## Summary of Changes

Implemented via subagent-driven-development: 4 tasks, each with its own
implementer + task review, then a final whole-branch review (opus) with
one consolidated fix wave + scoped re-review. Every stage's review came
back clean (task reviews: 3 clean, 1 with 2 minors deferred, 1 with a
finding adjudicated as a reviewer misread; final review: approved "with
fixes," fix wave addressed all 6 findings, re-review confirmed clean).

- `internal/build`: split `requireMainPackage` into a shared
  `mainPackageName` helper plus the existing error-producing caller and a
  new exported `IsMainPackage(pkgPath string) bool` for best-effort
  detection — `requireMainPackage`'s behavior/error text is byte-for-byte
  unchanged, `CrossCompile`'s call site untouched.
- `internal/gitversion`: added `HasAnyTag(dir string) bool`, reusing
  `Resolve`'s git-opening approach; stops on the first tag found via
  go-git's `storer.ErrStop`, verified correct against the installed
  go-git v5.17.1 source.
- `cmd/gosd`: new `init` command (`cmd/gosd/init.go`,
  `cmd/gosd/initdetect.go`). `gosd init [path-to-main-package]` writes a
  starter `gosd-build.toml`, mirroring `docs/build-config.md`'s own
  example with every non-detected line commented out. Detects `[app].main`
  (cwd itself, else a single unambiguous `cmd/*` subdirectory, else left
  commented), `[app].version` (live `git:v*.*.*` only when the repo has a
  confirmed tag), and `label-prefix` (always derived, since it's on-disk
  ABI). Explicit positional args go through the existing `validatePkgPath`
  security check before anything else. Refuses to overwrite an existing
  file without `--force`/`-f`; the overwrite check runs before any
  detection work (including `go list` subprocess calls) in both the arg
  and no-arg paths, while `validatePkgPath` still runs first on the
  explicit-arg path. A dedicated test renders the template and uncomments
  every commented key individually to confirm each still parses under its
  correct table — the regression test for the one real bug the final
  review caught (a block of top-level keys briefly sitting under `[publish]`
  post-render).
- Manually verified end-to-end (built the binary, ran `gosd init` then
  `gosd build` against a scratch module): the generated file round-trips
  correctly through the reader.
- Docs: one-sentence pointers to `gosd init` in `docs/build-config.md` and
  `README.md`. Changeset: `.changeset/gosd-r91i.md` (minor bump, new
  user-facing command).
