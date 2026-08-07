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
  ext4, not FAT32** (epic `gosd-lfu0`). `Options.Filesystem`'s zero value —
  and so every existing call to `disk.FormatAndMount`, which always used the
  zero value — now formats a blank disk as ext4 instead of FAT32. ext4 is
  journaled and crash-safe, which FAT32 never was, and internal drives (the
  only thing `disk` addresses — never removable media meant to be read on
  another host) are exactly where that matters.
  - **If your app's disk needs to stay FAT32 or exFAT** — most commonly
    because it's meant to be read on another host, or shared over USB via
    `gadget.MassStorage` to a computer expecting a FAT-family filesystem —
    pass `disk.Options{Filesystem: disk.FAT32}` (or `disk.ExFAT`) explicitly.
  - **A disk that already carries an established FAT32/exFAT volume under
    your app's label is unaffected**: it's mounted, not reformatted, on
    upgrade. Only a *blank* disk, or an explicit `Destructive: true`
    reformat, picks up the new ext4 default.
  - **ext4 needs `CONFIG_EXT4_FS` in the board's kernel.** Every
    Rockchip-family board (Radxa Zero 3E, NanoPi Zero2, ROCK 4SE, and the
    internal-only Radxa Cubie A5E) has it; the Raspberry Pi family does not,
    so asking for the default there — including implicitly, via the
    zero-value `Options{}` — fails with `disk.ErrUnsupportedFS`, naming the
    gap, before anything is touched. See `COMPATIBILITY.md`'s "ext4 on
    attached disks" row.
  - **`emmc/` is unaffected**: it stays FAT32-only by design, unconditionally
    — this change is `disk`-only.
  - Formatting writes a checked-in golden ext4 image and grows it online to
    the disk's real size exactly once, at first establishment; re-mounting
    an established volume adopts it rather than reformatting or re-growing
    it. The ext4 journal buys metadata crash-consistency and mount-time
    replay, **not** data durability — the four-step fsync pattern
    (`docs/runtime.md`'s "Making a write durable") remains the app-facing
    contract regardless of filesystem. See `internal/blockmount`'s package
    doc for the full format/grow/adopt mechanism and crash-ordering
    argument.
  - Shipped as a **minor** version bump: a breaking default, not a breaking
    API signature.
