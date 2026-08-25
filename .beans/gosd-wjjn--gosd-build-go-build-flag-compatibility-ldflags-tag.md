---
# gosd-wjjn
title: 'gosd build: go build flag compatibility (--ldflags, --tags, --trimpath, --gcflags, --asmflags)'
status: completed
type: feature
priority: normal
created_at: 2026-08-24T10:38:27Z
updated_at: 2026-08-25T06:19:06Z
---

`gosd build`'s app-compile step doesn't accept the `go build` flags a Go
developer expects (`-ldflags`, `-tags`, `-trimpath`, `-gcflags`, `-asmflags`).
Add them for their own sake — `gosd build` already shells out to a real
`go build` for the app, so a developer's existing muscle memory should just
work on it. That goreleaser's `go` builder (`tool: gosd`) would then also
work is a side effect of getting this right, not the reason to do it — see
atfs's `ATFS-aogu` for the consumer-side context that
originally prompted this.

Two separable increments.

## Increment A — `--ldflags` (has a concrete motivating gap today)

Closes an existing gap: a gosd-built image never stamps a version into the
compiled app binary at all, unlike `make build`/`flake.nix` in consumer repos
(e.g. atfs), which both stamp `-X main.version=...`. `--app-version` doesn't
help here — it bakes into `config.json`/crash reports, never into the
compiled binary.

Design:

- New `internal/build.AppCompileOptions{Tags, LDFlags string}` +
  `appGoBuildArgs(outputPath, pkgPath string, opts AppCompileOptions) []string`
  in `internal/build/build.go` — a *new*, purpose-built pair, **not** a reuse
  of `crossCompileOpts`/`buildGoBuildArgs` from
  `internal/build/gosdinit.go:91-140`. That pair always emits `-C <dir>` and
  resolves an absolute output path, both specific to building gosd-init/
  tsfunnel from a *different*, detected/downloaded source tree. Forcing the
  app's `CrossCompile` through it would mean a spurious `-C ""` or a real,
  unwanted `-C .` behavior change — the app always builds from the caller's
  own working directory, using `pkgPath`/`outputPath` exactly as given.
- `CrossCompile(pkgPath, outputPath string, opts AppCompileOptions, arch boards.Arch) error`
  (was `tags string`) — ripples through:
  - `compileForBoards`'s `compileApp` func-type param
    (`cmd/gosd/archbuild.go:44-51`)
  - its callsite in `runBuild` (`cmd/gosd/build.go:303`)
  - the direct `CrossCompile` call in `gosd run` (`cmd/gosd/run.go:160`) —
    `gosd run` gets **no** `--ldflags` flag of its own (mirrors its existing
    "not every build flag is mirrored here" precedent at run.go:118-120),
    just the mechanical signature update.
- New `--ldflags` flag registered in `newBuildCmd()`
  (`cmd/gosd/build.go:127-169`), following the exact existing `StringVar`
  idiom used by every other flag there.
- A `gosd-build.toml` key too: `[app] ldflags`, per the file's documented
  flag↔key mapping (`docs/build-config.md`).

Tests:

- Update the 6 direct `CrossCompile(...)` call sites in
  `internal/build/build_test.go`.
- New `appGoBuildArgs` unit tests mirroring
  `TestBuildGoBuildArgsOmitsTagsAndLdflagsWhenOptsIsZero`
  (`internal/build/gosdinit_test.go:112-130`).
- Update `archbuild_test.go`'s `countingCompiler`/`appCall`/inline fakes and
  the 8 `compileForBoards(...)` call sites for the new param.
- New `internal/build/testdata/versioned` fixture (a small `main.go` with an
  exported `version` var — `testdata/hello` has nothing to target) +
  `TestCrossCompileAppliesLDFlags`, compiling with
  `AppCompileOptions{LDFlags: "-X main.version=stamped"}` and inspecting the
  output binary for the stamped string.
- A flag-registration test (`Lookup("ldflags")`, `DefValue == ""`), matching
  `TestGosdInitSrcFlagDefaultsToEnv`'s pattern (`build_test.go:891-901`).
- Ideally a `build_integration_test.go` case building end-to-end and
  asserting the stamped string lands in the built image.

## Increment B — `--tags`, `--trimpath`, `--gcflags`, `--asmflags` (no current caller)

