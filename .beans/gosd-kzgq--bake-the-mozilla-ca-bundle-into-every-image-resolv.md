---
# gosd-kzgq
title: 'Bake the Mozilla CA bundle into every image (resolves gosd-6zd1: option B)'
status: completed
type: feature
created_at: 2026-08-07T12:50:52Z
updated_at: 2026-08-07T16:00:00Z
---

JP decision (2026-08-07, cloudflared-ingress planning): CA roots are baked into
EVERY image, unconditionally — every reasonable device use touches the internet.
This records gosd-6zd1's open A/B/C decision as **B** (note it there too).
Independent of the ingress epic, useful on its own, and a prerequisite for
`--ingress cloudflared` (cloudflared has no embedded trust store — it fails
with x509 errors on systems without /etc/ssl, proven by scratch-container
reports upstream).

## Locked decisions

- Bundle lands at `/etc/ssl/certs/ca-certificates.crt` (0644) in the initramfs —
  Go's default root path on Linux, so apps get HTTPS for free too (~230KB).
- Source: curl.se's Mozilla extract at a **dated** snapshot URL
  (https://curl.se/ca/cacert-YYYY-MM-DD.pem) + sha256, pinned in a new
  `internal/cacerts` package (Version/URL/SHA256) — the dated URL can never rot
  under the pin the way the rolling `cacert.pem` URL would. Downloaded at build
  time via `fetch.ToDir` (cache: `os.UserCacheDir()/gosd/ingress/` or its own
  dir), `--artifacts-dir` well-known-name (`ca-certificates.crt`) override
  checked first — that override is also the integration-test seam.
  `cmd/gosd/kernelfirmware.go` is the ~90-line template for this rail.
- Pipeline: new `pipeline.Options.ExtraFiles map[string]io.Reader`, written at
  0644 and identity-covered like ExtraFirmware (`ExtraExecutables` is 0755-only,
  wrong mode for a cert). This map is reused by the ingress epic later.
- Roots update via pin bump per gosd release; document the bump procedure next
  to the pin.

## Todos

[x] `internal/cacerts` pin package (+ bump-procedure doc comment)
[x] `pipeline.Options.ExtraFiles` (0644, identity-covered) + unit test
[x] Build wiring: resolve for every board/build; `--artifacts-dir` override first
[x] Seed the cacert fixture in every existing build-integration test (network
    tripwire must stay green — no test may hit curl.se)
[x] Integration assertion: every built image contains the bundle at 0644
[x] docs/runtime.md HTTPS section rewrite: roots ship in-image now; the
    x509roots blank-import becomes unnecessary (keep as historical/alternative
    note); examples/sattrack can keep its import (harmless)
[x] Record decision B in gosd-6zd1 and check its open todo

## Summary of Changes

- Added `internal/cacerts`: pins the dated Mozilla bundle
  (`https://curl.se/ca/cacert-2026-07-16.pem` + sha256, verified today),
  `ArtifactName` ("ca-certificates.crt", the well-known `--artifacts-dir`
  override/cache-file name), `InitramfsPath`
  (`/etc/ssl/certs/ca-certificates.crt`), and a package doc comment recording
  the bump procedure (pick the newest `cacert-YYYY-MM-DD.pem` from
  https://curl.se/ca/, verify against its published `.sha256`, update URL and
  SHA256 together).
- Added `pipeline.Options.ExtraFiles map[string]io.Reader`
  (`internal/pipeline/pipeline.go`): non-executable extra initramfs files,
  written at mode 0644 (a new `extraFileMode` constant) and covered by the
  image identity, mirroring `ExtraExecutables`' read-into-memory, close
  discipline, and payload/initramfs-file handling exactly, but at the
  firmware/config mode rather than the executable one. Added
  `TestAssembleWritesExtraFilesAtMode0644AndChangesIdentity` asserting an
  `ExtraFiles` entry lands in the initramfs at 0644 with its content intact
  and changes `Config.Identity` relative to a build without it.
  `ExtraFiles` is reused as-is by the ingress epic later per the locked
  decision.
- Added `cmd/gosd/cacerts.go`, mirroring `kernelfirmware.go`'s shape:
  `caCertsCacheDir` (`$UserCacheDir/gosd/cacerts`, separate from the board
  artifact cache), `resolveCACerts` (artifactsDir override first, else
  `fetch.ToDir` keyed by `<sha256>-ca-certificates.crt`), and
  `openCACertsForBoard` (a fresh `*os.File` per board, since
  `pipeline.Assemble` closes every reader it's handed). Wired into
  `cmd/gosd/build.go`'s `runBuild`: resolved once per invocation, right
  after the kernel-firmware resolution and before the per-board loop, then
  opened fresh and passed as
  `ExtraFiles: map[string]io.Reader{cacerts.InitramfsPath: caCerts}` for
  each board. `cmd/gosd/run.go` builds its own `pipeline.Options` (it
  doesn't share `runBuild`), so it needed the identical
  resolve-once-then-open-per-board wiring added there too, using its own
  `--artifacts-dir`/cache-dir plumbing — `gosd run` did NOT get this for
  free.
- Error messages are actionable per CLAUDE.md: a fetch failure names the
  network and suggests `--artifacts-dir`; a cache-dir failure suggests the
  same.
- Seeded `cmd/gosd/testdata/fake-artifacts/ca-certificates.crt` with a few
  lines of fake PEM text (never the real ~186KB bundle) — every existing
  build/run integration test already points `--artifacts-dir` at this one
  shared fixture directory, so adding the file there satisfied every test's
  network tripwire with no per-test plumbing changes needed. Added an
  `assertCACertsBaked` helper (`cmd/gosd/build_integration_test.go`) that
  checks the initramfs record's path, mode (`S_IFREG|0644`), and exact fixture
  content, and wired it into the fake-artifacts acceptance test for every
  board that has one (pi-zero-2w, pi-zero-w, pi-3b, radxa-zero-3e,
  nanopi-zero2, rock-4se, qemu-virt).
- Rewrote `docs/runtime.md`'s HTTPS subsection (now "HTTPS calls and the CA
  bundle") and its "At a glance" bullet: images ship the Mozilla bundle at
  the standard path, so `crypto/x509` finds system roots with no app-side
  step; the `x509roots/fallback` blank-import is now optional (kept as an
  alternative for an app that wants its own pinned-at-build-time roots
  independent of gosd's release cadence). Kept the Clock cross-reference.
  Also fixed `README.md`'s now-stale "GoSD images ship no CA bundle" callout
  and its anchor link. `examples/sattrack` was deliberately left untouched
  per the task scope (its own blank-import remains harmless).
- The "Record decision B in gosd-6zd1" todo was completed separately in this
  branch's planning-beans commit (gosd-6zd1's open JP todo is checked there).
