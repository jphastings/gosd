---
# gosd-ucgr
title: 'ext4 ship: qemu CI smoke, docs, COMPATIBILITY, CLAUDE.md, minor version'
status: in-progress
type: task
priority: normal
created_at: 2026-08-07T09:58:20Z
updated_at: 2026-08-07T14:45:04Z
parent: gosd-lfu0
blocked_by:
    - gosd-1c0x
---

Shipping bean for epic gosd-lfu0. A qemu-virt CI job (or an extension of the existing qemu boot tests) formats+grows an ext4 virtio disk through the real disk/ path and reboots to prove adoption; docs and locked decisions catch up; the release notes carry the breaking-default note and the minor version bump.

## Todos

- [x] qemu-virt smoke: disk.FormatAndMountWith(ext4) on a second virtio disk → write file → fsync pattern → hard kill → reboot → journal replay → file present, volume adopted not reformatted
- [x] COMPATIBILITY.md: ext4 rows per board (Rockchip fleet + qemu-virt yes; Pi boards no with the kernel-config reason)
- [x] docs: disk package docs + docs/runtime.md cross-link (journal ≠ data durability; fsync pattern still the story)
- [x] CLAUDE.md: update the Public API locked decision (disk fs tokens, ext4 default, emmc unchanged) + this epic's outcome
- [x] Release notes text for the minor bump (breaking default called out); JP tags the release
- [x] Bench follow-up filed for rock-4se NVMe real-hardware verification (power-cut test rig) if not already covered by a bench bean (bean gosd-vv5o, parented under gosd-lfu0, not blocked — needs hardware, not code)

## Summary of Changes

**New CI job `qemu-disk-ext4`** (`.github/workflows/ci.yml`, mirrors
`qemu-expand-data`'s shape): builds `examples/diskstorage` for
`--board=qemu-virt` (real artifacts), attaches a second, blank 2GiB sparse
virtio disk via `QEMU_EXTRA_ARGS` (unused in CI until now — the escape hatch
`internal/qemurun.ParseExtraArgsEnv` was already built and tested for), boots
once (`scripts/boot-and-grep.sh`, unmodified — its default hard-kill-via-
`pkill` behavior already IS the power-cut simulation qemu-expand-data relies
on), asserts the app's log shows the volume ready + `boots=1`, then a new
`scripts/assert-disk-grew.sh` greps the logged filesystem size and asserts
it's within a wide, unambiguous band of the 2GiB disk (well clear of ext4golden's
fixed 512MiB image — proving `EXT4_IOC_RESIZE_FS` actually ran). qemu is then
hard-killed and rebooted against the *same* two disk files; a second
`boot-and-grep.sh`/`assert-disk-grew.sh` pair asserts `boots=2` (proof of
adoption, not reformat — a reformat would reset the counter to 1, mirroring
`examples/hello`'s existing `boots=N` idiom) and that the grown size persisted
through the crash+adopt cycle.

**New example `examples/diskstorage`**: calls
`disk.FormatAndMountWith(label, mountpoint, disk.Options{})` — the zero
value, proving the ext4 *default*, not an explicit `disk.EXT4` token —
durably writes a boot counter with docs/runtime.md's four-step fsync
pattern, logs the mounted filesystem's `statfs` size, and serves HTTP for
the harness's readiness poll (same shape as `examples/hello`). Degrades
gracefully on `disk.ErrNoDisk`, like `examples/emmcstorage` does for
`ErrNoEMMC`. Added to both cross-compile smoke-build lists (arm64, armv6).

**Bug found and fixed by the smoke test itself**: the first real two-boot
run failed adoption outright —
`internal/diskfmt`'s `inspectEXT4`/`parseEXT4Superblock` refused to read
*any* ext4 superblock carrying the kernel's `INCOMPAT_RECOVER` bit (0x0004,
"journal needs replay"), treating it as an unrecognised feature. That bit is
not a format-shape feature — it's a transient flag the kernel sets whenever
it opens the journal and clears only on a *clean* unmount, and GoSD boards
never cleanly unmount (`gosd-init` has no shutdown path). So this wasn't a
narrow crash-only edge case: it would have refused adoption after
essentially every real-world reboot, not just a hard power cut. Fixed by
adding `ext4IncompatRecover` to `ext4KnownIncompat`
(`internal/diskfmt/ext4.go`), with the reasoning recorded in its doc comment:
every field `Inspect` reads (feature bits, label, UUID) is written with a
direct fsync/syncfs by Format/Grow/EstablishMarker, never left pending in the
journal, so a pending replay doesn't make any of it untrustworthy — the
replay itself happens inside the kernel's own `Mount` call that follows
Inspect, before anything reads file data. New unit test
`TestInspectToleratesRecoverFlag` (`internal/diskfmt/ext4_test.go`) pins it;
`TestInspectRefusesUnknownIncompatFeatures` still covers a genuinely unknown
bit. Re-ran the full local two-boot cycle after the fix: adoption succeeds,
`[...] EXT4-fs (vdb): recovery complete` appears in the kernel log before
gosd disk's own "ready"/`boots=2` lines, confirming the journal replay this
bean is supposed to prove.

