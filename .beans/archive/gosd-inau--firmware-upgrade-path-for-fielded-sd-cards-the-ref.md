---
# gosd-inau
title: Firmware upgrade path for fielded SD cards (the reflash-only gap left by app-slot OTA)
status: completed
type: feature
priority: normal
created_at: 2026-07-31T08:08:11Z
updated_at: 2026-07-31T10:26:08Z
---

`docs/design/ab-updates.md` §0 (epic `gosd-vxal`) locks OTA updates to the
app only: "the kernel, initramfs, and bootloader are reflash-only... this
document does not propose one". This bean is that proposal: figure out how
an SD card in the field moves to a new firmware version (kernel,
gosd-init/initramfs, boot files, bootloader — everything a reflash carries
today).

**Hard constraint:** the people operating these devices must not need a
terminal or anything more complex than what they already did (flash the
SD card via Imager, per docs/flashing.md). Whatever we pick has to be at
most that hard.

## Candidate routes (JP's two, plus variants to evaluate)

**1. Self-update** — the board downloads the updated image and applies it
at an appropriate moment, ready for a restart. Never works for boards
without an internet connection. Notes from existing design work: this
could ride the app-slot update endpoint's auth/transport (per-image HMAC,
concurrent-push rejection — ab-updates §6) or poll the same published
catalog URL developers already host. The hard part is power-loss safety
while replacing the pieces the FACTORY-image story deliberately never
touches: Pi boot files are FAT files (tryboot/bootcount exist — see
ab-updates Appendix A), but the Rockchip bootloader lives in raw sectors
outside any partition, rewritten in place by a running system. Appendix
B's kexec-chooser is the recorded escape hatch for kernel-level OTA and
should be re-weighed here.

**2. Custom flasher tool** — overwrites only the GOSD-BOOT volume,
keeping GOSD-DATA and preserving gosd.toml (and any other files not owned
by the new firmware). Open question (JP): how does a new firmware
*delete* files a previous firmware shipped? Candidate answer to evaluate:
the image ships a manifest of firmware-owned paths on GOSD-BOOT (it
already carries config.json); the flasher deletes old-manifest-minus-
new-manifest, preserves everything unmanifested (gosd.toml, user files).
Two more things this route must answer: (a) on Rockchip, the bootloader
is NOT on GOSD-BOOT — a boot-volume-only flasher either also rewrites the
idbloader/u-boot raw ranges or declares the bootloader pinned forever;
(b) the tool must be a GUI on Windows/macOS/Linux (operators flash from
all three today), which is a real maintenance surface — who builds,
signs, and hosts it?

**3. (For evaluation) Make plain Imager reflash non-destructive.** The
zero-new-tooling route: operators re-flash exactly as before. Today that
wipes GOSD-DATA because writing the image rewrites the MBR, dropping
partition 2's entry even though the data bytes past the image length
survive. `--data-size=expand` (bean `gosd-6sac`) already gives first boot
the machinery to create partition entries; it could plausibly *re-adopt*
an orphaned GOSD-DATA (scan the expected offset for a FAT32 volume
labelled GOSD-DATA, re-add the entry instead of formatting). gosd.toml
would still be re-provisioned by Imager's wizard — operators re-enter
WiFi/hostname, which is the flow they already know. If acceptable, the
"upgrade tool" is just... Imager, again.

**4. (For evaluation) Sneakernet/staged update** — operator drops an
update bundle onto the card's data volume (card reader, or the USB
mass-storage gadget) and gosd-init applies it to GOSD-BOOT early on next
boot, before anything is loaded. Covers offline boards without building a
flasher GUI; shares route 1's staging/verify/power-safety design and
Rockchip raw-sector problem.

## Cross-cutting questions the design must answer

- What is "a firmware version" for GoSD? Each image bundles CLI version +
  artifacts pin + the developer's app — upgrade granularity and
  compatibility gating (app vs firmware coupling) need defining.
- Discovery: how does anyone learn an update exists? The hosted
  os_list.json catalog is the natural channel for both self-update polls
  and flasher tools.
- Power-loss safety per route, especially Rockchip raw-sector bootloader
  writes; what's the recovery story when an upgrade dies mid-write?
- Authenticity: the app-slot HMAC design covers app pushes; firmware
  needs an equivalent (or explicit trust-the-host reasoning).
- Whether GOSD-DATA preservation and gosd.toml preservation are both
  requirements, or data-only is enough (route 3 preserves data but not
  provisioning; JP's route 2 preserves both).

## Todos

- [x] Design spike (docs/design/upgrade-path.md): four routes evaluated; route 3 (non-destructive reflash) is the baseline with the provisioning-snapshot mitigation, route 1 is phase 2, route 4 folds into it, route 2 not pursued
- [x] Stale-file-deletion answered: dissolves under route 3 (flash rewrites all of GOSD-BOOT); manifest scheme deferred to phase-2 self-update (gosd-522n)
- [x] Rockchip bootloader stance decided (JP, 2026-07-31): pinned, full reflash only
- [x] Split implementation beans: gosd-lirl (GOSD-DATA re-adoption), gosd-acdn (image identity), gosd-ry3b (provisioning snapshot/self-heal, blocked by gosd-acdn), gosd-zlee (docs), gosd-522n (phase-2 design, blocked by gosd-vxal)


## Summary of Changes

docs/design/upgrade-path.md written and the route decision locked with JP
(2026-07-31): plain Imager reflash becomes the baseline upgrade path, made
non-destructive by GOSD-DATA re-adoption (expand-mode images) and a
provisioning snapshot in /data that self-heals gosd.toml hand-edits and
wizard-skipped WiFi on the first boot after a reflash. Rockchip bootloader
pinned (full reflash only). Self-update over the network is phase 2 riding
gosd-vxal; the custom flasher GUI route is not pursued. Five follow-up
beans filed (see checked todo).


**Addendum (JP, 2026-07-31, follow-up on PR #157 review):** no fixed boot
volume size is reasonable (Betamin needs >1GB; most apps don't). Decision:
per-app `gosd build --boot-size` (new bean gosd-m70t), and the shipped
size is that app's layout ABI — changing it in a later release erases the
data volume on upgrade, cleanly, as a documented release-level breaking
change. Consequence: dataexpand derives the data offset from the flashed
MBR instead of a mirrored constant (folded into gosd-lirl). The
generation-marker idea is dropped as unnecessary — a size change IS the
break, and nothing at flash time could act on a marker anyway.
