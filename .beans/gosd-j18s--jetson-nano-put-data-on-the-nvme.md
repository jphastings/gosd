---
# gosd-j18s
title: 'Jetson Nano: put /data on the NVMe'
status: completed
type: feature
priority: normal
created_at: 2026-11-07T20:59:10Z
updated_at: 2026-11-08T08:27:51Z
parent: gosd-72je
---

JP decided on 2026-11-07 that the eMMC module keeps `/data` on an NVMe drive, not on the eMMC. On the bench, that's the 477 GB drive in the Turing Pi 2's node 4 M.2 slot, which NVIDIA's U-Boot already sees as `nvme0`. The reasons:

- NVIDIA's GPT owns the eMMC.
- `APP` (partition 1, 14 GiB) holds GoSD's FAT boot partition.
- Every reflash rewrites the eMMC from scratch (see the flashing guide).

**Before this bean, on this board, nothing broke but nothing persisted:**
- gosd-init only looked for data at `mmcblk{0,1}p2` and `vda2`, which on this board is NVIDIA's `DTB` partition.
- `dataexpand` only understands GoSD's MBR, and refuses NVIDIA's GPT before it writes anything.

So the app ran with an empty, read-only `/data`.

## Todo

- [x] How a board declares "data lives on NVMe".
- [x] How gosd-init finds, creates and adopts the data volume on `nvme0n1`, reusing `internal/blockmount` (no second format/adopt path).
- [x] How the label (`<prefix>-data`), `--data-filesystem` and `--data-size` apply.
- [x] What happens with no NVMe fitted: fail fast with an actionable line.
- [x] A guarantee that nothing on the eMMC but `APP` is ever written.
- [x] Kernel: `CONFIG_BLK_DEV_NVME=y` and `CONFIG_NVME_MULTIPATH` off in the jetson-nano kernel (defconfig, per the board's `kernel.config` snapshot); `CONFIG_PCI_TEGRA=y` is in the fragment. Pinning `BLK_DEV_NVME` in the fragment is the sibling bean gosd-wxy7's.
- [x] Crash-ordering argument (below).
- [x] Independent adversarial review pass (CLAUDE.md: this formats and adopts on-disk state). It found no path that formats a drive holding someone's data; its fixes are in the second commit and folded into the sections below.
- [x] Bench, drive holding something else → refused (2026-11-07): node 4's drive held an ext4 volume labelled `DISKTEST`. gosd-init named it, refused to reformat it as `hello-data`, wrote nothing, and mounted `/data` read-only; the app started.
- [x] Bench: blank drive → formatted; reboot → adopted; reflash → adopted, settings restored (2026-11-08):
  - A throwaway app (image identity `29d5a75b7d4c`, with `config/hostname` set to `nanobench` in its boot partition before signing) zeroed the `DISKTEST` volume's first MiB.
  - The next boot logged `formatting the NVMe drive at /dev/nvme0n1 as ext4, labelled hello-data`, `/data is a new ext4 volume …` and `kept for the next re-flash: config/hostname`.
  - A reboot logged `/data is the ext4 volume labelled hello-data already on the NVMe drive`.
  - Reflashing plain `hello` (a different identity, default hostname) adopted the volume, logged `config/hostname restored from the copy kept …`, and came up as `nanobench` in that same boot.
- [x] Bench: drive removed → read-only `/data` at once (2026-11-08). The module moved to node 3, whose M.2 slot is empty. About 4 s after `/init` started, gosd-init logged `mounting data partition failed; mounting /data read-only: no NVMe drive found: the PCIe bus has had no NVMe controller on it for 3s; …`, matching the development notes, and `/app` started straight after.

## Design

1. **Declaration.** An optional capability, `boards.NVMeDataBoard` (`DataOnNVMe() bool`, helper `boards.DataOnNVMe`), shaped like `InitramfsLimiter`; jetson-nano implements it. The pipeline bakes it as `initcfg.Config.DataOnNVMe` (`dataOnNVMe` in config.json), and gosd-init acts on that bit alone: no board-ID check anywhere at runtime.
2. **What the build does with the data flags.** For such a board the pipeline zeroes `--data-size` (no partition 2 in the image) and `--data-size=expand`, drops `--data-flush` and bakes `dataFilesystem: ext4`; `gosd build` prints one line per such board saying so. Ignored rather than refused, because a bare `gosd build` builds every public board and these flags are legitimate for the others, and because the flashed image only ever contributes its boot partition to `APP` anyway. Known wrinkle, left alone: `--ingress tailscale-funnel`'s fleet-wide "needs `--data-size`" check still fires for a jetson-nano-only build; passing one is harmless since it's then ignored.
3. **Filesystem: always ext4, whatever `--data-filesystem` says.** FAT32's default exists so a card reads in any computer's SD reader, which a drive inside a carrier board isn't; GoSD's FAT32 formatter refuses volumes over 256 GiB, so the default would fail on the bench's 477 GB drive and on most NVMe drives; the `disk` package, the same machinery, defaults to ext4 for internal drives for the same reasons (epic gosd-lfu0); and ext4's journal matters on a device with no clean shutdown. gosd-init honours whatever config.json bakes rather than hard-coding ext4, so the choice lives in one place (the pipeline).
4. **Label and layout.** `<prefix>-data` (config.json's `dataLabel`), as a whole-device volume with no partition table: blockmount's `disk` shape, unchanged.
5. **Which device.** `cmd/gosd-init/internal/nvmedata` ranks only `/sys/block` names matching `^nvme\d+n\d+$` (namespaces; not partitions, not multipath paths), through `blockmount.Candidates`, so in-use, empty and read-only devices are excluded as for `disk`. After the first namespace appears it settles for 500 ms so siblings can appear too, then never guesses: one namespace is used; of several, the one carrying this app's volume (this label, this filesystem) is used if exactly one does; otherwise nothing is written and the log names the namespaces and the fix (fit only the drive meant for `/data`; if several carry the label, erase all but one). Probe order isn't stable across boots, so "the lowest-named" would not be a safe tie-break.
6. **Waiting, and failing fast.** Polls every 250 ms, only while a drive can still appear, with a bound per state:
   - nothing on the PCIe bus yet: up to 10 s;
   - an NVMe controller (PCI class `0x010802`) is there but no namespace yet: up to 30 s, since a controller's ready timeout can run to tens of seconds. This isn't fail-fast territory, because the controller is known to exist. When it runs out, the message differs by whether a driver has bound to the controller (`/sys/bus/pci/devices/*/driver`): no driver points at the kernel config, and bound-but-no-namespace points at the drive;
   - an enumerated bus (any PCI function) with no NVMe controller: believed after 3 s by elapsed time, not by a count of looks. The kernel adds a root port to sysfs before the device behind it, and a slow drive can still be training its link. The message says a slow drive can need another reboot.

   This assumes one PCIe host controller enumerates every slot, which is true of Tegra210 (marked `ponytail:` in the code).
7. **Safety policy.** gosd-init always calls `blockmount.Run` with `destructive=false`, so its existing rules decide everything:
   - a blank drive is formatted. "Blank" means the drive's first MiB reads as zeroes, which is what Inspect checks. A drive that reads as zeroes but won't take writes (an OPAL-locked self-encrypting drive, say) passes this and then fails at write time, so it errors rather than being mistaken for usable;
   - this label, ext4 and the establishment marker: adopted untouched. Adoption is by label alone, by design: a drive moved from another GoSD device whose app has the same label prefix is adopted as this one's `/data`;
   - this label, ext4, no marker and nothing in the root but an empty `lost+found`: interrupted-establishment debris, reformatted. A non-empty `lost+found` counts as content, because fsck moves orphaned files there; that's now true for `disk` and `emmc` too;
   - anything else (another label or filesystem, unrecognised content, this label with content but no marker, this volume unmountable): refused, untouched.

   blockmount gained `Storage.Remedy`, so a refusal ends with an operator's fix ("take it out and erase it on another computer…") instead of the app API's `destructive=true`; `disk` and `emmc` messages are unchanged.
8. **No drive, or a refused one: degraded, not fatal.** The same read-only tmpfs `/data` as a missing data partition elsewhere; the app still starts, and the log line names the fix. Not a halt: "a missing or unmountable data partition is never fatal" applies, and dataexpand's halt (`ErrDataCorrupt`) rests on an MBR commit record a whole-device volume has no equivalent of. Its `LAST_FATAL_ERROR.md` would also land on the eMMC, which nobody can read by plugging it into a computer. The fail-fast decision is met by item 6.
9. **Reflash and on-disk ABI.** A reflash rewrites the eMMC and never touches the NVMe, so the next boot finds its own marked ext4 volume and adopts it, and the config store on `/data` restores the device's settings. This is the opposite of a fixed-size SD image, whose `/data` a reflash wipes. A changed label (a renamed app or new `--label-prefix`) is where the NVMe differs from an SD card's partition 2: there the adoption gate cleanly reformats, but GoSD can't tell an old volume on a drive it didn't just flash from a stranger's, so it's refused instead. The filesystem can't change between releases (always ext4).
10. **Nothing on the eMMC but `APP` is written.** For a `dataOnNVMe` image gosd-init never tries a partition-2 candidate and never runs dataexpand, and nvmedata's rank can't select an `mmcblk` device at all. Its only eMMC writes remain the existing ones to the boot partition (`APP`: config-tree edits, crash reports). An app's own `disk`/`emmc` calls see `mmcblk0` as in use (`APP` is mounted) and the NVMe as in use (`/data`). Not covered, and out of scope: an app deliberately handing one of NVIDIA's partitions (e.g. `/dev/mmcblk0p2`) to `gadget.MassStorage`, since devreserve reserves only `APP`. Worth a follow-up if gadget mode is ever proven on this carrier.

## Crash-ordering argument

All the writing is `internal/blockmount`'s establish sequence. That package's doc carries the argument filesystem by filesystem; this is its ext4 case as it applies here.

1. **Format** writes GoSD's 512 MiB ext4 golden image **superblock last** (`internal/diskfmt`'s `writeEXT4`, changed by this bean for every caller):
   1. zero the first MiB, flush;
   2. write everything after it, flush;
   3. write the first MiB (superblock, label, group descriptors), flush.

   Until step 3 lands, the drive's first MiB reads as zeroes, which Inspect reports as Blank. Erasing first is what makes that true of a reformat too, not just of a drive that started blank.
2. **SyncDevice** fsyncs the device node. When it returns, the whole format is on the medium.
3. **Mount** interprets only what 1 and 2 made durable.
4. **Grow** (`EXT4_IOC_RESIZE_FS`) fills the drive, then `syncfs`. When it returns, the grow is durable.
5. **EstablishMarker** creates `.gosd-established`, fsyncs it, then fsyncs the root directory. This is the commit record, written only after 2 and 4 have returned.

A later boot adopts only on label + filesystem + marker. A probe (signature, label, a successful mount) is never enough. An interruption before step 5 leaves one of these:

- **Anywhere before the superblock write lands:** a first MiB of zeroes, so Blank, so formatted again freely. No new on-disk record is needed, and this is the case that matters on the very first boot.
- **Part-way through that one superblock write:** possibly a superblock that won't mount, refused as `ErrUnmountable`, with `/data` read-only until someone erases the drive. GoSD can't tell it from its own damaged volume still holding data, so it refuses. The window is a single 1 MiB write and its flush, down from the whole 512 MiB stream before this bean.
- **After the format, before the marker (mid-grow or just after):** our label, a mountable ext4, no marker, only an empty `lost+found` in the root. The app never ran against it, because gosd-init only starts `/app` after `mountData` returns. So it's unmounted and the whole sequence is redone, grow included.

Two things about failures this run notices:

- If Grow or EstablishMarker fails after the mount, establish now unmounts the half-established volume before returning (best effort, with an unmount failure appended to the error). So nothing can write into it under the read-only `/data` fallback.
- After step 5 the volume is established: `destructive=false` plus the marker means no later boot ever formats it. If the marker is removed and the root holds content (including a non-empty `lost+found`), ext4 refuses rather than adopts.

Two things this layout lacks, and why they're acceptable:

- **No unforgeable commit record,** like dataexpand's MBR entry. The volume has no partition table, so the marker lives inside the filesystem it gates, exactly as for `disk`.
- **Reflash is not a crash risk.** A reflash writes only the eMMC and so can't interrupt anything on the NVMe.

After the mount, gosd-init's own writes to `/data` (`.gosd-data`, the config store, the boot counter) are unchanged, with their existing durability arguments. App data durability stays the app's, per the fsync sequence in docs/runtime.md; the journal buys metadata consistency only.

## Summary of Changes

- `internal/boards`: `NVMeDataBoard` capability and `DataOnNVMe` helper; jetson-nano implements it.
- `internal/initcfg`: `Config.DataOnNVMe` (`dataOnNVMe`).
- `internal/pipeline`: for such a board, no data partition, `dataFilesystem: ext4`, no expand or flush, and `dataOnNVMe` baked.
- `cmd/gosd`: one line per such board saying the data flags don't apply. Integration test `TestBuildForJetsonNanoPutsDataOnNVMe` builds with `--data-size`/`--data-flush`, reads the image back and asserts no partition 2 and the baked fields, with the network tripwire.
- `cmd/gosd-init/internal/nvmedata` (new): drive discovery (settle, then no guessing between several), the bounded waits (3 s for no controller, 10 s for no bus, 30 s for a controller's namespace, with distinct messages for unbound and no-namespace), and `blockmount.Run` with `destructive=false`. It has platform_linux/platform_other seams and fake-driven tests covering every adopt/format/refuse branch.
- `cmd/gosd-init/internal/boot`: `Deps.MountNVMeData`, nil-checked; for a `dataOnNVMe` image, `Run` skips dataexpand and the partition-2 candidates, and falls back to the read-only `/data` on failure. `mountData` now takes the mount to try.
- `internal/blockmount`: `Storage.Remedy` replaces the `destructive=true` advice for a caller with no such option. A volume whose grow or marker write fails is unmounted again. `RootHasOtherContent` counts a non-empty `lost+found` as content, and moved to a portable file so it's tested on macOS too.
- `internal/diskfmt`: the ext4 golden is written superblock last (erase head, flush, body, flush, head, flush), so an interrupted format reads as blank. This applies to `disk`, `emmc` and dataexpand as well.
- Docs: the Jetson Nano development notes (where `/data` lives, the expected console lines), the flashing guide's status note and prerequisites, the runtime contract's `/data` section, the config tree's reflash section, and the CLAUDE.md board entry.