Build only alongside an actual decision to drive `gosd build` through
goreleaser's `go` builder specifically, or if the general-compatibility
motivation alone is judged worth it on its own. Nothing in gosd or any
consumer needs these four today.

Design:

- `AppCompileOptions` grows `GCFlags, ASMFlags string`, `TrimPath bool`
  alongside Increment A's `Tags`/`LDFlags`; `appGoBuildArgs` appends each
  when set, in a fixed order, before the package path.
- `--tags` is a plain `StringVar` (not `StringArrayVar` — unlike
  `--board`/`--env`/etc, `go build -tags` itself takes exactly one string
  value). **Merge, don't replace**: comma-join the caller's value onto the
  mandatory `gosd,gosd_<board>` tags (`boards.BuildTags(b)`,
  `internal/boards/boards.go:259-261`) — never let a caller's `--tags`
  silently drop gosd's own board-gating tags.
- Validate and reject, don't defensively dedupe: a caller-supplied
  `gosd`/`gosd_*`-namespaced token is always either a no-op (redundant with
  the tag `compileForBoards` was going to add anyway) or, in a multi-`--board`
  build, a real but wrong tag for boards other than the one it happened to
  match — worth catching at flag-parse time. Mirrors `parseEnvFlags`'s
  existing `GOSD_`-prefix rejection (`cmd/gosd/build.go:741-743`). New
  `parseExtraTags(raw string) ([]string, error)`: split on comma/whitespace
  (go build -tags accepts either), trim, dedupe, reject any `"gosd"` or
  `"gosd_"`-prefixed token — reject by namespace, not by matching a
  currently-registered board, so it stays forward-compatible with future
  boards. Call it early in `runBuild`, alongside the other `parseXFlags`
  calls, before board resolution, so a bad `--tags` value fails before any
  board is even touched.
- `--trimpath`/`--gcflags`/`--asmflags` need no gosd-side validation beyond
  "append when set" — no reserved namespace, and since gosd invokes `go` via
  `exec.Command("go", args...)` (never through a shell), any value arrives
  and forwards as a single argv token exactly as `tsfunnelLDFlags = "-s -w"`
  already proves works today (`tsfunnel.go:31`) — no new quoting/escaping
  design needed.
- `compileForBoards` gains four more flat positional params (`extraTags
  []string, gcflags, asmflags string, trimpath bool`), matching its existing
  flat-param style (it already takes `tempDir, pkgPath, gosdInitSrc,
  needsTsfunnel` flat, not a struct) — not a struct refactor of the whole
  function.

Tests:

- Pure-function tests for `parseExtraTags`: nil/empty, comma- and
  space-separated splitting, dedup, rejects `"gosd"` exactly, rejects any
  `"gosd_"`-prefixed token including ones matching no registered board.
- Extend the `appGoBuildArgs` tests for ordering + omit-when-unset, one per
  new field.
- `archbuild_test.go`: extend `appCall`/`countingCompiler.compileApp` to
  capture `GCFlags`/`ASMFlags`/`TrimPath`; new
  `TestCompileForBoardsMergesExtraTagsWithMandatoryBoardTags`, asserting each
  board's merged tags contain both the mandatory tokens and every
  `extraTags` entry (mirrors `TestCompileForBoardsTagsEveryAppCompileWithTheBareGosdTag`,
  `archbuild_test.go:219-232`); update all 8 `compileForBoards(...)` call
  sites again.
- Flag-registration tests for the three new flags.
- Integration-level: a `--tags` merge case, and a case confirming
  `--tags gosd_pi-zero-2w` is rejected with an actionable error before any
  image bytes exist (mirrors `TestBuildRefusesADataSizeFAT32CannotHold`'s
  shape, `build_integration_test.go:391-414`).
- `docs/board-build-tags.md` gets a short new section documenting the
  merge-not-replace behavior and the reserved-namespace rejection — it's
  currently silent on a caller being able to add tags at all.

## Non-issues, confirmed, not touched by either increment

- Ambient `GOOS`/`GOARCH`/`CGO_ENABLED` in a caller's process environment
  before invoking `gosd build`: already harmless. `archEnv()`
  (`internal/build/build.go:57-67`) appends gosd's own values last, and
  `exec.Cmd.Env` is last-wins.
- `gosd build -o <path>` always producing a full bootable disk image, never a
  bare Go binary, isn't a technical blocker for a goreleaser-shaped caller —
  goreleaser doesn't inspect file contents, just archives/checksums whatever
  bytes are at the path.
