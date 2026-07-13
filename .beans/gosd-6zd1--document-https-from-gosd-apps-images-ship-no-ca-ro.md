---
# gosd-6zd1
title: 'Document HTTPS from gosd apps: images ship no CA roots (x509roots/fallback pattern)'
status: in-progress
type: task
priority: normal
created_at: 2026-07-13T06:39:42Z
updated_at: 2026-07-13T08:39:49Z
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

- [x] docs/runtime.md: networking section gains an HTTPS subsection (symptom, why, the fallback import, when roots update)
- [x] Quickstart/README mention where apps first touch the network
- [ ] Record decision A/B/C here (JP)


## Summary of Changes

- `docs/runtime.md`: added a `### HTTPS calls need a CA bundle your app supplies` subsection at the end of "Networking comes up after your app does" (before "Provisioning"). Covers the symptom (an `x509: certificate signed by unknown authority`-style failure on-device that doesn't repro under `go run`), why (GoSD images ship no `/etc/ssl` CA bundle, so `crypto/x509` has no system roots), the fix (blank-import `golang.org/x/crypto/x509roots/fallback`, pointing at `examples/sattrack/main.go` as the production pattern), and when roots update (pinned at app build time via the module version in `go.mod`; bump the dependency to refresh). Also cross-references the existing "Clock" gotcha, since a wrong clock breaks cert validity checks independently of CA roots.
- `README.md`: added a short blockquote callout right after the Quickstart step 2 `main.go` example (the first place a reader adds their own app code) noting that outbound HTTPS calls need the same blank-import, linking to `docs/runtime.md#https-calls-need-a-ca-bundle-your-app-supplies`. Chose this spot over the top-level Features bullet list since step 2 is literally where apps first touch the network in the doc's flow.
- Left the third todo ("Record decision A/B/C here (JP)") unchecked — that decision is explicitly tagged for JP in the bean and was not made by this change; docs were written assuming option A per the bean's own recommendation and the pattern already in production use (examples/sattrack), but A/B/C was not recorded as a locked decision here.
- Ran the repo's full quality gate set (`go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...`) since this is docs-only but the repo requires them regardless.