**COMPATIBILITY.md**: new "ext4 on attached disks (default)" row (❌ on all
three Pi boards, ✅ on Radxa Zero 3E/NanoPi Zero2/ROCK 4SE) plus a `[^ext4]`
footnote covering the golden-image/grow/adopt mechanism, the per-board kernel
truth (including the internal `qemu-virt` profile and the internal-only
Radxa Cubie A5E, neither of which gets its own table column, matching the
table's existing convention for internal-only boards), and a pointer to
`gosd-vv5o` for hardware verification. `[^disk]`'s footnote and the "Attached
disk format/mount" row's prose updated for the ext4-default reality (was
still describing the pre-`gosd-1c0x` FAT32 default).

**docs/runtime.md**: "Attached disk storage" section rewritten — ext4 is now
`FormatAndMount`'s default; a new "ext4 by default, or FAT32/exFAT for
removable media" subsection replaces "FAT32 or exFAT", covering the
golden-image/grow/adopt mechanism, the journal-is-not-durability distinction
(the four-step fsync pattern from "Making a write durable" remains the
contract), per-filesystem label caps (ext4 16 bytes vs FAT-family 11), the
`ErrUnsupportedFS` preflight for both `CONFIG_EXT4_FS` and `CONFIG_EXFAT_FS`,
and the foreign-filesystem-under-a-matching-label refusal rule. Also points
at `examples/diskstorage` as the worked example (previously "there is no
disk-specific example yet"), and fixes a stale "formats an SSD or USB drive
as exFAT" line in "How big the data partition can be".

**CLAUDE.md**: Public API locked decision updated (disk's typed `Filesystem`
token, ext4 zero-value default, one-liner pointing at
`internal/blockmount`'s package doc, emmc explicitly unaffected); new
dedicated locked-decision bullet recording the epic's outcome (mechanism,
crash-ordering discipline, minor-version-bump framing, CI proof, bench
follow-up), following the same pattern as the vfat-flush and Layout-ABI
bullets above it.

**Release notes**: no CLI `vX.Y.Z` tag has ever been cut in this repo (`gh
release list` shows only `artifacts/vX.Y.Z` releases, a separate
kernel/U-Boot versioning track) and there's no CHANGELOG or
`docs/releases/`-equivalent convention to append to, so per the task's
fallback instruction: added `docs/releases/UNRELEASED.md`, a
Keep-a-Changelog-style pending-notes file, with the full breaking-default
callout (what changed, the explicit-token escape hatch, the
already-established-volumes-are-unaffected reassurance, the per-board kernel
caveat, emmc's non-involvement, and the minor-bump framing). JP folds this
into the real release notes when the first CLI tag is cut.

**Bench follow-up**: filed `gosd-vv5o` ("rock-4se NVMe ext4 bench
verification — power-cut rig"), parented under `gosd-lfu0`, status `todo`,
not blocked on anything (needs the bench, not code) — format/grow/adopt on a
real NVMe SSD, plus a physical power-cut during sustained writes via the
sdwire bench skill, closing the loop this bean's qemu proof can't reach.

**Local validation** (this bean's task step 7): the existing qemu harness
(`scripts/boot-and-grep.sh`, `scripts/qemu-run.sh`) already supports a local
run via Homebrew's `qemu-system-aarch64`, so the whole two-boot cycle was run
by hand on this dev machine (`--board=qemu-virt`, real downloaded artifacts,
`QEMU_EXTRA_ARGS` attaching a 2GiB sparse second disk) — the run that found
the RECOVER bug above, and a second, clean run after the fix that passed
both boots and both `assert-disk-grew.sh` checks. (One local-only wrinkle,
not a code issue: this dev machine already has an unrelated `ipfs` daemon
listening on :8080, so boot-and-grep.sh's default HTTP-readiness poll
false-positives against *that* instead of qemu; worked around for the local
runs only by setting `BOOT_WAIT_FOR` to a log string, which CI's fresh
runners won't need.)

Quality gates (`go test ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run ./...` + `GOOS=linux golangci-lint run ./...`) run next,
foreground, before pushing.

PR: https://github.com/jphastings/gosd/pull/194

Quality-gates follow-up: all five ran and passed before the push above (the go test ./... paragraph a few lines up was written mid-run; it's now confirmed clean, including a second full-suite run after an earlier flaked cmd/gosd timeout that was resource contention, not a real failure — reproduced clean in isolation at 175s).