- `--board` stays the sole board selector, validated against the closed
  `internal/boards` registry — neither increment touches board selection or
  collapses it onto GOOS/GOARCH.

## Correction while implementing (2026-08-24)

The bean's proposed `[app] ldflags`/`[app] tags`/etc. gosd-build.toml keys
conflict with a real, already-enforced locked invariant this repo has
(bean gosd-mwct, pinned by `cmd/gosd/buildconfigfile_test.go`'s
`TestFlagKeyParityIsStructural`): the flag<->key mapping is *purely
structural* — a flag `--<section>-<rest>` maps to `[section].rest` only
when `<rest>` is genuinely a `--app-*`-style flag. Since this bean's design
deliberately keeps the flags bare (`--ldflags`, `--tags`, `--trimpath`,
`--gcflags`, `--asmflags` — exactly `go build`'s own flag names, for
muscle-memory reasons the bean itself states), the structural rule places
them as **top-level** gosd-build.toml keys (`ldflags`, `tags`, `trimpath`,
`gcflags`, `asmflags`), not nested under `[app]`. This keeps both the
muscle-memory flag names and the existing structural-parity test intact;
renaming the flags to `--app-ldflags` etc. to fit `[app]` would have
defeated the bean's own stated motivation. No other design bullet is
affected. Also: several bean cross-references (`cmd/gosd/build.go:303`,
`run.go:160`, `build.go:741-743`'s `parseEnvFlags`) point at line numbers/
a helper that don't exist in the current tree (there is no `--env` flag or
`parseEnvFlags`); the *entities* they describe (the `compileForBoards`
callsite, the direct `CrossCompile` call in `gosd run`, and the
reserved-namespace-rejection pattern precedent set by
`validateDataFilesystemSupport`/`validateUsbGadget`'s capability-refusal
shape) all still exist and the design against them still holds — only the
citations were stale.

## Summary of Changes

Implemented both increments in one PR (per JP's answer when asked about
scope):

- `internal/build.AppCompileOptions{Tags, LDFlags, GCFlags, ASMFlags,
  TrimPath}` + pure-function `appGoBuildArgs`, replacing `CrossCompile`'s old
  bare `tags string` param. All direct `CrossCompile` call sites updated:
  `internal/build/build_test.go` (6), `internal/build/injection_test.go` (2,
  not previously enumerated by the bean), `cmd/gosd/run.go` (mechanical, no
  new flag exposed), `cmd/gosd/withexternal_integration_test.go` (1, not
  previously enumerated).
- `compileForBoards` gains `ldflags string, extraTags []string, gcflags,
  asmflags string, trimpath bool`; per-board `Tags` merges `boards.BuildTags`
  with `extraTags` rather than replacing it. All 8
  `cmd/gosd/archbuild_test.go` call sites and its `countingCompiler`/
  `appCall` fakes updated; two new tests added
  (`TestCompileForBoardsMergesExtraTagsWithMandatoryBoardTags`,
  `TestCompileForBoardsPassesLDFlagsGCFlagsASMFlagsTrimPathThrough`).
- New `--ldflags`, `--tags`, `--trimpath`, `--gcflags`, `--asmflags` flags on
  `gosd build` (none mirrored on `gosd run`, matching its existing
  not-every-flag precedent). `parseExtraTags` validates/splits `--tags` and
  rejects any `gosd`/`gosd_`-namespaced token by prefix (forward-compatible
  with future boards), called early in `runBuild` before board resolution.
- `gosd-build.toml` gains matching **top-level** keys (`ldflags`, `tags`,
  `trimpath`, `gcflags`, `asmflags`) — see the correction note above for why
  top-level rather than the bean's original `[app]`-nested proposal.
