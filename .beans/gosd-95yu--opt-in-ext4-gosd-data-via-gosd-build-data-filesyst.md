---
# gosd-95yu
title: Opt-in ext4 GOSD-DATA via gosd build --data-filesystem (FAT32 stays the default)
status: completed
type: feature
priority: normal
created_at: 2026-08-09T06:11:08Z
updated_at: 2026-08-09T09:35:44Z
---

Let an app choose ext4 for the SD card's data partition at build time, for crash resilience under rapid power-off; FAT32 remains the default so a flashed card stays readable in any computer's SD reader.

## Why

GOSD-DATA is FAT32 today so a flashed card reads in any computer's SD reader — that stays the default, and is the point of the default. But some apps need /data to recover from crashes and rapid power-off better than FAT32 can: ext4's journal buys metadata crash-consistency and mount-time replay. As with the `disk`/`emmc` ext4 default (epic gosd-lfu0), the journal is NOT data durability — the four-step fsync pattern in docs/runtime.md remains the app-facing contract regardless of filesystem.

This amends gosd-lfu0's non-goal ("/data on the SD card stays FAT") at JP's request (2026-08-09): the non-goal's rationale — removable-media interop — is preserved by keeping FAT32 the default; ext4 becomes an explicit opt-in for apps that choose crash resilience over host readability. The original v1 blocker recorded in gosd-xelb ("there is no pure-Go mkfs.ext4") is gone: `internal/diskfmt.FormatEXT4` writes the checked-in golden ext4 image pure-Go, no root, on macOS.

## Locked decisions

