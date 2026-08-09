# Unreleased

Release-notes-level call-outs — breaking changes above all — accumulate here
between CLI `vX.Y.Z` tags. At each release they fold into the tag's notes
(`gh release create --notes-file`, edited as needed) and this file resets to
this stub. Last folded into: v0.1.0 (2026-08-08).

## Breaking changes

- **FAT volume labels are now per-app, not the fixed `GOSD-BOOT`/`GOSD-DATA`**
  (bean `gosd-lo7k`). Every image now ships `<prefix>-boot` and
  `<prefix>-data` labels, where `<prefix>` defaults to the app's own name —
  sanitized and truncated to 6 bytes — so an app called `hello` now shows up
  as a drive named `hello-boot` (and `hello-data`, if `/data` is FAT32) when
  the card is plugged into a computer, instead of the generic `GOSD-BOOT`.
  Override the prefix with the new `gosd build --label-prefix` /
  `gosd run --label-prefix` flag; an explicit prefix is used verbatim.
  - **Clean break, no migration.** The label pair is part of the app's
    on-card ABI, exactly like `--boot-size` and `--data-filesystem`: a card
    already flashed by a pre-`gosd-lo7k` release carries the old
    `GOSD-DATA` label, which no image built after this change will ever
    recognize as its own. That card's **first reflash-upgrade with a
    rebuilt image cleanly reformats its data partition** — the device does
    not halt, and the boot partition is unaffected either way. There is no
    adoption alias for the old label.
  - **Anything that keyed on the literal `GOSD-DATA` label must update** —
    for example a udev rule, a host-side backup script, or tooling that
    mounts the card by label. Match on the new per-app label instead (or
    mount by partition/device node, which never changed).
  - **Cross-app reflash no longer silently inherits the previous app's
    data.** Every app previously shared one `GOSD-DATA` label, so flashing
    app B's image onto a card that last ran app A would re-adopt app A's
    data as if it were app B's. Labels are per-app now, so that no longer
    happens — unless two apps happen to share the same 6-byte default (or
    explicit) prefix, which still re-adopts across them, exactly like
    today's universal label did, just narrower.
  - Runtime label matching is case-insensitive, so a host tool that
    displays or rewrites a label uppercased can't trigger a spurious
    reformat.