- Tests: `appGoBuildArgs` unit tests (omit-when-zero, one per flag, and a
  fixed-order all-flags-set case), `TestCrossCompileAppliesLDFlags` against a
  new `internal/build/testdata/versioned` fixture, `parseExtraTags` unit
  tests (empty, split, dedupe, reject-gosd, reject-gosd_-even-for-an-
  unregistered-board), a flag-registration test for all 5 new flags, and
  three `cmd/gosd` integration tests: `TestBuildAppliesLDFlags` (new
  `testdata/versionedfixture`), `TestBuildTagsMergesWithMandatoryBoardTags`
  (a new `extratagmarker`-gated file added to the existing
  `testdata/boardtagfixture`), and `TestBuildRejectsAGosdNamespacedTagsValue`
  (mirrors `TestBuildRefusesADataSizeFAT32CannotHold`'s shape).
- Docs: `docs/build-config.md`'s example gains the 5 new top-level keys;
  `docs/board-build-tags.md` gets a new "Adding your own tags with --tags"
  section documenting the merge-not-replace behavior and the reserved-
  namespace rejection.

All quality gates green: `go build ./...`, `go test ./...` (whole repo),
`go vet ./...`, `gofmt -l .` (clean), `golangci-lint run ./...` and
`GOOS=linux golangci-lint run ./...` (0 issues each). `js/` untouched, so its
gate doesn't apply. `COMPATIBILITY.md` untouched — this is a generic CLI
flag, not a board/feature-support change.

## PR

https://github.com/jphastings/gosd/pull/369 (branch
`bean/gosd-wjjn-go-build-flag-compatibility`) — all CI checks green,
including one gate not anticipated by the bean or the commit history
available when work started: this repo's `.changeset/*.md`
change-file-check (knope-based release flow, `docs/releasing.md`), which
required adding `.changeset/go-build-flag-compatibility.md` (gosd: minor).
Awaiting JP's review; not self-merging per project convention.

## Increment C — `--ldflags` can reference `--app-version`'s resolved value

Added during PR #369 review (not originally in this bean): JP pointed out
that `--ldflags` has no equivalent to `--app-version`'s `git:v*.*.*`
resolution, so stamping the same version into both `config.json` and the
compiled binary meant a caller's build script had to resolve it once and
pass it to both flags separately. First response was to document the
limitation; JP asked to close it properly instead.

Design: `--ldflags` supports exactly one literal template token,
`{{.AppVersion}}` (internal whitespace tolerated, e.g. `{{ .AppVersion }}`,
matching how a real Go template would treat it — added per JP's comment on
the first draft of this addition's plan), substituted with `--app-version`'s
fully resolved value right after `resolveAppVersion` runs, before
`compileForBoards`. No text/template dependency: a scan for every
`{{...}}`-shaped substring in `ldflags`, refusing outright if any one isn't
`{{.AppVersion}}` (catches a typo or unsupported field before it reaches
`go build` as a literal, silently-wrong value), then refusing if the token
is present but `--app-version` resolved to empty. Mirrors `--tags`'
"validate and reject, don't silently continue" precedent
(`parseExtraTags`). New `resolveLDFlagsTemplate` in `cmd/gosd/appversion.go`
(same file `resolveAppVersion` lives in). No change to `AppCompileOptions`/
`appGoBuildArgs`/`compileForBoards` — `ldflags` is already a plain literal
string by the time it reaches them. `gosd run` untouched (no `--ldflags` or
`--app-version` there at all).

Tests: table-driven `TestResolveLDFlagsTemplate` in `appversion_test.go`
(no-token passthrough, substitution, whitespace tolerance, multiple
occurrences, empty-app-version refusal, unsupported-token refusal including
alongside the supported token). Integration:
`TestBuildLDFlagsSubstitutesAppVersionToken`,
`TestBuildRejectsLDFlagsAppVersionTokenWithoutAppVersion`,
`TestBuildRejectsAnUnsupportedLDFlagsTemplateToken` (all in
`archbuild_goflags_integration_test.go`), and the actual motivating
end-to-end scenario, `TestBuildLDFlagsSubstitutesGitResolvedAppVersion` in
`appversion_integration_test.go` — a `git:v*.*.*` resolution feeding both
`config.json` and the compiled `/app` binary from one resolved value.
`writeTestAppRepo` was generalized into a thin wrapper around new
`writeTestAppRepoWithMain` so this test could supply a `main.go` with
something to `-X`-target (the original fixture's `main.go` has nothing to
stamp).

Docs: `--ldflags`/`--app-version` help text cross-reference each other;
`docs/build-config.md`'s "no git: resolution of its own" paragraph (added
earlier in this same PR, in response to the same review) replaced with the
new token's behavior and an example; the top-level example block's
`ldflags` line updated to show the token. `.changeset/go-build-flag-
compatibility.md` gained a bullet describing it (not yet released, safe to
edit in place).

All quality gates green (`go build`, `go test ./...` whole repo, `go vet`,
`gofmt`, `golangci-lint` native + `GOOS=linux`).
