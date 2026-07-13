---
# gosd-6zd1
title: 'Document HTTPS from gosd apps: images ship no CA roots (x509roots/fallback pattern)'
status: todo
type: task
created_at: 2026-07-13T06:39:42Z
updated_at: 2026-07-13T06:39:42Z
---

Found during [[gosd-e9fy]] (sattrack): GoSD images contain no /etc/ssl CA bundle, so crypto/x509 finds zero roots and EVERY outbound HTTPS request from an app fails with certificate errors on-device — while working fine in `go run` on the developer's machine. This will bite every app that calls an HTTPS API.

## Current workaround (proven in examples/sattrack)

Blank-import the Mozilla root bundle into the app binary:

    import _ "golang.org/x/crypto/x509roots/fallback"

Pure Go, ~300KB of binary size, no image change, roots update with the dependency.

## Options for the real fix

- **A — document the blank import as THE pattern** in docs/runtime.md (networking section) and the quickstart. Zero image cost, per-app opt-in, roots pinned at app build time. Cheapest, honest. **Recommended now.**
- **B — ship a CA bundle in the image** (/etc/ssl/certs in the initramfs): every app gets HTTPS for free, +~200KB per image, but roots age with the gosd release and add an update-story obligation.
- **C — A now, revisit B when OTA updates ([[gosd-vxal]]) give bundles an update path.**

## Todos

- [ ] docs/runtime.md: networking section gains an HTTPS subsection (symptom, why, the fallback import, when roots update)
- [ ] Quickstart/README mention where apps first touch the network
- [ ] Record decision A/B/C here (JP)
