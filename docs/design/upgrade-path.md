# Design spike: firmware upgrade path for fielded SD cards

Bean `gosd-inau`. This is the piece `docs/design/ab-updates.md` §0
deliberately left open: the kernel, initramfs, boot files — everything a
reflash carries — are "reflash-only", and this document decides what
"reflash" means for a device that is already in someone's hands.

**The operator constraint (locked):** the people running these devices
never use a terminal. Upgrading must be at most as hard as what they
already did — flash the SD card with Raspberry Pi Imager per
`docs/flashing.md`.

## 0. Decisions locked in this spike (JP, 2026-07-31)

1. **The Rockchip bootloader is pinned: full reflash only.** idbloader and
   u-boot.itb live in raw sectors outside any partition; no boot-volume
   mechanism can touch them, and no in-place rewrite of them will be
   designed. A bootloader bump ships as a new image the operator flashes —
   which this design makes non-destructive. (Bootloader bumps are rare and
   per-family; this is the same posture ab-updates took for the kernel.)
2. **Route 3 (non-destructive plain reflash) is the baseline upgrade
   path, contingent on a config-tree self-healing mitigation** — losing
   the operator's hand-edits (or forcing WiFi re-entry on a board whose
   provisioning came from a card edit rather than the wizard) was judged
   not acceptable without one. The mitigation is §3 (shipped as the config
   store, `cmd/gosd-init/internal/configstore`, bean `gosd-87ip` — its
   mechanics ended up per-setting-file rather than the whole-document
   snapshot sketched below, but the requirement and the locked precedence
   are unchanged).
3. **Route 1 (self-update over the network) is phase 2**, riding the
   app-slot update machinery (`gosd-vxal`) once it exists. Route 4
   (sneakernet bundle) folds into phase 2's staging design. Route 2 (a
   custom flasher GUI) is not pursued: its only unique win over route 3 +
   mitigation is preserving files the mitigation already preserves, and it
   costs a signed, maintained GUI on three OSes.
4. **The boot volume size is per-app, not a GoSD constant** (JP,
   2026-07-31). There is no reasonable fixed size: Betamin (the mpv video
   appliance driving `docs/externals.md`) needs a boot volume over 1GB,
   and forcing every deployment to carry the largest app's headroom is
   not acceptable. `gosd build` grows a `--boot-size` (default: today's
   256MiB), and the size an app ships becomes **that app's layout ABI**:
   a later release that changes it (either direction) erases the data
   volume on upgrade — cleanly, via the adoption gate in §2, never as
   corruption — and must say so at release level. Consequence for §2:
   nothing on the device may assume a fixed data-partition offset; it is
   derived from the flashed MBR (§2). Side benefit: per-app sizing is
   also what makes app-slot OTA (`gosd-vxal`) workable for large apps —
   two slots of a Betamin-sized payload never fit in a fixed 256MiB.
5. **The data partition's filesystem is per-app, exactly like its size**
   (JP, 2026-08-09, bean `gosd-95yu`). `gosd build --data-filesystem`
   chooses `fat32` (default, universally readable) or `ext4` (journaled,
   gated per-board — see `COMPATIBILITY.md`'s ext4 data partition row), and
   like `--boot-size` in decision 4, that choice becomes part of the
   app's on-card layout ABI: a FAT32 data partition is not an ext4 one, so a
   later release that switches filesystems can't simply adopt what's
   already on the card. §2's adoption gate treats a filesystem mismatch
   the same way it already treats a boot-size-driven offset mismatch — a
   clean reformat to the new choice on the next upgrade, never
   corruption, and a release-notes-level breaking change for that app.
6. **The boot/data volume labels are per-app, and are on-card ABI too**
   (JP, 2026-08-09, bean `gosd-lo7k`). Labels stop being the fixed
   `GOSD-BOOT`/`GOSD-DATA` and become `<prefix>-boot`/`<prefix>-data`,
   where `<prefix>` defaults to the app's sanitized name (truncated to 6
   bytes) and is overridable with `gosd build --label-prefix`. Exactly
   like decisions 4 and 5, the label pair joins `--boot-size` and
   `--data-filesystem` as part of the app's on-card layout ABI: §2's
   adoption gate now checks the data partition's label against this
   image's configured data label, so changing the prefix (or renaming the
   app, since the prefix defaults to it) between releases means the next
   reflash-upgrade finds the old label, treats the partition as debris,
   and cleanly reformats it — never a halt, and the boot partition is
   unaffected. **Clean break, no migration:** cards flashed by a
   pre-`gosd-lo7k` release carry `GOSD-DATA`, which no image built after
   this change will ever recognize as its own, so their first
   reflash-upgrade reformats the data partition — a release-notes-level
   breaking change. A side effect: cross-app reflash no longer silently
   inherits the previous app's data the way sharing one universal
   `GOSD-DATA` label always did — unless two apps happen to share the
   same 6-byte prefix, which re-adopts across them exactly as before,
   just narrower.

