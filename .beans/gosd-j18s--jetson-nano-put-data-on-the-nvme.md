---
# gosd-j18s
title: 'Jetson Nano: put /data on the NVMe'
status: in-progress
type: feature
priority: normal
created_at: 2026-11-07T20:59:10Z
updated_at: 2026-11-07T21:53:18Z
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
- [ ] Independent adversarial review pass, before JP's review (CLAUDE.md: this formats and adopts on-disk state).
- [ ] Bench: blank drive → formatted; reboot → adopted; reflash → adopted, settings restored; drive holding something else → refused; drive removed → read-only `/data` at once. Expected console lines are in the Jetson Nano development notes.

## Design

1. **Declaration.** An optional capability, `boards.NVMeDataBoard` (`DataOnNVMe() bool`, helper `boards.DataOnNVMe`), shaped like `InitramfsLimiter`; jetson-nano implements it. The pipeline bakes it as `initcfg.Config.DataOnNVMe` (`dataOnNVMe` in config.json), and gosd-init acts on that bit alone: no board-ID check anywhere at runtime.
2. **What the build does with the data flags.** For such a board the pipeline zeroes `--data-size` (no partition 2 in the image) and `--data-size=expand`, drops `--data-flush` and bakes `dataFilesystem: ext4`; `gosd build` prints one line per such board saying so. Ignored rather than refused, because a bare `gosd build` builds every public board and these flags are legitimate for the others, and because the flashed image only ever contributes its boot partition to `APP` anyway. Known wrinkle, left alone: `--ingress tailscale-funnel`'s fleet-wide "needs `--data-size`" check still fires for a jetson-nano-only build; passing one is harmless since it's then ignored.
3. **Filesystem: always ext4, whatever `--data-filesystem` says.** FAT32's default exists so a card reads in any computer's SD reader, which a drive inside a carrier board isn't; GoSD's FAT32 formatter refuses volumes over 256 GiB, so the default would fail on the bench's 477 GB drive and on most NVMe drives; the `disk` package, the same machinery, defaults to ext4 for internal drives for the same reasons (epic gosd-lfu0); and ext4's journal matters on a device with no clean shutdown. gosd-init honours whatever config.json bakes rather than hard-coding ext4, so the choice lives in one place (the pipeline).
4. **Label and layout.** `<prefix>-data` (config.json's `dataLabel`), as a whole-device volume with no partition table: blockmount's `disk` shape, unchanged.
5. **Which device.** `cmd/gosd-init/internal/nvmedata` ranks only `/sys/block` names matching `^nvme\d+n\d+$` (namespaces; not partitions, not multipath paths), through `blockmount.Candidates`, so in-use, empty and read-only devices are excluded as for `disk`. Several namespaces: the lowest-named (`nvme0n1`, a drive's first namespace) is used and the others are logged. A Jetson Nano's carrier has one M.2 slot; a board with two NVMe controllers would need to choose by something stable across boots, since probe order isn't (marked `ponytail:` in the code).
6. **Waiting, and failing fast.** Polls every 250 ms, up to 10 s, only while a drive can still appear: the PCIe bus hasn't enumerated yet, or an NVMe controller is there whose namespace scan (asynchronous) hasn't finished. Once the bus has enumerated (any PCI function in sysfs) with no NVMe-class function (`0x010802`) on two looks in a row, it fails at once. Two looks, because the kernel adds an enumerated bus's devices to sysfs one at a time. This assumes one PCIe host controller enumerates every slot, which is true of Tegra210 (also marked `ponytail:`).
7. **Safety policy.** gosd-init always calls `blockmount.Run` with `destructive=false`, so its existing rules decide everything:
   - a blank drive (first MiB all zeroes) is formatted;
   - this label, ext4 and the establishment marker: adopted untouched;
   - this label, ext4, no marker and nothing in the root but `lost+found`: interrupted-establishment debris, reformatted;
   - anything else (another label or filesystem, unrecognised content, this label with content but no marker, this volume unmountable): refused, untouched.

   blockmount gained `Storage.Remedy`, so a refusal ends with an operator's fix ("take it out and erase it on another computer…") instead of the app API's `destructive=true`; `disk` and `emmc` messages are unchanged.
8. **No drive, or a refused one: degraded, not fatal.** The same read-only tmpfs `/data` as a missing data partition elsewhere; the app still starts, and the log line names the fix. Not a halt: "a missing or unmountable data partition is never fatal" applies, and dataexpand's halt (`ErrDataCorrupt`) rests on an MBR commit record a whole-device volume has no equivalent of. Its `LAST_FATAL_ERROR.md` would also land on the eMMC, which nobody can read by plugging it into a computer. The fail-fast decision is met by item 6.
9. **Reflash and on-disk ABI.** A reflash rewrites the eMMC and never touches the NVMe, so the next boot finds its own marked ext4 volume and adopts it, and the config store on `/data` restores the device's settings. This is the opposite of a fixed-size SD image, whose `/data` a reflash wipes. A changed label (a renamed app or new `--label-prefix`) is where the NVMe differs from an SD card's partition 2: there the adoption gate cleanly reformats, but GoSD can't tell an old volume on a drive it didn't just flash from a stranger's, so it's refused instead. The filesystem can't change between releases (always ext4).
10. **Nothing on the eMMC but `APP` is written.** For a `dataOnNVMe` image gosd-init never tries a partition-2 candidate and never runs dataexpand, and nvmedata's rank can't select an `mmcblk` device at all. Its only eMMC writes remain the existing ones to the boot partition (`APP`: config-tree edits, crash reports). An app's own `disk`/`emmc` calls see `mmcblk0` as in use (`APP` is mounted) and the NVMe as in use (`/data`). Not covered, and out of scope: an app deliberately handing one of NVIDIA's partitions (e.g. `/dev/mmcblk0p2`) to `gadget.MassStorage`, since devreserve reserves only `APP`. Worth a follow-up if gadget mode is ever proven on this carrier.

## Crash-ordering argument

All the writing is `internal/blockmount`'s establish sequence. That package's doc carries the argument filesystem by filesystem; this is its ext4 case as it applies here.

1. **Format** streams GoSD's 512 MiB ext4 golden onto `/dev/nvme0n1`. Nothing is durable yet, in any order.
2. **SyncDevice** fsyncs the device node. When it returns, the whole format is on the medium.
3. **Mount** interprets only what step 2 made durable.
4. **Grow** (`EXT4_IOC_RESIZE_FS`) fills the drive, then `syncfs`. When it returns, the grow is durable.
5. **EstablishMarker** creates `.gosd-established`, fsyncs it, then fsyncs the root directory. This is the commit record, written only after 2 and 4 have returned.

A later boot adopts only on label + filesystem + marker. A probe (signature, label, a successful mount) is never enough. An interruption before step 5 leaves one of these:

- **Nothing identifiable:** `Blank`, formatted again freely.
- **Our label, no marker, only `lost+found` in the root:** the app never ran against it, because gosd-init only starts `/app` after `mountData` returns. So it's unmounted and the whole sequence is redone, grow included.
- **Our label, but it won't mount (torn):** refused as `ErrUnmountable`, and `/data` is read-only until someone erases the drive. This is the one interrupted state that needs a human. It's accepted because GoSD can't tell it from its own damaged volume still holding data, and the window is the few seconds the golden takes to stream on the very first boot.
- **A label that isn't ours,** because the label landed late: refused. This is blockmount's documented limit: it doesn't wipe volumes it can't show are its own.

After step 5 the volume is established: `destructive=false` plus the marker means no later boot ever formats it. If the marker is removed and the root holds content, ext4 refuses rather than adopts (blockmount's `RootHasOtherContent` rule).

Two things this layout lacks, and why they're acceptable:

- **No unforgeable commit record,** like dataexpand's MBR entry. The volume has no partition table, so the marker lives inside the filesystem it gates, exactly as for `disk`.
- **Reflash is not a crash risk.** A reflash writes only the eMMC and so can't interrupt anything on the NVMe.

After the mount, gosd-init's own writes to `/data` (`.gosd-data`, the config store, the boot counter) are unchanged, with their existing durability arguments. App data durability stays the app's, per the fsync sequence in docs/runtime.md; the journal buys metadata consistency only.

## Summary of Changes

- `internal/boards`: `NVMeDataBoard` capability and `DataOnNVMe` helper; jetson-nano implements it.
- `internal/initcfg`: `Config.DataOnNVMe` (`dataOnNVMe`).
- `internal/pipeline`: for such a board, no data partition, `dataFilesystem: ext4`, no expand or flush, and `dataOnNVMe` baked.
- `cmd/gosd`: one line per such board saying the data flags don't apply. Integration test `TestBuildForJetsonNanoPutsDataOnNVMe` builds with `--data-size`/`--data-flush`, reads the image back and asserts no partition 2 and the baked fields, with the network tripwire.
- `cmd/gosd-init/internal/nvmedata` (new): drive discovery, the bounded fail-fast wait, and `blockmount.Run` with `destructive=false`, with platform_linux/platform_other seams and fake-driven tests.
- `cmd/gosd-init/internal/boot`: `Deps.MountNVMeData`; for a `dataOnNVMe` image, `Run` skips dataexpand and the partition-2 candidates, and falls back to the read-only `/data` on failure. `mountData` now takes the mount to try.
- `internal/blockmount`: `Storage.Remedy` replaces the `destructive=true` advice for a caller with no such option.
- Docs: the Jetson Nano development notes (where `/data` lives, the expected console lines), the flashing guide's status note and prerequisites, the runtime contract's `/data` section, the config tree's reflash section, and the CLAUDE.md board entry.
