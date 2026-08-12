---
# gosd-1jjh
title: 'The artifacts cache has no trust anchor: a poisoned manifest.json backdoors every image built after it'
status: todo
type: bug
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-12T04:18:42Z
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

- [ ] Bake and verify a SHA-256 for the release manifest.json alongside `artifacts.Version`
- [ ] Update docs/artifacts.md's release procedure with the new value
- [ ] `io.LimitReader` around the zstd stream in `fetchTarball`
- [ ] A timeout on the shared http.Client used by fetch and artifacts