## 1. The four routes, against the constraint

| Route | Operator effort | Offline boards | Data partition | Config tree | New tooling |
|---|---|---|---|---|---|
| 1 self-update | zero (automatic) | ✗ never works | survives | survives | update endpoint (gosd-vxal) |
| 2 custom flasher | new tool, same clicks | ✓ | survives | survives | GUI × 3 OSes, signed, maintained |
| 3 reflash, non-destructive | identical to first flash | ✓ | survives | via mitigation (§3) | none |
| 4 sneakernet bundle | copy file to card/USB drive | ✓ | survives | survives | bundle format + apply-at-boot |

Route 3 wins the baseline because the operator's mental model doesn't
change at all: "new version? flash the card again, like last time." Its
two gaps — data loss on reflash and provisioning loss — are closed by §2
and §3. Route 1 is the right end state for connected fleets and is
already half-designed (`ab-updates` §6's endpoint, HMAC auth, and
`gosd push` flow); extending it from app slots to boot-file payloads is
phase 2, out of scope here. Route 4 shares phase 2's staging/verify
design and is deferred with it.

## 2. Making plain reflash non-destructive (data partition re-adoption)

Why reflashing destroys `/data` today: writing the image rewrites the
MBR, whose partition table has no partition 2 — the entry is dropped even
though, for a `--data-size=expand` image, the image file *ends* at the
boot partition and the data region's bytes are never touched by the
flash. (A fixed-size image embeds a freshly formatted data partition in the
image itself, so the flash overwrites the data region directly — nothing
can save it. Consequence: **`--data-size=expand` is the recommended mode
for updatable deployments**, and the docs will say so.)

The fix is one insertion in `dataexpand`'s existing first-boot sequence.
Today both sides hardcode the data offset
(`internal/image.dataPartitionOffsetBytes` = 272MiB, mirrored as
`dataexpand.dataPartitionStartLBA`); with per-app boot sizes (§0.4) that
mirror is wrong by construction, so **dataexpand derives the data offset
from the flashed MBR instead: partition 1's start + size**, which the
image writer put there and the flash just rewrote. The mirrored constant
is deleted, not parameterized — the MBR is already on the card and is
always right for the image that was actually flashed.

```
offset := end of MBR partition 1        # CHANGED: derived, not constant
AddKernelPartition(...)                 # partition node appears (existing)
contents := Inspect(partitionDevice)    # NEW: look before formatting
if contents is FAT32 labelled with this image's configured data label:
    skip FormatFAT32                    # adopt the survivor
FormatFAT32(...)                        # otherwise, as today
WriteMBR(...)                           # commit record, as today
```

Power-loss safety is unchanged: the MBR write remains the commit record
(ab-updates' analysis of `gosd-6sac` holds — a crash before it lands
leaves no entry, and the next boot redoes everything, now finding and
adopting the same survivor).

Adoption is gated on **four** things: the derived offset, FAT32, the
app's configured data label (matched case-insensitively, so a host tool
that displays or rewrites it uppercased can't trigger a spurious reformat)
— and a **format-completion marker** dataexpand itself wrote
into the filesystem's root after the format's sync barrier (write file →
sync → marker → sync). The marker exists because a filesystem probe is
not proof of a *completed* format: go-diskfs writes the volume label
last with no intervening syncs, so a power cut during a first boot's own
format can persist label-bearing debris whose FAT tables never landed —
which a probe-only gate would adopt and commit forever, where today's
always-reformat code self-heals it. The marker's durable presence
implies everything before the barrier persisted, so only a genuinely
completed format (from any earlier life) is ever adopted; anything else
— including such debris — formats fresh, exactly as today. The marker is
a reserved root file named `gosd-data-established` — deliberately not a
dotfile, because go-diskfs derives an empty 8.3 short name for
leading-dot files and then filters them out of its own listings, making
a dotfile marker invisible to the very check that needs it. Like
`.gosd-data`, apps must leave it alone; its absence under an
already-committed MBR entry is deliberately NOT treated as corruption
(an app deleting it must not halt the device).

