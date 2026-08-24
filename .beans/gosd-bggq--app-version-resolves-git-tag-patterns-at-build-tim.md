---
# gosd-bggq
title: 'app-version resolves git: tag patterns at build time'
status: completed
type: feature
priority: normal
created_at: 2026-08-24T03:40:19Z
updated_at: 2026-08-24T04:06:08Z
---

`--app-version` (and `[app] version` in gosd-build.toml, where this matters most — the file is checked in once and can't be edited per release) can name a git source instead of a literal: `git:v*.*.*` resolves at build time to a describe-style version derived from the app repo's tags.

## Locked decisions (JP, 2026-08-24)

- **Pure Go, no shelling out**: resolution uses the go-git library ([[gosd-mwct]] follow-on; JP chose the pure-Go package over exec'ing a git binary). Tests create fixture repos with go-git too, so they stay hermetic and pass on machines with no git installed.
- **Syntax**: a `git:` prefix on the version value is a resolution scheme; everything after it is a glob matched against tag names (path.Match, same wildcard family as git's fnmatch). Bare `git:` means any tag (`*`). Values without the prefix stay verbatim — gosd still never interprets the *resulting* version string.
- **"Most recent" = git describe semantics, not highest-tag**: the matching tag nearest HEAD's ancestry wins (distance = commits reachable from HEAD but not from the tag), so building a maintenance branch never picks up a newer tag from another branch. Ties: smaller distance, then annotated over lightweight, then newest (tagger date, else commit date), then name — pinned by tests.
- **Full describe output, suffix-not-refuse**: exactly-tagged HEAD yields the tag; otherwise `<tag>-<N>-g<7-hex>`; a dirty worktree appends `-dirty` and NEVER fails the build. Untracked files do not count as dirty (matching `git describe --dirty`'s diff-index semantics).
- **Extraction is mechanical**: the glob's literal prefix (characters before its first wildcard) is stripped from the tag — `git:v*.*.*` turns `v1.4.2-5-gabc1234` into `1.4.2-5-gabc1234`, `git:release-*` turns `release-7` into `7`. No hardcoded "strip v" vocabulary.
- **Resolved against the app's repo**: the search starts at the app main package's directory (walking up to the enclosing repo), not the cwd — the monorepo `--build-config` shape versions the right repo.
- **Shallow checkouts get specific guidance**: the no-matching-tag error detects a shallow clone and names the fix directly (`git fetch --unshallow --tags`; for actions/checkout, `fetch-depth: 0` which fetches all history and tags). The same guidance lives in the build-config docs page. A plain no-match (not shallow) names the glob and how many tags exist.
- gosd run has no --app-version and gains nothing here.

## Todo

- [x] Add go-git dependency; `internal/gitversion`: scheme parse, describe (reachability walk + distance), tie-breaks, dirty check, prefix strip, shallow-aware errors + hermetic go-git-built fixture tests
- [x] Wire cmd/gosd: resolve `git:` after file/flag merge using the app dir; amend --app-version help ("never interpreted" → "resolves a git: source, never interprets the result")
- [x] Integration test: bare build with [app] version = "git:v*" bakes the described version into config.json
- [x] docs/build-config.md section (incl. CI shallow-checkout guidance) + changeset (gosd: minor)
- [x] flake.nix vendorHash bump for the new dependency (no local nix — take the hash from the CI job's mismatch report)
- [x] Quality gates (go test/vet/gofmt/golangci-lint darwin+linux)

## Summary of Changes

- `internal/gitversion`: pure-Go describe on go-git v5.17.1 (already in the module graph, so the direct import added no version churn). Nearest-reachable-tag selection with exact distance (|ancestry(HEAD)| − |ancestry(tag)|), tie-breaks pinned by tests, `-N-g<7hex>` and `-dirty` suffixes matching `git describe --tags --dirty` (untracked ≠ dirty), literal-prefix extraction, shallow-aware actionable errors. Fixture repos are built with go-git, so the tests run with no git binary installed.
- `cmd/gosd`: `resolveAppVersion` runs after the file/flag merge against the app main package's enclosing repo; import-path operands are refused actionably; the resolved version is printed to stderr; `--app-version` help amended (gosd resolves a `git:` source, never interprets the result). `filesystemPathLike` extracted from `packagePathLike`.
- End-to-end integration test bakes `"appVersion":"2.5.0"` from a toml `git:v*.*.*` source into config.json, network-tripwired.
- docs/build-config.md section (describe semantics, extraction rule, CI shallow-checkout guidance) + `gosd: minor` changeset; flake.nix vendorHash bumped via CI's hash oracle.
