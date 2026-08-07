---
# gosd-kzgq
title: 'Bake the Mozilla CA bundle into every image (resolves gosd-6zd1: option B)'
status: todo
type: feature
created_at: 2026-08-07T12:50:52Z
updated_at: 2026-08-07T12:50:52Z
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

[ ] `internal/cacerts` pin package (+ bump-procedure doc comment)
[ ] `pipeline.Options.ExtraFiles` (0644, identity-covered) + unit test
[ ] Build wiring: resolve for every board/build; `--artifacts-dir` override first
[ ] Seed the cacert fixture in every existing build-integration test (network
    tripwire must stay green — no test may hit curl.se)
[ ] Integration assertion: every built image contains the bundle at 0644
[ ] docs/runtime.md HTTPS section rewrite: roots ship in-image now; the
    x509roots blank-import becomes unnecessary (keep as historical/alternative
    note); examples/sattrack can keep its import (harmless)
[ ] Record decision B in gosd-6zd1 and check its open todo
