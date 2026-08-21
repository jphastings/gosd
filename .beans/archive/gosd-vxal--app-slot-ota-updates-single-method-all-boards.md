---
# gosd-vxal
title: App-slot OTA updates (single method, all boards)
status: scrapped
type: epic
priority: deferred
created_at: 2026-07-04T21:04:04Z
updated_at: 2026-08-21T04:40:26Z
parent: gosd-p3zw
---

Umbrella for the accepted A/B design: OTA updates replace only the APP via slots on GOSD-BOOT; kernel/initramfs/bootloader are reflash-only. Single board-agnostic mechanism per docs/design/ab-updates.md (merged PR #15). The initramfs-baked /app is the immutable factory fallback; rollback ladder: new slot → previous good slot → factory. Do not start before v0.2 ships.


## Reasons for Scrapping

**JP, 2026-08-21: OTA is dropped entirely. Reflashing becomes the permanent,
only update path.** This scraps the whole chain — this epic and its children
gosd-6k2n, gosd-1epa, gosd-b4ns, gosd-mpr4 and gosd-xkkm, plus gosd-522n (the
phase-2 boot-file self-update design, which was blocked on this bean).

### Why

Reflash is already the documented baseline. `docs/design/upgrade-path.md`
locked "non-destructive plain reflash" as the upgrade path on 2026-07-31, and
CLAUDE.md has said so ever since. Since then two things shipped that make a
reflash keep what a device already had, and together they close most of the
gap this epic existed to close:

- a `--data-size=expand` image **re-adopts its own data partition** on first
  boot (gated on the image's configured data label), so app data survives an
  Imager reflash; and
- the config store in `/data` (`cmd/gosd-init/internal/configstore`, bean
  gosd-87ip) records every setting that differs from what the image shipped
  and **puts the operator's own settings back** onto the newly flashed card —
  hostname, WiFi credentials, hand-edited `env/` values and all.

So an upgrade already costs an operator one Imager run, exactly what they did
the first time, and loses neither their data nor their settings. What OTA adds
on top of that is convenience and reach, not capability — and it costs a
network listener, a per-image HMAC key, a slot store with FAT's weak
atomicity, a probation supervisor, a three-rung rollback ladder and a new CLI
verb, all of them permanently maintained on an appliance whose whole security
posture is "no interactive surface at all".

### What is given up — stated plainly

**There is no way to fix a deployed fleet without physical access to each
card.** Every device needs someone to power it down, pull the SD card,
reflash it and put it back. For a rack or a room that is an afternoon; for
devices posted to end users it means a card in the post, or talking a
non-technical operator through Raspberry Pi Imager a second time. A security
fix in an app, a bad release, a WiFi network that changed — each of them is a
physical visit. That cost is accepted with eyes open, not overlooked, and it
is the reason this decision is recorded rather than merely acted on.

It reopens only if that turns out to be the wrong trade in practice — a real
fielded fleet that cannot be reached, not a hypothetical one.

### Follow-through

The design record is kept, not deleted: a design that says "we considered
this and chose not to" is worth having; one that reads like a plan is
misleading. `docs/design/ab-updates.md` and `docs/design/upgrade-path.md`
(both the top of the document and its phase-2 section) now open with a note
recording this decision, its date and its reasoning. CLAUDE.md's
no-interactive-surface decision no longer promises a future update endpoint —
mDNS is the only listener in gosd-init, full stop — and COMPATIBILITY.md no
longer says GoSD "will gain OTA app updates". `docs/runtime.md`'s persistence
section, `cmd/gosd-init/internal/dataexpand` and `internal/initcfg` had their
"once the update mechanism lands" wording corrected too.