What happens when a release changes the boot volume size (§0.4's
caveat, mechanically): if it **grew**, the flash itself overwrote the
old data partition's superblock (the bigger image extends past the old data
offset), the new offset holds unrecognizable mid-partition bytes, and
dataexpand formats fresh — a clean wipe, indistinguishable from a first
flash. If it **shrank**, the old superblock actually survives (it lies
beyond the new image's end) but at an offset nothing looks at; adoption
stays exact-offset-only and we deliberately do NOT scan for it, so the
outcome is the same clean wipe. (Scanning candidate offsets to rescue
the shrink case is a possible future refinement, not part of this
design.) Either way a size change means data loss with a working device,
never corruption — and it is a release-notes-level breaking change for
that app (§0.4).

What re-adoption does NOT promise: schema compatibility of `/data`
contents across app versions is the app's own concern, same as after any
app update (`docs/runtime.md` already frames `/data` this way).

## 3. Self-healing the config tree across a reflash (the config store)

Requirement (locked): an upgrade must not silently discard the
operator's provisioning — hand-edited `env/` values, and WiFi/hostname
on boards provisioned by card-edit rather than the wizard.

Mechanism sketched at this spike's date: gosd-init snapshots the whole
effective boot-partition config into the data partition, and re-applies it
on the first boot after a reflash. **What actually shipped differs in its
mechanics** (`cmd/gosd-init/internal/configstore`, bean `gosd-87ip`, once
the settings tree — epic `gosd-rw6n` — replaced the single hand-editable
file this section was written against): rather than one whole-file
snapshot compared key-by-key against a remembered baked default, the store
keeps one entry *per setting file*, keyed on whether that file's bytes
differ from what the running image shipped it with — no copy of any old
default needed, since "differs from the image's own value" is decidable
from `config.json`'s per-file digests alone. The requirement this section
locks, and the freshest-intent-wins precedence, carried through unchanged;
see [the config tree's own
guide](../config.md#keeping-your-settings-across-a-reflash) and
[the runtime contract's
description](../runtime.md#keeping-settings-across-a-reflash-the-config-store)
for how it actually behaves. The rest of this section is kept for its
historical reasoning.

- **Record (every boot, after provisioning settles):** write an entry for
  every setting whose card file differs from the running image's own,
  keyed to the data partition. Durable-write rules apply
  (`docs/runtime.md` "Making a write durable"). No data partition → nothing
  kept → no self-healing; the docs say so (one more reason `expand`
  is the updatable-deployment default).
- **Detect "first boot after reflash":** the store's recorded image
  identity differs from the running image's (§4). Wizard re-provisioning
  is visible independently: fresh cloud-init files on the boot partition,
  consumed into the tree before this reconciliation ever runs.
- **Restore precedence (freshest intent wins):**
  1. Anything the operator just provided via the wizard (fresh cloud-init
     hostname/WiFi, already folded into the tree by the time this runs) or
     a pre-boot hand-edit is applied as normal and wins outright — the
     freshest statement of the operator's intent.
  2. A kept setting whose value differs from the newly flashed card's own
     is restored back onto the card.
  3. Anything neither kept nor present on the freshly flashed card falls
     back to this image's own baked defaults, exactly as on a first flash.
- The invariant this spike locks, still true of what shipped: **the card's
  own freshest value always wins over the kept copy, and the kept copy
  always wins over baked defaults.**

## 4. Image identity (small prerequisite)

`config.json` (`internal/initcfg`) currently records no build identity.
Add one — content-derived (e.g. a digest over the boot payload set), not
a timestamp, so identical rebuilds compare equal and the qemu CI path
stays deterministic. Used by: snapshot skew detection (§3), and later by
phase-2 self-update to answer "am I already running this?" the same way
the catalog answers it (`extract_sha256` — no semver scheme needed
anywhere).

## 5. The stale-file question, answered

"How does a new firmware delete files of a previous firmware?" — under
route 3 the question dissolves: the flash rewrites all of the boot
partition, so stale files cannot exist; the only thing that survives is
what §3 deliberately restores. The manifest-of-owned-paths scheme is
therefore NOT needed for the baseline. It returns in phase 2 (self-update
writes into a live boot partition and must delete what the new payload
doesn't carry) and is recorded there, not built now.

## 6. Phasing and implementation beans

Phase 1 (this design, buildable now):

- `gosd-m70t` — `gosd build --boot-size`: per-app boot volume size, with
  build-time fit validation and a usage report so developers watch their
  headroom (§0.4).
- `gosd-lirl` — dataexpand: derive the data offset from the flashed MBR
  (deleting the mirrored constant) and re-adopt an orphaned data partition
  on first boot (§2).
- `gosd-acdn` — image identity in config.json (§4).
- `gosd-ry3b` — provisioning snapshot + first-boot self-heal (§3;
  blocked by `gosd-acdn`).
- `gosd-zlee` — docs: the upgrade story (runtime.md, publishing.md,
  flashing.md's "upgrading" section; expand as the updatable-deployment
  default).

Phase 2 (deferred, new design work, after `gosd-vxal` lands its
endpoint): self-update of boot files over the network — staging area on
the boot partition, verify-then-commit, the manifest scheme from §5, and the
sneakernet bundle (route 4) as the offline carrier of the same payload
format. Tracked as `gosd-522n` (blocked by `gosd-vxal`).

## Acceptance for phase 1

An operator with a `--data-size=expand` deployment upgrades by flashing
the new image with Imager exactly as they first did (wizard or not):
their app data is intact, their hand-edited `[env]` values are back, a
wizard-skipping reflash rejoins WiFi by itself, and at no point did they
open a terminal.
