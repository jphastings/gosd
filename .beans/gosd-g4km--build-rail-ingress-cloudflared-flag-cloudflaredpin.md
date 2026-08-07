---
# gosd-g4km
title: 'Build rail: --ingress cloudflared flag, cloudflaredpin, initcfg field, integration test'
status: todo
type: task
priority: normal
created_at: 2026-08-07T12:51:49Z
updated_at: 2026-08-07T12:53:22Z
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

[ ] cloudflaredpin (pin the current cloudflared release + real sha256)
[ ] flag + parse/validate/collision, actionable errors, unit tests
[ ] download rail + per-board open + staticelf gate
[ ] initcfg field + pipeline wiring
[ ] integration test: fake static arm64 ELF fixture via --artifacts-dir;
    tripwire proves no fetch; asserts /bin/cloudflared 0755 + config.json flag
    + gosd.toml example present; without flag → neither
[ ] gosd run --ingress
