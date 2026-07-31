---
# gosd-mt53
title: 'Give /data more than 256 GiB: lift the FAT32 formatter ceiling'
status: todo
type: feature
priority: normal
created_at: 2026-07-31T03:31:51Z
updated_at: 2026-07-31T03:31:51Z
---

GoSD cannot give an app more than 256 GiB of `/data`. Three caps enforce that
today, all of them working around the same go-diskfs defect diagnosed in bean
`gosd-8kdm` (its FAT32 formatter counts sectors-per-FAT in a uint16, silently
truncating past 274,940,836,864 bytes / 256.06 GiB and panicking past ~512 GiB):

- `internal/diskfmt`'s `maxFAT32Bytes` + `checkFAT32Size` — `FormatFAT32`
  refuses any oversized device, so `disk` and `emmc` fail loudly instead of
  writing a corrupt volume.
- `cmd/gosd-init/internal/dataexpand`'s `maxPartitionBytes` — a
  `--data-size=expand` partition is capped at 256 GiB, with a log line saying
  how much of the card is left unused.
- `cmd/gosd`'s `parseDataSize` — `gosd build --data-size` above the limit is
  rejected at flag validation, naming the maximum and linking
  `docs/runtime.md#how-big-the-data-partition-can-be`.

This bean is where lifting them is decided. **Nothing is implemented here** —
pick a route first.

## What actually hurts

Only the `expand` cap costs a real user anything. A *fixed* `--data-size` above
256 GiB implies a `.img` file that size, which nobody downloads or flashes, so
the CLI rejection is a guard rather than a limitation. The case worth solving is
a 512 GB or 1 TB card in a board built with `--data-size=expand`, where today
the app silently gets 256 GiB and the rest of the card stays dark. Attached
storage is already unaffected: `disk` can format exFAT with no ceiling.

## Route A — fix go-diskfs upstream, then bump the pin

The patch is written and reviewed-pending in `gosd-8kdm` (compute
sectors-per-FAT in 64 bits, keep it in the uint32 the on-disk FATSz32 field
actually is), together with its upstream test and PR description. **Do not
duplicate it here**, and note CLAUDE.md's rule: sending it to
`diskfs/go-diskfs` is JP's call, not an agent's.

- Cheapest by far on our side: a pin bump plus deleting three caps, no new
  filesystem code to own, `disk`/`emmc`/`/data` all fixed at once.
- Costs: it runs on someone else's review and release schedule.
- Even after the fix, `fat32.Create` builds the whole FAT in RAM
  (`make([]uint32, fatSize/4)`) — roughly 120 MB for a 1 TB volume, on boards
  with 512 MB. That is a genuine gosd-init constraint and may need a cap of its
  own (or a streaming write) rather than none.
- FAT32 keeps its 4 GiB per-file ceiling however large the volume, and tops out
  near 2 TiB at 512-byte sectors regardless.

## Route B — format GOSD-DATA as exFAT

`internal/diskfmt` already writes exFAT from the Microsoft spec (bean
`gosd-1ici`), validated, pure Go, 32-bit FAT length, no 4 GiB per-file limit.
Reusing it for GOSD-DATA is the only route that removes both FAT32 ceilings.
What it costs:

- **gosd-init must mount it.** `cmd/gosd-init/internal/boot/mounts.go` mounts
  `/data` with a hardcoded `"vfat"`; it would need to detect the filesystem
  (`diskfmt.Inspect` already does) and mount `exfat`, with the read-only
  fallback when the kernel lacks the driver.
- **Kernel support is per board, and incomplete.** Checked against the recorded
  configs at the time of writing (`internal/artifacts.Version` = v0.8.0) —
  re-check the files rather than trusting this list:
  `pi-zero-2w`, `pi-zero-w`, `pi-3b` and `rock-4se` have `CONFIG_EXFAT_FS=y` in
  their published `kernel.config`; `radxa-zero-3e` and `nanopi-zero2` publish
  `# CONFIG_EXFAT_FS is not set` with their fragments already enabling it, so
  they gain it at the next artifacts release; **`qemu-virt` has neither** — its
  fragment does not enable exFAT, so the CI boot-to-HTTP board could not mount
  an exFAT `/data` until that changes.
- **The MBR partition type changes** from `0x0c` (Fat32LBA) to `0x07`, in both
  `internal/image` and dataexpand's partition-entry writer — and dataexpand's
  "is this a GoSD layout?" check reads those bytes.
- **Hosts:** Windows and macOS mount exFAT natively, Linux since 5.4. The "plug
  the card into any computer" promise weakens slightly, and old
  card readers/appliances lose it entirely.
- **Probably not a blanket switch.** FAT32 is right for the small partitions
  most images ship. The likely shape is FAT32 by default and exFAT only above
  the FAT32 ceiling — which means `/data`'s filesystem varies with card size and
  an app can no longer assume FAT semantics. That is a locked-decision call for
  JP, not an implementation detail.
- exFAT is no more power-loss-robust than FAT; nothing in `docs/runtime.md`'s
  durability advice changes.

## Route C — write our own FAT32 formatter

We already write a harder filesystem from spec, and a FAT32 layout is
well-trodden; owning it would give full control of cluster size (64 KiB
clusters reach FAT32's own ~2 TiB architectural ceiling) and drop the
dependency entirely.

- Most new code to own, in the one place a bug is silent data loss, and
  go-diskfs stays the *reader* either way, so we would be testing our writer
  against their reader plus real kernels.
- Only worth it if Route A stalls and Route B is rejected.

## Route D — ask go-diskfs for 4096-byte sectors (cheap, unverified)

`fat32.Create` accepts a blocksize of 512 *or 4096*, and picks cluster bytes
from the volume size in bytes, so passing 4096 divides the sectors-per-FAT
count by eight and moves the uint16 ceiling to roughly 2 TiB with no upstream
change at all.

- But it changes on-disk geometry for every volume we write
  (`BytesPerSector` 4096) while our partitioning is 512-byte-sector throughout,
  and Windows in particular expects bytes-per-sector to match the device's.
- The uint32 numerator wrap moves with it rather than going away.
- Unverified: it needs a real card read on Linux, macOS and Windows before it
  is more than a curiosity. Listed because it is the only route that costs
  nothing to try.

## Acceptance, whichever route wins

- A >256 GiB card gives its whole remainder to `/data` under
  `--data-size=expand`, verified on real hardware (not just a sparse file).
- **All three caps go in the same PR**, plus the prose that explains them:
  `docs/runtime.md`'s "How big the data partition can be" and "FAT32 or exFAT"
  sections, and COMPATIBILITY.md's `[^exfat]` footnote. Removing one alone
  leaves a story that contradicts itself.
- Nothing at or below the current ceiling changes shape — existing cards keep
  mounting.

## Todos

- [ ] JP picks the route (A is the default if upstream moves; B is the only one
      that also lifts the 4 GiB per-file ceiling)
- [ ] Implement it and remove all three caps together
- [ ] Bench-verify on a >256 GiB card, both `--data-size=expand` and a fixed
      size at the old boundary
