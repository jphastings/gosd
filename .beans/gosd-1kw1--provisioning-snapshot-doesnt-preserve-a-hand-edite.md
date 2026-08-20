---
# gosd-1kw1
title: Provisioning snapshot doesn't preserve a hand-edited data_flush across reflash
status: completed
type: task
priority: low
created_at: 2026-08-02T14:25:46Z
updated_at: 2026-08-20T05:39:34Z
---

Found while implementing gosd-9m1k (PR #177). The provisioning snapshot
(gosd-ry3b) classifies and restores hostname/WiFi/[env] hand-edits after
a reflash, but gosd.toml's new data_flush key is not snapshotted — a
hand-edited `data_flush = true` reverts to the baked default on upgrade,
inconsistent with the hand-edits-survive-upgrades story.

Decide: either add DataFlush to the snapshot's per-key classification
(restore iff it differs from the contemporaneous baked default and the
fresh card shows no fresh intent — the existing rule shape), or
explicitly document it as build-flag-owned (upgrade docs note that card-
level data_flush is per-flash). Small either way; the first keeps one
mental model.

## Investigation

This bean predates epic `gosd-rw6n` (2026-08-12/13), which replaced the
old gosd.toml-era "provisioning snapshot" (gosd-ry3b) entirely with the
per-attribute config tree plus `cmd/gosd-init/internal/configstore` (bean
`gosd-87ip`). `data_flush` is no longer a special gosd.toml field with its
own snapshot classification code path — it is an ordinary config-tree
setting (`internal/configtree/defaults/data_flush`, `config/data_flush` on
the card), read and restored by exactly the same fully generic,
path-agnostic machinery as `hostname` and `wifi/ssid`:
`configstore.Reconcile` iterates every path in the card's tree and every
digest in the image's baked `ConfigDigests` — neither of which
special-cases any one setting by name. `boot/sequence.go` already
documents the resulting ordering nuance explicitly (data_flush decides how
`/data` is mounted, so a restored value only takes effect from the boot
*after* the reflash, not during it).

Confirmed by adding `TestAHandEditedDataFlushSurvivesAReFlash` to
`cmd/gosd-init/internal/configstore/configstore_test.go` (edits
`data_flush` on a card, boots under image A, reflashes to defaults under
image B, and asserts the store restores it byte-for-byte) — it passes
against the current code with no production change required. The bug this
bean reports was already fixed as a side effect of the config-tree
rewrite; nobody had gone back to close this bean out.

## Summary of Changes

No production code changed. Added a dedicated regression test
(`TestAHandEditedDataFlushSurvivesAReFlash` in
`cmd/gosd-init/internal/configstore/configstore_test.go`) that pins
`data_flush` surviving a reflash through the config store, the same way
`hostname`'s round-trip is already pinned — so this can't silently regress
again without a test failing. Confirmed `data_flush` needs no special-case
handling: it is a fully ordinary config-tree setting from the store's
point of view, covered by `Reconcile`'s generic per-path logic exactly
like every other setting.
