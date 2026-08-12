---
# gosd-zlee
title: 'Docs: the upgrade story (expand as updatable-deployment default, re-adoption, snapshot)'
status: completed
type: task
created_at: 2026-07-31T09:17:50Z
updated_at: 2026-07-31T09:17:50Z
---

Phase 1 of the upgrade-path design (bean gosd-inau, docs/design/upgrade-path.md §6). runtime.md: /data survives reflash for expand images once re-adoption lands; snapshot/self-heal behavior. publishing.md: recommend --data-size=expand for updatable deployments. flashing.md: a short, jargon-free 'Upgrading your device' section (flash again, same steps). Update COMPATIBILITY.md rows/footnotes as features land (in those beans' PRs, per project rule).

## Summary of Changes

Verified against the merged code (`cmd/gosd-init/internal/dataexpand`,
`cmd/gosd-init/internal/provsnapshot`, `internal/initcfg/identity.go`,
`cmd/gosd/build.go`'s `--boot-size` flag) rather than the design doc alone,
and updated every doc that needed it:

- **`docs/runtime.md`**: the "At a glance" root-filesystem bullet now notes
  that an `expand` build's `/data` (and hand-edited hostname/WiFi/`[env]`)
  survives a reflash. In "Persistent storage: `/data`", replaced the
  now-false "reflashing resets the cycle" claim with the real mechanics —
  re-adoption gated on the `gosd-data-established` completion marker, why a
  `--boot-size` change between releases breaks adoption (release-notes-level
  breaking change), and that a fixed `--data-size` is still wiped every
  time. The reserved-marker bullet now also names `gosd-data-established`
  and `/data/.gosd/` alongside the existing `.gosd-data`. Added a new
  subsection, "The provisioning snapshot: surviving a reflash", covering the
  `/data/.gosd/provision-snapshot/` mechanism, the wizard > snapshot > baked
  precedence with its hand-edit-detection logic, and what does *not*
  restore (other wizard settings, `/data` schema/contents, hand-written
  `gosd.toml` comments). All eight pinned heading anchors are unchanged.
- **`docs/publishing.md`**: two new sections after "Baking default app
  environment variables" — recommending `--data-size=expand` for updatable
  deployments (why: a fixed size ships its `GOSD-DATA` inside the `.img`,
  so every reflash wipes it), and documenting `--boot-size` (default,
  actionable-error-on-overflow, the usage report, and the layout-ABI/
  breaking-change caveat for changing it later).
- **`docs/flashing.md`**: a short, jargon-free "Upgrading your device"
  section between "Find your device" and "Troubleshooting" — flash again
  from the start with the same card and the new link; saved settings come
  back on their own. No internals, no partition talk; every existing image
  reference and step number is untouched.
- **`COMPATIBILITY.md`**: extended the `[^data-opt-in]` footnote with the
  reflash-survival fact for `expand` (citing bean `gosd-lirl`, PR #158) and
  the `--boot-size`-must-match caveat; no matrix cell or existing footnote
  key touched.
- **`README.md`**: one-line touch to the "Going further" runtime-contract
  bullet, adding "and what survives an upgrade".

No code-vs-design divergences found — `gosd-lirl`/`gosd-m70t`/`gosd-acdn`/
`gosd-ry3b` implemented docs/design/upgrade-path.md §0.4, §2, §3 and §4 as
locked. Quality gates (`go test ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`) all
pass; this is a docs-only change.
