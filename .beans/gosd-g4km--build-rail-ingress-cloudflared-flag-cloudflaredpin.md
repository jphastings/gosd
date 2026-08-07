---
# gosd-g4km
title: 'Build rail: --ingress cloudflared flag, cloudflaredpin, initcfg field, integration test'
status: completed
type: task
priority: normal
created_at: 2026-08-07T12:51:49Z
updated_at: 2026-08-07T18:04:23Z
parent: gosd-virc
blocked_by:
    - gosd-7upw
    - gosd-kzgq
---

Ingress epic gosd-virc bean 2 (after gosd-7upw; needs gosd-kzgq's
pipeline.ExtraFiles + CA rail merged first).

## Locked decisions

- New `internal/cloudflaredpin`: pinned release `Version`, `ByGOARCH
  map[string]Artifact{URL,SHA256,Name}` — arm64 only; THE MAP IS the capability
  table (pi-zero-w's arm-6 has no entry). SHA256s from the GitHub release body.
  Bump procedure in a doc comment.
- `cmd/gosd/ingress.go` + flag in build.go: repeatable `--ingress` (only value:
  `cloudflared`), parse fail-fast pre-compile; `validateIngress` beside
  validateUsbGadget — unsupported-board error names which selected boards DO
  support it and suggests `--board=` (pi-zero-w wording: official arm build is
  GOARM=7 and faults on armv6). Collision check vs `--with-external` dests
  (/bin/cloudflared, /etc/ssl/certs/ca-certificates.crt).
- Download via fetch.ToDir into `os.UserCacheDir()/gosd/ingress/`, cached
  `<sha256>-<Name>`; `--artifacts-dir` well-known-name override checked FIRST
  (integration-test seam; kernelfirmware.go is the template). Per-board fresh
  *os.File (pipeline closes readers) + staticelf.Verify against b.Arch() —
  applies to artifacts-dir overrides too.
- Embed at `/bin/cloudflared` via ExtraExecutables; `initcfg.Config` gains
  `IngressCloudflared bool` (json `ingressCloudflared,omitempty`) — the
  build→runtime contract.
- `gosd run` (cmd/gosd/run.go) also gains `--ingress` — qemu-virt is arm64 and
  exercises the runtime path in CI.

## Todos

[x] cloudflaredpin (pin the current cloudflared release + real sha256)
[x] flag + parse/validate/collision, actionable errors, unit tests
[x] download rail + per-board open + staticelf gate
[x] initcfg field + pipeline wiring
[x] integration test: fake static arm64 ELF fixture via --artifacts-dir;
    tripwire proves no fetch; asserts /bin/cloudflared 0755 + config.json flag
    + gosd.toml example present; without flag → neither
[x] gosd run --ingress


## Summary of Changes

- `internal/cloudflaredpin`: pins cloudflared `2026.7.3`, `ByGOARCH` with a
  single `"arm64"` entry (the capability table). **Pin verification
  record:** SHA256 for `cloudflared-linux-arm64` was read from the GitHub
  release body (`gh release view 2026.7.3 -R cloudflare/cloudflared`), then
  independently re-derived by downloading the same asset
  (`gh release download 2026.7.3 -R cloudflare/cloudflared -p
  cloudflared-linux-arm64`) and running `shasum -a 256` on it — both landed
  on `65259e652a7bea08bf5df603233ab22b8bf3116af8df9f9206209af6a1b955c0`.
  Asset URL:
  `https://github.com/cloudflare/cloudflared/releases/download/2026.7.3/cloudflared-linux-arm64`.
  `file` confirms it's a statically linked ELF 64-bit ARM aarch64
  executable (~35MiB), matching the epic's staticelf claim.
- `cmd/gosd/ingress.go`: `parseIngressFlags` (fail-fast, only value
  `cloudflared`), `validateIngress` (mirrors `validateUsbGadget`; pi-zero-w
  gets the GOARM=7/armv6 wording), `ingressGOARCHes` (per-arch dedupe,
  mirrors `compileForBoards`), `resolveIngressCloudflared` (artifacts-dir
  override first, then `fetch.ToDir` into
  `os.UserCacheDir()/gosd/ingress/`), `openIngressCloudflaredForBoard`
  (fresh `*os.File` + `staticelf.Verify`, applies to overrides too).
- `cmd/gosd/sharedcontent.go`: extended (not duplicated) to also resolve/
  open the cloudflared binary, so both `gosd build` and `gosd run` go
  through the one path — `openSharedContent` now takes the board and
  returns both `ExtraFiles` and `ExtraExecutables`.
- `cmd/gosd/external.go`: `/bin/cloudflared` and the CA bundle's path
  (`cacerts.InitramfsPath`) are now unconditionally in
  `reservedExternalDests`, so `--with-external` can never collide with
  either, whether or not `--ingress`/the CA bundle happen to be in play for
  that particular build.
- `internal/initcfg`: `Config.IngressCloudflared bool`
  (`ingressCloudflared,omitempty`) — the entire build→runtime contract;
  excluded from `ComputeIdentity`'s payload same as the rest of
  config.json.
- `internal/pipeline`: `Options.IngressCloudflared` threaded straight into
  `initcfg.Config`.
- `cmd/gosd/build.go` and `cmd/gosd/run.go`: both gain repeatable
  `--ingress` wired through the shared path above; `gosd run` validates
  against qemu-virt (arm64) the same way.
- Tests: `internal/cloudflaredpin` (capability-table shape),
  `internal/initcfg` and `internal/pipeline` (round-trip/bake-in),
  `cmd/gosd/external_test.go` (new reserved dests),
  `cmd/gosd/ingress_integration_test.go` (embed+flag, opt-out, bad value,
  pi-zero-w rejection + capable-board suggestion, reserved-dest collision),
  `cmd/gosd/run_integration_test.go` (`gosd run --ingress`), and
  `cmd/gosd/buildrun_parity_integration_test.go` extended to build/run with
  `--ingress cloudflared` and assert both sides actually carry
  `bin/cloudflared` (not just agree by both omitting it). New fixture:
  `cmd/gosd/testdata/fake-artifacts/cloudflared-linux-arm64` — a 64-byte
  hand-built minimal ELF (ELFCLASS64/EM_AARCH64, no program headers, so
  trivially static), generated the same way `withexternal_integration_test.go`'s
  `writeDynamicELF` builds its fixtures, just without the PT_INTERP header.
- Not in this PR, deliberately: COMPATIBILITY.md / other docs. The epic's
  own child-bean order puts docs after the runtime wiring bean (gosd-66ax)
  and provsnapshot — the `ingressCloudflared` bit this PR bakes is inert
  until gosd-init actually reads it, so there's no user-facing behavior to
  document yet.

## Gates

`go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run
--allow-parallel-runners ./...` and the `GOOS=linux` variant all clean.
One unrelated failure in `internal/diskfmt` (`TestFormatEXT4GeneratesAFreshUUIDEachTime`,
"no space left on device") is this shared machine's disk pressure (900MiB-1.1GiB
free during this session) — that package is untouched by this bean; CI is
the authoritative full gate.
