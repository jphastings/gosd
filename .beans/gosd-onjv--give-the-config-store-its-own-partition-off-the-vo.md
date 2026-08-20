---
# gosd-onjv
title: Give the config store its own partition, off the volume apps share
status: todo
type: feature
priority: high
created_at: 2026-08-20T09:39:04Z
updated_at: 2026-08-20T09:39:04Z
---

Move the config store off `/data` and onto a partition of its own, so an app
sharing its data volume cannot expose the device's settings.

## Why (JP, 2026-08-20)

`gosd-7m9y` closed as **accepted risk**: the store cannot be authenticated on
these boards, and a reflash must keep every setting, credentials included,
because surviving a reflash is the store's whole purpose. That decision stands
and this bean does not revisit it.

What this bean fixes is a different and more frequently realised problem:
**the store lives on the volume apps publish.** In one review batch that cost
us two findings, both in `gosd-cayj` / PR #342:

- `examples/usbwebsite` served `http.FileServer(http.Dir("/data"))`, and
  `http.Dir` performs no dotfile filtering — `GET
  /.gosd/config/values/wifi/passphrase` returned the passphrase over plain
  HTTP, to anyone who could reach port 80. No cable, no physical access.
- The same example shared `/data` over USB mass storage, where **a LUN is the
  whole volume** — so consenting to share a website also shared the tailnet
  node's private key, the cloudflared token and the Funnel authkey.

Both were fixed by scoping what that one example publishes. Neither is fixed
*structurally*: the next app to serve or share `/data` re-creates them, and
the failure is silent. Isolation is the structural answer, and it protects
every setting rather than the two the rejected credential carve-out covered.

**What this does NOT do,** stated so nobody records it as more than it is: a
compromised `/app` runs as root and can mount any partition, and someone
holding the card can write a third partition exactly as easily as the second.
This is an accidental-exposure fix, not an attack-surface reduction, and
`gosd-7m9y`'s residual risk is unchanged by it.

## Locked decisions

- **Layout: `p1` boot (FAT32) → `p2` config (ext4) → `p3` data.** Config must
  sit BEFORE data: `--data-size=expand` fills the card to its end, so nothing
  can follow the data partition.
- **32 MiB by default**, overridable at build time (`--config-size`, matching
  `--boot-size`'s shape). It holds kilobytes; the size is for headroom and
  alignment, not capacity.
- **ext4, from a NEW golden** — see the golden section below. The existing
  512 MiB golden cannot be used: it is the only ext4 GoSD can produce today
  and 512 MiB is its floor.
- **Label `<prefix>-conf`**, following the per-app label rule (`gosd-lo7k`).
  With the prefix truncated to 6 bytes this is at most 11 characters.
- **The partition is gosd-init's, never the app's.** It must not appear as a
  candidate anywhere an app can reach it — check `disk.Devices`,
  `disk.rank`/`Usable` and `FormatAndMountDevice`, and `emmc.chooseEMMC`. An
  app that enumerates storage must not be able to pick it up and publish it
  as a mass-storage LUN, which is the exact failure this bean exists to
  prevent. Relates to `gosd-ix0r` (gosd-init publishing which disk it booted
  from).
- **On-card ABI.** Like `--boot-size`, `--data-filesystem` and
  `--label-prefix`, `--config-size` is part of the app's on-card ABI:
  changing it between releases fails the adoption gate and cleanly reformats,
  never halts.

## The new golden

`internal/diskfmt/ext4golden` ships a 512 MiB image with a 128 MiB journal,
sized that way because **the journal can never be resized after format** and
so must serve the grown filesystem's whole life. A 32 MiB partition needs its
own:

- Build it with the existing parameterised recipe (`build/ext4-golden/build.sh`)
  and give it its own provenance `manifest.json`, same discipline as the
  original — pinned e2fsprogs tag and commit, every parameter's WHY in the
  README rather than the manifest.
- **Journal at the ext4 minimum** (1024 blocks = 4 MiB at the 4096-byte block
  size the existing golden uses and every board's page size matches). Note
  that 4 MiB of a 32 MiB volume is journal; that is expected, and is why 32
  MiB rather than something smaller.
- **It needs none of the growth machinery.** The data golden ships
  `^resize_inode,meta_bg` specifically to escape `resize_inode`'s 8 TiB cap
  (see that README's argument, and `gosd-2ssb`). A fixed-size config
  partition never grows, so keep the feature set plain and standard — the
  more ordinary the filesystem, the better any host's `e2fsck` can repair it.
- If `--config-size` exceeds the golden, grow once at establishment exactly
  as the data path does (`EXT4_IOC_RESIZE_FS`, `internal/blockmount`); at the
  default size there is no resize at all, which is the simpler and commoner
  path.

## Consequences that need deciding or announcing

- **Every board now needs `CONFIG_EXT4_FS`, unconditionally.** Today ext4 is
  opt-in per image (`--data-filesystem ext4`, refused per-board via
  `boards.EXT4Support`). A config partition every image carries turns that
  into a hard requirement of board support itself. No board GoSD currently
  ships is affected (`gosd-ssth` corrected the belief that the Pis lacked
  it), but `boards.EXT4Support` changes meaning and a future board without it
  could not run GoSD at all. Decide whether to keep the per-board check as a
  build-time refusal or promote it to a board-registration assertion.
- **Kept settings are lost once, on the upgrade to this layout.** Inserting a
  partition ahead of the data partition moves that partition's start offset,
  so the existing store is not at a readable location afterwards — a
  migration that reads the old store cannot generally run, because by the
  time the new image boots, the bytes are not where its own MBR says
  anything is. Release-notes-level; say it plainly rather than implying
  settings carry over.
- **`gosd-df24` gets better and stays necessary.** Better: a reset can wipe
  the config partition and leave app data intact, which is what people
  actually mean by "reset my settings". Still necessary: an ext4 config
  partition is no more clearable from macOS than an ext4 `/data`, so the
  boot-partition trigger remains the only mechanism an owner can operate.
- Establishment follows `internal/blockmount`'s existing discipline — write →
  sync → marker → sync, adoption gated on the marker, never on a probe. Write
  the crash-ordering argument for the config partition explicitly; it is a
  third caller of that machinery, not a copy of it.

## Todo

- [ ] New 32 MiB golden + provenance manifest + README rationale
- [ ] `--config-size` build flag; three-partition layout in `internal/image`
- [ ] Config partition establishment/adoption via `internal/blockmount`
- [ ] Point `cmd/gosd-init/internal/configstore` at the new mount
- [ ] Exclude it from every app-reachable storage enumeration (`disk`, `emmc`)
- [ ] Decide the `boards.EXT4Support` question above
- [ ] `dataexpand`: derive offsets with a third partition present
- [ ] Change file naming the one-time settings loss and the layout ABI break
- [ ] Update `gosd-df24` to target the config partition
- [ ] Docs: the config tree guide, and the upgrade-path design doc