1. **Flag: `gosd build --data-filesystem`, values `fat32` (default) | `ext4`.** Token spellings mirror `diskfmt.FS`/`disk.Filesystem`. exFAT is deliberately NOT offered here (gosd-mt53 Route B tracks it separately) but the value space must admit it later without breaking. Build-time only: the choice is baked into config.json (initcfg) — no gosd.toml key, no GOSD_* env var. The data filesystem is on-card ABI, not a provisioning tweak.
2. **The data filesystem is part of the app's on-card ABI, like `--boot-size`** (docs/design/upgrade-path.md): a `--data-size=expand` survivor is adopted only when its established filesystem matches the baked choice; a mismatch is treated like any other non-established debris — cleanly erased and re-established with the new filesystem. **The existing over-the-top reflash recovery path must keep working with ext4**: when a new image is flashed onto a card whose fresh MBR no longer references the old GOSD-DATA, first-boot re-adoption (docs/design/upgrade-path.md — survivor found at the MBR-derived offset, partition table re-pointed at it) recovers an established ext4 survivor exactly as it recovers a FAT32 one today, keeping its data, with journal replay happening on the re-adopting mount. Changing an app's data filesystem between releases is a release-notes-level breaking change.
3. **Boards whose stock kernel lacks CONFIG_EXT4_FS reject `--data-filesystem=ext4` at build time** with an actionable error naming the boards and the remedy. Today that is all three Pi boards (pi-zero-w, pi-zero-2w, pi-3b: `# CONFIG_EXT4_FS is not set` in their kernel.config); the Rockchip fleet, cubie-a5e, and qemu-virt have ext4 (matches COMPATIBILITY.md and `blockmount.remedyFor`'s wording). Since a bare `gosd build` builds all public boards, the error must say "pass --board to restrict, or drop --data-filesystem=ext4". Enabling ext4 in the Pi family kernel pin is deliberately NOT in scope — see the open question.
4. **`--data-flush` together with `--data-filesystem=ext4` is a build-time error** — `flush` is a vfat-only mount option (`internal/blockmount/mountoptions.go`; decision gosd-9m1k). A runtime gosd.toml `data_flush` override on an ext4 image is logged and ignored, never a boot failure.
5. **ext4 requires `--data-size` of at least the golden image size (512MiB) or `expand`**, validated at build time.
6. **Crash-ordering follows the house discipline**: golden write → sync → (first-boot) grow → sync → `gosd-data-established` marker → sync; adoption gated on the marker, never a probe. Precedents: dataexpand's existing marker and `blockmount.runEXT4`'s five-step sequence. Adversarial review pass before requesting JP's review, per CLAUDE.md.

## Mechanism (facts from current code)

- **Build**: `internal/image/image.go` formats partition 2 via go-diskfs `TypeFat32` directly (image.go:312), not through diskfmt, and hardcodes MBR type `Fat32LBA` (0x0C) for both partitions (image.go:262,270). Nothing writes files into the data partition at build time — it ships empty. ext4 route: write the `diskfmt.FormatEXT4` golden into the partition region; MBR type 0x83.
- **Grow**: `FormatEXT4` emits a fixed 512MiB filesystem and explicitly does not grow (ext4format.go); growing is `EXT4_IOC_RESIZE_FS` on a live Linux mount (`blockmount/platform_linux.go`, `GrowEXT4`). So EVERY ext4 data partition — fixed size > 512MiB or `expand` — grows exactly once on first boot, gated by the established marker. Fixed-size FAT32 images need no first-boot step today; fixed-size ext4 introduces one.
- **dataexpand** (`cmd/gosd-init/internal/dataexpand`) is FAT32-hardcoded end to end: `FormatFAT32` at final size in one shot, `survivorPresent`/`verifyEstablished` gate on `FS == diskfmt.FAT32`, partition type 0x0C. Its 256GiB ceiling is a FAT32-formatter limit (gosd-8kdm) that does not apply to ext4 — so this bean partially relieves gosd-mt53 for apps that accept ext4 (a relationship, not subsumption; gosd-mt53 stays open for FAT-family interop cards).
- **Mount**: gosd-init hardcodes `"vfat"` plus the flush option (`cmd/gosd-init/internal/boot/mounts.go:233`); the fstype must come from the baked config instead. Keep `msNoSuid|msNoDev`. `blockmount.Mountable` (/proc/filesystems) is the on-device preflight precedent for an actionable serial-console error.
- **Plumbing pattern to mirror**: the `--data-flush` chain (build flag → `pipeline.Options` → `initcfg.Config` JSON → boot resolver), minus the toml/env override layers per decision 1.
- The golden image is embedded compressed (~17KiB), so carrying it in gosd-init for the expand route costs nothing meaningful.

## Todos

- [x] `--data-filesystem` flag with validation (per-board kernel support, size ≥ 512MiB, `--data-flush` conflict), plumbed through pipeline → initcfg
- [x] Build-time ext4 data partition: golden write via `diskfmt.FormatEXT4`, MBR type 0x83, label GOSD-DATA
- [x] gosd-init: mount fstype from baked config (ext4, no flush option), /proc/filesystems preflight with actionable error
- [x] First-boot grow-once for fixed-size ext4, and a dataexpand ext4 route for `expand` (create partition → golden → grow partition → resize fs → marker), with survivor gates extended to match the baked filesystem
- [x] Fixture-driven build integration test reading the image back (MBR 0x83 + ext4 superblock label, network tripwire — pattern in `cmd/gosd/build_integration_test.go`)
- [x] qemu-virt CI: boot an ext4-data image, write, hard kill, reboot, adopt + journal replay (precedent: the `qemu-disk-ext4` job), plus an over-the-top reflash case: flash a fresh ext4-data image over the card, boot, and verify the old ext4 GOSD-DATA is re-adopted with its data intact
- [x] Docs: COMPATIBILITY.md per-board support in the same PR; build docs for the flag including the host-readability tradeoff (an ext4 card can't be read or repaired from a macOS/Windows host — gosd-6cf2's sticky-/data lesson applies doubly, so state-file self-healing guidance matters more); runtime.md pointer that the fsync contract is unchanged
- [x] Amend gosd-lfu0's non-goal line to reference this bean

## Open question for JP

Should the Pi family kernels gain CONFIG_EXT4_FS so ext4 GOSD-DATA works there too? That is a family-wide commit-pin change plus the artifacts release dance, and kernel size on pi-zero-w is the sensitive spot. If yes, file it as a separate bean; until then decision 3's build-time rejection covers the gap honestly.

## Summary of Changes

`gosd build --data-filesystem=fat32|ext4` (default `fat32`). Everything below is inert for a
default build: the FAT32 path is byte-for-byte what it was, pinned by tests that assert the
default partition type and filesystem explicitly so it cannot flip silently.

**Build time.** `internal/diskfmt` gained `WriteEXT4` (an exported wrapper over the existing
golden-image streamer, taking an `io.WriterAt` so a caller owns the offset) plus `MinEXT4Bytes`
and `EXT4SizeLimitReason` mirroring their FAT32 counterparts. `internal/image` gained
`Spec.DataFilesystem`; for ext4 it writes partition 2 as MBR type 0x83 through an
offset-confined `io.WriterAt` that rejects negative offsets and overruns (so an ext4 write
cannot reach the boot partition or run off the image), and skips the
`LargestSelfConsistentFAT32Bytes` trim, which is a go-diskfs FAT32 workaround with no meaning
for ext4.

**The grow, which is the structurally new part.** `FormatEXT4` writes a fixed 512MiB golden and
cannot grow; growing is `EXT4_IOC_RESIZE_FS` against a live Linux mount. So *every* ext4 data
partition — fixed-size as well as `expand` — ships the golden and grows exactly once on first
boot. That is new work on a path (fixed `--data-size`) that previously needed no first-boot step
at all, and it is why `dataexpand` now runs for any ext4 image rather than only `expand` ones.
gosd-init reuses `internal/blockmount`'s existing linux helpers (`GrowEXT4`,
`EstablishEXT4Marker`, `EXT4MarkerEstablished`, `Mountable`, `Mount`/`Unmount`) rather than
reimplementing them, behind the usual Deps seam so the tests still run on macOS.

