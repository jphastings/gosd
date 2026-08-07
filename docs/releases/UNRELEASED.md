# Unreleased

No `gosd` CLI `vX.Y.Z` tag has been cut yet (see `gh release list` — only
`artifacts/vX.Y.Z` releases exist so far, a separate versioning track for
compiled kernels/U-Boot, not the CLI itself). This file accumulates
release-notes-level call-outs — breaking changes above all — until JP cuts
the first CLI tag, at which point its contents fold into that release's
notes (`gh release create`'s `--notes`, or GitHub's generated notes edited
by hand) and this file is cleared back to an empty "Unreleased" section.

## Breaking changes

- **`disk.FormatAndMount`/`FormatAndMountWith`'s default filesystem is now
  ext4, not FAT32** (epic `gosd-lfu0`). **`emmc.FormatAndMount`/
  `FormatAndMountWith`'s default filesystem is now ext4 too, not FAT32**
  (bean `gosd-9sc4`, mirroring `disk`'s flip exactly, shipping in this same
  release). Each package's `Options.Filesystem` zero value — and so every
  existing call to `disk.FormatAndMount`/`emmc.FormatAndMount`, which always
  used the zero value — now formats a blank disk/eMMC as ext4 instead of
  FAT32. ext4 is journaled and crash-safe, which FAT32 never was, and
  internal drives (the only thing `disk`/`emmc` address — never removable
  media meant to be read on another host) are exactly where that matters.
  - **If your app's disk or eMMC needs to stay FAT32 or exFAT** — most
    commonly because it's meant to be read on another host, or shared over
    USB via `gadget.MassStorage` to a computer expecting a FAT-family
    filesystem — pass `disk.Options{Filesystem: disk.FAT32}` (or
    `emmc.Options{Filesystem: emmc.FAT32}`, or `ExFAT`) explicitly.
  - **A disk or eMMC that already carries an established FAT32/exFAT volume
    under your app's label is unaffected**: it's mounted, not reformatted,
    on upgrade. Only a *blank* disk/eMMC, or an explicit
    `Destructive: true` reformat, picks up the new ext4 default. An
    established FAT32 eMMC volume plus the new zero-value (ext4) request
    refuses without `Destructive: true`, naming both filesystems and the
    flag in the error — exactly like `disk`'s existing fs-mismatch rule.
  - **ext4 needs `CONFIG_EXT4_FS` in the board's kernel.** Every
    Rockchip-family board (Radxa Zero 3E, NanoPi Zero2, ROCK 4SE, and the
    internal-only Radxa Cubie A5E) has it; the Raspberry Pi family does not
    (and has no onboard eMMC at all, so `emmc`'s default never encounters
    the gap in practice). Asking for the default on a board that lacks it —
    including implicitly, via the zero-value `Options{}` — fails with
    `disk.ErrUnsupportedFS`/`emmc.ErrUnsupportedFS`, naming the gap, before
    anything is touched. See `COMPATIBILITY.md`'s "ext4 on attached disks"
    and "ext4 on the eMMC" rows.
  - Formatting writes a checked-in golden ext4 image and grows it online to
    the disk's/eMMC's real size exactly once, at first establishment;
    re-mounting an established volume adopts it rather than reformatting or
    re-growing it. The ext4 journal buys metadata crash-consistency and
    mount-time replay, **not** data durability — the four-step fsync
    pattern (`docs/runtime.md`'s "Making a write durable") remains the
    app-facing contract regardless of filesystem. See
    `internal/blockmount`'s package doc for the full format/grow/adopt
    mechanism and crash-ordering argument, shared identically by both
    packages.
  - **What did not change for `emmc`**: candidate selection stays a single
    onboard device with no `disk`-style multi-class ranking or named-device
    equivalent — this bean only changed which filesystem gets written, not
    which device gets chosen.
  - Shipped as a **minor** version bump: a breaking default, not a breaking
    API signature.
