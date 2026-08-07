---
# gosd-apmv
title: 'diskfmt: ext4 inspect + format-by-golden-copy (pure Go)'
status: in-progress
type: task
priority: normal
created_at: 2026-08-07T09:57:52Z
updated_at: 2026-08-07T11:12:52Z
parent: gosd-lfu0
blocked_by:
    - gosd-u988
---

internal/diskfmt grows ext4 support alongside FAT32/exFAT for epic gosd-lfu0: Inspect (recognize an ext4 superblock, read its 16-byte volume label + UUID) and Format (stream the decompressed golden image from gosd-lfu0's checked-in asset to the target, then set label + fresh random UUID in the superblock and recompute the superblock checksum — csum_seed makes that superblock-local; verify). Pure Go, fake/file-backed tests that pass on macOS, mirroring diskfmt's existing structure and its documented go-diskfs-distrust conventions (not applicable to ext4 — we own this path end to end).

## Todos

- [x] Superblock reader: magic, feature flags (fail loudly on unknown incompat features), label, UUID, block count
- [x] Format: golden copy + label (16-byte limit enforced with an actionable error) + random UUID + superblock csum update
- [x] Behavioral tests: format a file-backed target, re-Inspect it, assert label/UUID/features; corrupt-superblock and truncated-golden-write cases produce honest errors/probe failures
- [x] Document (docstrings) that Format's output is only established once blockmount's marker lands — a bare probe-passing superblock is expected debris after a crash
- [ ] Quality gates + PR

## Summary of Changes

internal/diskfmt gains ext4 alongside FAT32/exFAT, pure Go, no mounting anywhere in Go code:

- **Inspect**: isEXT4/parseEXT4Superblock (internal/diskfmt/ext4.go) read the superblock at byte offset 1024 (magic 0xEF53), returning the 16-byte NUL-trimmed label and canonical-hex UUID via the same Contents{FS, Label} surface FAT32/exFAT use (Contents gained a UUID field). An unknown INCOMPAT feature bit (outside the exact set gosd-u988's golden manifest declares: filetype, meta_bg, extent, 64bit, flex_bg, metadata_csum_seed) is a hard error, not a silently-adopted or misreported volume.
- **Format** (internal/diskfmt/ext4format.go): streams the embedded golden image (internal/diskfmt/ext4golden.Compressed, go:embedded in a new golden.go since gosd-u988 only shipped the asset + its own contract test) onto the target in 1 MiB chunks — never buffering the whole 512 MiB, since the smallest GoSD board is RAM-constrained — patching the primary superblock and every sparse_super backup copy with a fresh random UUID and the caller's label (16-byte limit enforced) as each passes through, then recomputing that copy's own checksum.
- **Superblock checksum**: kernel.org's fs/ext4/super.c computes it as crc32c(~0, superblock_bytes[0:0x3FC]) with no final complement (unlike the "standard" CRC-32C used by e.g. iSCSI) — confirmed two ways: (1) TestEXT4ChecksumMatchesGoldenSuperblock recomputes this package's ext4Checksum against the checksum mke2fs itself wrote into the golden image and its stored s_checksum_seed (both match); (2) the containerized e2fsck cross-check below. s_checksum_seed is never touched — its whole purpose (metadata_csum_seed) is that every *other* metadata checksum in the filesystem is computed from that stored, frozen seed rather than the live UUID, so only each superblock copy's own uuid/label/checksum needs updating.
- **Backup superblocks**: stamped identically to the primary (same UUID, label, freshly computed own checksum), at every sparse_super backup group (group 1, and any group that's a power of 3, 5 or 7) — this is what tune2fs -U/-L actually does, confirmed empirically against real e2fsprogs (1.47.0, the same debian:bookworm image already pinned as container.KernelBuildImage) on a populated copy of the golden image before writing any Go: both the primary and its two backups (groups 1 and 3, in the 512 MiB / 128 MiB-per-group golden) came back with the new UUID, new label, and a self-consistent checksum after tune2fs -U -L, and e2fsck -fn was clean afterward. ext4BackupSuperblockOffsets computes this generically from the golden's own parsed geometry (blocks-per-group, block count, sparse_super flag) rather than hardcoding the golden's current 4-group layout, and refuses loudly if a backup would ever straddle the streaming chunk boundary rather than silently mis-patch one.
- **Containerized e2fsck cross-check ran, and passed clean.** TestFormatEXT4PassesRealE2fsck (internal/diskfmt/ext4docker_test.go) formats a home-staged file-backed target, then runs real e2fsprogs (container.KernelBuildImage, already pinned in this repo) against it: e2fsck -fn clean, and tune2fs -l / blkid -o export both report exactly the label and UUID Inspect itself read back. It mirrors internal/container/smoke_test.go's precedent for a docker-dependent test — opt-in via GOSD_CONTAINER_SMOKE_TEST=1, skipped (not failed) when no runtime is available — so it never becomes a silent CI dependency. Run locally via colima with that env var set: e2fsck reported the volume clean, and tune2fs -l/blkid both matched Inspect's label and UUID exactly.
- **Behavioral tests** (internal/diskfmt/ext4_test.go, ext4format_test.go): format-then-Inspect round-trip; overlong label refused; a device smaller than the golden refused; two independent formats get different UUIDs; backup superblocks verified consistent with the primary; a failing underlying writer (simulated disk-full mid-stream) surfaces as a real error rather than a silent success — the closest in-process proxy for "truncated/partial write" available without a real crash, since Inspect deliberately does not (and per this epic's crash-ordering convention, must not) treat a merely-parseable superblock as proof a whole golden image reached the medium; corrupt magic -> not recognized; unknown incompat bit -> loud error.
- **Docstrings**: FormatEXT4's doc explicitly says its output is not "established" in the crash-ordering sense — mirrors dataexpand.EstablishedMarker's phrasing — until blockmount's marker lands (bean gosd-1c0x); a probe that parses fine is not proof the whole image reached the medium.
- **Scope**: disk.Options's filesystem token, blockmount wiring, mount + EXT4_IOC_RESIZE_FS grow, and the establishment marker are gosd-1c0x's territory, untouched here; EXT4 FS / MountType() / String() were added to diskfmt.go for symmetry with FAT32/ExFAT since Format/Inspect need them, but nothing outside internal/diskfmt references them yet.

Quality gates (go test/vet/gofmt/golangci-lint incl. GOOS=linux) all pass.
