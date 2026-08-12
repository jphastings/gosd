---
# gosd-gdro
title: 'gosd leaves unmanaged, unbounded caches on users'' machines — self-bound them to the current version'
status: completed
type: feature
priority: normal
created_at: 2026-08-08T00:00:00Z
updated_at: 2026-08-08T00:00:00Z
---

JP (2026-08-08): caching the CURRENT version's assets is desirable (~hundreds
of MiB is fine), but NOTHING may grow in proportion to how many times / how
many versions of gosd are run. Measured on JP's Mac: os.UserCacheDir()/gosd =
101MiB (artifacts/v0.10.0 = 65MiB + ingress cloudflared ~25MiB + cacerts),
and artifacts/ is keyed by artifacts.Version and NEVER pruned — every gosd
release adds another ~65MiB tree forever.

## This PR (core, everyday-user fix)
After a build SUCCESSFULLY resolves the current pins, prune superseded entries
so the cache holds exactly the current version's assets:
- artifacts/: keep only the current artifacts.Version dir, remove sibling
  vX.Y.Z dirs.
- cacerts/, ingress/: keep only the file(s) matching the current pin
  (sha256-name), remove others.
Cheap to refetch; does NOT break offline re-run (same pinned version = same
dir = still a hit). Only prudes gosd's OWN cache dirs, only after success.

## Follow-ups (NOT this PR — leave as todos)
- Durable build-state dir (kernel-build/external-build, ~652MiB, opt-in
  driver-dev, EXPENSIVE to rebuild): bound by keep-last-N / size cap, prune
  stale content-addressed entries after a successful build, never the
  just-built key (preserve gosd-l4y9 mid-build-purge safety). Separate PR.
- `gosd cache` subcommand (dir/size/clean [--builds]) convenience. Separate PR.

## Todos
[x] artifacts/ prune-to-current-version after successful resolve
[x] cacerts/ + ingress/ prune-to-current-pin
[x] behavioral tests over a temp cache tree (no network)
[x] docs: note the self-bounding behavior + cache location in build --help/README

## Summary of Changes

Implemented the core, everyday-user fix only (both follow-ups below remain
open, NOT touched by this PR):

- `cmd/gosd/cacheprune.go` (new): `pruneCacheToCurrent(dir, keep,
  isManaged)` is the small, pure, unit-tested helper - it removes every
  top-level entry in `dir` that `isManaged` recognises but isn't listed in
  `keep`, leaving anything `isManaged` doesn't recognise strictly alone, and
  no-ops on a missing dir. Three thin wrappers sit on top:
  `pruneArtifactCache` (keeps only `internal/artifacts.Version`'s `vX.Y.Z`
  directory under `artifactCacheDir()`), `pruneCACertsCache` (keeps only the
  file matching the current `cacerts.Pin`), and `pruneIngressCache` (keeps
  the file(s) matching every GOARCH currently pinned in
  `cloudflaredpin.ByGOARCH`, not just the ones this invocation happened to
  resolve). `pruneDownloadCaches(cmd, artifactsDir)` wires all three
  together, logs any failure to `cmd`'s stderr, and always returns
  (best-effort - never fails a build that already succeeded).
- `cmd/gosd/build.go` / `cmd/gosd/run.go`: call `pruneDownloadCaches` once,
  after the per-board build loop / after `pipeline.Assemble` succeeds -
  never per board. Also added a `Long` description to `gosd build --help`
  documenting the cache location and the automatic pruning.
- `cmd/gosd/cacheprune_test.go` (new): behavioral tests over `t.TempDir()`
  trees (no network) asserting only-superseded-removed, current-kept,
  unknown-entries-left-alone, an in-progress `.part-*` download left alone
  (race safety), missing-dir-noop, and that `pruneDownloadCaches` is a
  strict no-op whenever `--artifacts-dir` is set.
- `docs/artifacts.md` / `README.md`: documented the self-bounding behavior
  and cache location.

### Safety rules enforced

- Pruning only ever touches gosd's own three cache dirs
  (`artifactCacheDir()`, `caCertsCacheDir()`, `ingressCacheDir()`) - nothing
  else on disk.
- Never removes the current version/pin: `artifacts/`'s current
  `internal/artifacts.Version` dir and `cacerts`/`ingress`'s current pinned
  filename(s) are always in `keep` and therefore untouched.
- Never removes an entry it doesn't recognise the shape of:
  `artifacts/` only ever considers entries matching `^v\d+\.\d+\.\d+$`;
  `cacerts/`/`ingress/` only ever consider entries matching the
  `<sha256>-<name>` fetch-cache naming convention, and explicitly exclude
  `fetch.ToDir`'s own in-progress `<name>.part-*` temp files so a prune can
  never race a concurrent invocation's in-flight download in the same cache
  dir. An unrelated stray file/dir is always left alone.
- Never prunes when `--artifacts-dir` was passed: `boards.ResolveArtifacts`
  and the cacerts/ingress equivalents check `--artifacts-dir` per file
  before ever falling back to the download cache, so an `--artifacts-dir`
  build may not have touched (or fully populated) the cache at all -
  `pruneDownloadCaches` skips entirely in that case.
- Runs only after a successful resolve, once per invocation (after the
  per-board loop in `gosd build`, after `pipeline.Assemble` in `gosd run`) -
  never per board, never on a failed build.
- Best-effort: a removal failure is collected (not raised) by
  `pruneCacheToCurrent`, and `pruneDownloadCaches` only logs it to stderr -
  it can never turn an otherwise-successful build into a failure.
- Does NOT break offline re-run of the same pinned version: pruning only
  ever removes *sibling* version/pin entries, never the one just resolved,
  so a second offline build at the same gosd version is still a cache hit.

## Follow-ups (confirmed NOT done here, left as-is above)

- Durable build-state dir (`gosd build-kernel`/`build-external`, opt-in,
  content-addressed, expensive to rebuild) pruning - separate PR.
- `gosd cache` subcommand (dir/size/clean [--builds]) - separate PR.
