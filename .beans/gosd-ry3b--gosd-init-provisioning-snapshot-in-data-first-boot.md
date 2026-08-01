---
# gosd-ry3b
title: 'gosd-init: provisioning snapshot in /data + first-boot-after-reflash self-heal'
status: completed
type: feature
priority: normal
created_at: 2026-07-31T09:17:50Z
updated_at: 2026-07-31T09:17:50Z
blocked_by:
    - gosd-acdn
---

Phase 1 of the upgrade-path design (bean gosd-inau, docs/design/upgrade-path.md §3). Snapshot effective gosd.toml + contemporaneous baked [env] defaults + image identity into /data/.gosd/provision-snapshot/ (durable-write rules) after provisioning settles each boot. On first boot after reflash (image identity differs): restore provable hand-edits into the new card's gosd.toml (written back to GOSD-BOOT so the operator can still see/edit), restore hostname/WiFi only when the fresh boot provides none (wizard-skipped flow). Locked invariant: wizard wins over snapshot; snapshot wins over baked defaults. Exact merge rules decided here within that invariant. No data partition = no snapshot = no self-heal (documented).

## Summary of Changes

New package `cmd/gosd-init/internal/provsnapshot` (pure decision logic, file
IO behind a `Deps` seam, fake-driven tests that pass on macOS), called from
`boot.Run` immediately after the data mount — before the WiFi/env decisions,
so a restore takes effect on the boot that performs it rather than the next
one.

### Snapshot format (decided here)

`/data/.gosd/provision-snapshot/` holds two files:

- `gosd.toml` — the provisioning this boot **settled on** (hostname, WiFi,
  `[env]` after the locked `gosd.toml > cloud-init > config.json` merge),
  rendered with `gosdtoml.Render`, the same template the CLI writes onto an
  image. Deliberately the *effective* values rather than a copy of the card's
  gosd.toml: wizard-supplied provisioning never appears in gosd.toml at all,
  and rescuing exactly that case is what §3 rule 3 is for.
- `snapshot.json` — schema version, the image identity the snapshot was taken
  under, the contemporaneous baked defaults from config.json
  (hostname/wifi/env), and a SHA-256 of `gosd.toml`.

`snapshot.json` is written **last and acts as the commit record**: a snapshot
whose gosd.toml doesn't match the recorded digest is a torn write and is
ignored wholesale, the same "a marker proves everything before the barrier
landed" reasoning [[gosd-lirl]] used for `gosd-data-established`. Both writes
use the full `docs/runtime.md` durable sequence (new
`provsnapshot.WriteFileDurably`: write temp → fsync file → rename → fsync
file → fsync dir), as does the gosd.toml write-back to GOSD-BOOT (new
`boot.Platform.WriteBootFile`, which brackets it with the read-write /
read-only remounts, and reports a failure to restore the read-only mount).

### Exact merge rules (within the locked invariant)

Each field — hostname, the WiFi ssid/passphrase pair, and every `[env]` key
independently — is classified on the same three-way test:

- **Fresh intent**: a cloud-init hostname/WiFi left by the wizard, or a
  gosd.toml value differing from the *running* image's baked default (a
  freshly flashed card's gosd.toml is the rendered template, so any
  difference is a hand-edit made before this boot).
- **Snapshot intent**: the snapshot's effective value differs from the baked
  default recorded in that same snapshot (the contemporaneous default).
- Otherwise it is just a baked default.

**A field is restored iff there is no fresh intent for it and there is
snapshot intent for it.** Consequences:

- New image's template changed a key's default and the snapshot value equals
  the *old* default → no snapshot intent → the new default wins.
- Operator hand-edited a key *and* the new image changed its default → the
  hand-edit is restored (snapshot beats baked defaults).
- A key the operator added that no image ever baked → restored (it differs
  from the absent/empty contemporaneous default).
- WiFi is restored as a pair, so a changed passphrase for an unchanged SSID
  counts as a hand-edit. A restored passphrase may be cloud-init's 64-hex
  PSK; `wifiup` already distinguishes the two by shape.
- Fresh intent only *blocks* a restore; it is never written into gosd.toml,
  so the locked precedence chain still decides which of the two takes effect.

Restores are written back to `gosd.toml` on GOSD-BOOT (re-rendered from the
CLI template: values survive, an operator's own added comments do not) and
applied in memory for the boot in progress.

### Failure behaviour

Nothing here can stop a boot. Missing, torn, malformed or future-schema
snapshot → logged, ignored, boot continues without a self-heal. Read-only or
absent `/data` → one log line, no snapshot. No image identity on either side
(pre-[[gosd-acdn]] image, or a snapshot taken by one) → snapshot still kept
up to date, self-heal skipped with a log line. If the GOSD-BOOT write-back
fails, the restore still applies to this boot and the snapshot is
deliberately **not** refreshed, so the next boot sees the same identity skew
and retries rather than silently forgetting the restore. An unchanged
snapshot is never rewritten (structural comparison first — every FAT write is
an erase-block rewrite).

### CI

The existing `qemu-expand-data` job gained two greps, no new boots: the first
boot must log `provisioning snapshot saved` (proof it reaches a real FAT
filesystem, not just the unit-test fakes) and the second must log
`provisioning snapshot unchanged` — which, since qemu is killed abruptly
after the first boot, is also a durability assertion on the snapshot write.

### Observation for a follow-up (not changed here)

`gosd build` always renders `hostname = "..."` into gosd.toml (the flag
defaults to the sanitized package name), and gosd.toml outranks cloud-init in
the locked precedence chain — so an Imager wizard *hostname* is shadowed on
every image today, wizard WiFi is not. This package records what took effect,
so it is faithful either way, but the wizard-hostname path is worth a bean of
its own.
