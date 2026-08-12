---
# gosd-rmqo
title: 'examples/sattrack: keep or drop the now-redundant x509roots blank-import'
status: completed
type: task
priority: low
created_at: 2026-08-07T13:58:41Z
updated_at: 2026-08-07T15:19:11Z
---

Since gosd-kzgq every image ships CA roots, sattrack's
`import _ "golang.org/x/crypto/x509roots/fallback"` is no longer required
for HTTPS to work. Two defensible positions — pick one and align the docs:

- **Keep it**, repositioned: sattrack becomes the documented example of an
  app pinning its own roots at build time (independent of gosd's release
  cadence) — docs/runtime.md already describes that alternative and
  currently points at sattrack.
- **Drop it**: examples stay minimal; runtime.md's alternative paragraph
  loses its example pointer or gains an inline one-liner instead.

Either way, sattrack's in-code comment about WHY the import exists must
match the chosen story — right now it still says images ship no roots,
which is stale.



## Decision

KEEP the blank import, repositioned: sattrack stays the documented example
of an app pinning its own roots at build time, independent of gosd's
release cadence. docs/runtime.md's HTTPS section (added by gosd-kzgq)
already describes this alternative and points at sattrack's main.go as
'the pattern in production use' — that pointer still reads correctly
against the reworded comment below, so no change was needed there.

## Summary of Changes

- examples/sattrack/main.go: reworded the in-code comment on the
  `x509roots/fallback` blank import. It no longer claims images ship no
  CA roots (stale since gosd-kzgq); it now says every image already ships
  a bundle, and the import is kept as the pattern for pinning your own
  roots at build time instead.
- docs/runtime.md: reviewed the HTTPS section's pointer to sattrack;
  confirmed it already reads correctly against the new comment, so left
  unchanged.

## Todos

- [x] Decide keep vs. drop
- [x] Update sattrack's in-code comment to match
- [x] Check docs/runtime.md's HTTPS section still reads correctly
- [x] Run quality gates