**Crash ordering.** The expand path keeps its invariant that an MBR entry only ever exists over a
filesystem proven finished, with the mount-based establishment slotted into FAT32's existing
shape: format → sync → mount+grow+marker(fsync file, fsync dir)+unmount → sync → MBR entry. Power
loss anywhere before the entry leaves no entry, so the next boot redoes everything and converges.

The fixed-size path needed a different argument and gets a weaker, safer one: with an entry
already present it **only ever grows and marks, never formats or erases**. Growing is
non-destructive and `EXT4_IOC_RESIZE_FS` returns success as a no-op once the filesystem already
matches the requested size, so a missing marker — including one an app deleted — costs a
redundant ioctl rather than data. That is why this path needs none of `blockmount.runEXT4`'s
`RootHasOtherContent` second opinion.

`EXT4Established`'s mount-failure semantics are deliberately asymmetric, documented at both call
sites: on the adoption path a failure folds into "treat as debris" (nothing has an MBR entry yet,
so nothing can be lost), while on the already-established path it propagates as a plain error and
is never read as "not established" — that would risk growing against a filesystem this boot never
proved anything about. Neither is ever `ErrDataCorrupt`.

**Re-flash recovery is preserved for ext4.** `survivorPresent` is now filesystem-aware, so an
`expand` image flashed over a card whose MBR no longer references the old GOSD-DATA re-adopts an
established ext4 volume with its data intact, exactly as it does a FAT32 one. CI proves it.

**Refusals, all before anything is compiled or assembled**, alongside the existing `--usb-gadget`
check: an unknown value; a selected board whose pinned kernel lacks `CONFIG_EXT4_FS` (the three Pi
boards — the error names them, names which selected boards *do* support ext4, and says to restrict
with `--board` or drop the flag, which matters because a bare `gosd build` builds every public
board); `--data-flush`, which is a vfat-only mount option; `--data-size=0`; and a fixed
`--data-size` below the golden's 512MiB. The 256GiB `--data-size` ceiling is a FAT32 formatter
limit and no longer applies to ext4, so `expand` + ext4 fills the whole card.

`internal/boards` gained `EXT4Support()` on the `Board` interface, mirroring `UsbGadgetSupport`.
`partitionSectors`' 256GiB cap is likewise now FAT32-only.

**Not in Identity.** `ComputeIdentity` excludes config.json wholesale and this field has no other
footprint in the hashed payload, so it is structurally excluded like `DataExpand`/`DataFlush` —
pinned by a test, with the reasoning in the field's docstring. Worth noting it is *not* excluded
for `DataFlush`'s reason: the data filesystem genuinely does change the on-card layout.

**Verification.** Full gates green (`go test ./...`, `go vet` on both GOOS, `gofmt`,
`golangci-lint` on darwin and linux). Each refusal was exercised through the real CLI. A real
`--board=rock-4se --data-filesystem=ext4 --data-size=1GiB` build was inspected byte-wise: p1
FAT32/0x0C untouched, p2 type 0x83 at 1.00GiB carrying an ext4 superblock (magic 0xEF53) labelled
GOSD-DATA, filesystem 512MiB inside a 1024MiB partition — the pre-grow state, as designed. A new
`qemu-data-ext4` CI job covers both shapes on the real boot path: create → grow → establish, hard
kill, re-adopt, then `dd` the pristine image back over the front of the card and assert the
orphaned ext4 partition comes back with `boots=3` rather than reformatted; plus a fixed-size image
proving the one-time 512MiB→1GiB grow.

**Bench verification is still outstanding** — no ext4 GOSD-DATA image has been booted on real
hardware, only under qemu. Follow-up beans filed: gosd-7bwv (bench verification) and gosd-ssth (the open Pi-kernel question).
