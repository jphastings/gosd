---
# gosd-1jjh
title: 'The artifacts cache has no trust anchor: a poisoned manifest.json backdoors every image built after it'
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-20T05:51:32Z
---

**Severity: Medium.** Requires local write access to the user's cache
directory, but it is silent, persistent, and inconsistent with how every
other pin in this codebase works.

## Verified — the asymmetry is the finding

Every other pin here is compiled in and re-checked on use:

- `internal/cacerts.Pin` and every `build/boards/*/manifest.json` blob pin
  are `go:embed`ed into the binary.
- `internal/fetch.ToDir` (`fetch.go:42-101`) fails closed on an empty
  SHA256, hashes **before** renaming into place, and **re-hashes a cache
  hit** rather than trusting its presence. Cache tampering there just causes
  a re-download.

`internal/artifacts` has no equivalent. `ensureBoard`
(`artifacts.go:104-170`) begins:

```go
if m, err := readManifestCache(manifestPath); err == nil {
    if bf, ok := m.Boards[board]; ok && verifyFiles(boardDir, bf.Files) == nil {
        return boardDir, nil // fully offline: cache already verified, no network touched
    }
}
```

`readManifestCache` (`:180-191`) is a bare `os.ReadFile` + `json.Unmarshal` —
no hash, no signature. `verifyFiles` then checks the board's files against
hashes taken from **that same untrusted cached file**. The only compiled-in
value is the `Version` string.

## Attack

Any process running as the developer — a compromised editor extension, an
npm/pip postinstall, another CLI tool — writes a self-consistent pair under
`$UserCacheDir/gosd/artifacts/<version>/<board>/`: a backdoored kernel or
U-Boot, plus a `manifest.json` whose hashes match the backdoored bytes.

Every subsequent `gosd build` for that board takes the cache-hit branch,
touches the network never, and bakes the backdoored kernel into every image
the developer flashes. `pruneArtifactCache` only removes **superseded**
versions, so the poisoned current version is never revisited. The compromise
outlives removal of the malware that planted it.

## Fix

Anchor the manifest the way `cacerts.Pin` anchors the CA bundle: bake the
canonical `manifest.json`'s SHA-256 into source next to
`internal/artifacts.Version`, and verify the manifest bytes against it before
trusting any entry inside. The release procedure in `docs/artifacts.md`
already bumps `Version` by hand for each `artifacts/vX.Y.Z` tag, so this adds
one value to an existing, already-manual step.

## Related, same file

**Decompression bomb.** `fetchTarball`/`extractFile` (`:247-297`) streams
`zstd.NewReader(resp.Body)` through `io.Copy` into files with no output-size
cap, and this happens **before** `verifyFiles` runs. A malicious release (or
the finding above) fills the disk before any hash mismatch is detected. Path
traversal itself is handled correctly (`:273-276` cleans and rejects
dot-dot/absolute entries) — this is DoS only. Wrap the reader in an
`io.LimitReader` sized above the largest expected board tarball.

**No HTTP timeout.** `fetch.go:46-48` and `artifacts.go:300-314` use
`http.DefaultClient` (`Timeout: 0`), and `cmd/gosd/main.go:26-31` attaches no
context deadline. A stalling upstream hangs `gosd build` indefinitely.

## Todos

- [x] Bake and verify a SHA-256 for the release manifest.json alongside `artifacts.Version`
- [x] Update docs/artifacts.md's release procedure with the new value
- [x] `io.LimitReader` around the zstd stream in `fetchTarball`
- [x] A timeout on the shared http.Client used by fetch and artifacts


## Summary of Changes

The anchor is `internal/artifacts.ManifestSHA256`, a compiled-in SHA-256 of
the pinned release's `manifest.json`, sitting next to `Version` exactly the
way `internal/cacerts.Pin` sits next to nothing else. `internal/artifacts`
now behaves the way `internal/fetch` always has:

- **Every read of the manifest is checked against it** — `parseManifest`
  hashes the bytes before `json.Unmarshal` ever sees them, and both
  `fetchManifest` (network) and `readManifestCache` (cache) go through it.
  A tampered cache no longer verifies, so `ensureBoard` falls through to the
  download path and replaces the poisoned files; offline it fails loudly
  instead of silently building the backdoor in.
- **The cached manifest is now the published bytes.** It was being
  re-`json.Marshal`ed, which could never have been re-checked against a
  digest of the release asset. It is written through a temp file + rename,
  like every other cache write here.
- **Empty pin fails closed**, mirroring `fetch.ToDir`'s empty-SHA256
  refusal: the error names `ManifestSHA256`, says it is a bug in the build
  rather than anything the user did, and points at `--artifacts-dir` as the
  way to keep working. No request is made at all in that state.
- `--artifacts-dir` is deliberately untouched: it is resolved in
  `boards.ResolveArtifacts` before `EnsureBoard` is reached, and a developer
  pointing gosd at their own `gosd build-kernel` output is not the threat.

Regression proof: `TestEnsureBoardRefetchesWhenTheCachedManifestWasTamperedWith`
and `TestEnsureBoardOfflineRefusesATamperedCacheRatherThanTrustingIt` plant
exactly the attack in the finding — a backdoored `kernel8.img` plus a
self-consistent `manifest.json` listing its digest — and both were confirmed
to FAIL against the pre-fix behaviour (simulated by dropping the digest check
in `readManifestCache`) and pass with it.

The two related findings in the same file are fixed too:

- `fetchTarball` reads the zstd stream through a `cappedReader` (1 GiB, ~40x
  the largest board's artifacts). It reports its own error rather than a
  clean EOF, so an expanding archive fails as what it is instead of as a
  truncated tar. The cap is a parameter, so the test proves it with 1 KiB
  instead of writing a gigabyte. The manifest read is bounded too (4 MiB) —
  the digest check can only run on bytes already in memory.
- `internal/fetch.DefaultClient` replaces `http.DefaultClient` in both
  packages: dial/TLS/response-header stall timeouts, and deliberately NO
  overall `Client.Timeout`, which would apply to the response body and fail
  a legitimately slow board-tarball download as readily as a stalled one.
  `http.ProxyFromEnvironment` is kept — CI's offline check drives gosd
  through a dead proxy.

Keeping the pin honest is automation's job, not a human's:
`build/artifacts/pin-bump.sh` now downloads the release's `manifest.json`,
hashes it, and rewrites both constants in one edit (exercised locally
against the real v0.10.1 and v0.10.2 releases, round-tripping to the digest
committed here). Its download doubles as a second check that the release is
really published. The `Verify artifacts pin` workflow needs no change: its
clean-machine build of every board from a real download now fails if the
digest is wrong, which is the check being asked for.

### Found while here, not fixed

`.github/workflows/pin-artifacts-version.yml` stages only
`internal/artifacts/artifacts.go` before committing, so the change file
`pin-bump.sh` writes — the whole point of which is that the bump reaches
people who install gosd rather than build it — is never committed. PR #306
(the v0.10.2 bump) landed with no change file, confirming it. Filed as a
follow-up rather than fixed here, since it is gosd-odx3's mechanism.


### The committed digest is CI-verified

`Verify artifacts pin` skips itself when the version doesn't move, so it
proves nothing here — but the `qemu boot-to-HTTP smoke test` does. That job
builds with no `--artifacts-dir`, so it downloads qemu-virt's kernel from the
pinned release for real, and its artifact cache is keyed on
`hashFiles('internal/artifacts/artifacts.go')` — which this change modifies,
so the run started cold, fetched `manifest.json` from artifacts/v0.10.2, and
had to satisfy the new anchor to get as far as booting. A wrong digest would
have failed it at the build step. PR #334: green.
