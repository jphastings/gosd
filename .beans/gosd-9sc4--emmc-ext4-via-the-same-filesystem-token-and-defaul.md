---
# gosd-9sc4
title: 'emmc/: ext4 via the same Filesystem token and default as disk/'
status: completed
type: feature
priority: normal
created_at: 2026-08-07T19:10:53Z
updated_at: 2026-08-07T19:46:37Z
parent: gosd-lfu0
---

JP (2026-08-07): eMMC should get ext4 like disk/ did — internal storage is exactly where the crash-safety argument applies most. This REPLACES the "emmc is FAT32-only by design" locked decision; CLAUDE.md's Public API section must be updated in the same PR.

## Locked decisions

- Mirror disk/'s surface exactly: emmc grows the same typed Filesystem token (ext4/fat32/exfat) with **zero value = ext4** — the same deliberate breaking default, shipped in the same CLI minor release as disk/'s flip (bean gosd-2194's release notes must cover both).
- Implementation should be mostly wiring: internal/blockmount's runEXT4 (golden copy → sync → mount → EXT4_IOC_RESIZE_FS grow → marker, PR #192) is already fs-parameterized and shared — emmc stops pinning diskfmt.FAT32 and passes the token through. Any place the emmc path structurally diverges from disk's ext4 path (candidate selection, eMMC device naming, boot-partition exclusion rules) gets called out explicitly, not silently absorbed.
- Kernel support is already there on every eMMC-bearing board (Rockchip fleet has EXT4_FS=y; the /proc/filesystems preflight still guards).
- Tests mirror gosd-1c0x's fake-driven state-machine set, emmc-flavored; the emmc-specific "provably unchanged FAT32 semantics" tests from #192 are superseded and updated to the new contract.
- fs-match adoption rule (from #192) now applies to emmc with ext4 requestable: an established FAT32 emmc volume + default(ext4) request → refuse without destructive; document the upgrade story in the release note (reformat is data loss — apps opt in).

## Summary of Changes

**emmc/emmc.go**: `Filesystem` type (`EXT4`/`FAT32`/`ExFAT`, zero value `EXT4`) mirrors `disk.Filesystem` token-for-token; `Options{Filesystem, Destructive}` + `FormatAndMountWith` added (`FormatAndMount` becomes a thin wrapper over `Options{Destructive: destructive}`, matching disk's shape minus a `Device` field — emmc addresses exactly one device, so there's no `FormatAndMountDevice`/`Devices` equivalent). `ErrUnsupportedFS` re-exported from `blockmount.ErrUnsupportedFS`. Package doc, `FormatAndMount`, `Filesystem` consts and `Options` docstrings rewritten to state the ext4 default and that FAT32/exFAT remain available. `chooseEMMC`'s doc comment gained a note that candidate selection is unaffected by and unrelated to this bean.

**emmc/platform_linux.go / platform_other.go**: `newPlatformDeps` wires the six ext4-only `Deps` fields (`SyncDevice`, `Grow`, `EstablishMarker`, `MarkerEstablished`, `RootHasOtherContent`, `Unmount`) to the same `internal/blockmount` functions `disk/platform_linux.go` already used — no new blockmount code was needed; emmc stops pinning `diskfmt.FAT32` in `FormatAndMount` and passes its own token through instead.

**internal/blockmount/blockmount.go**: package doc and the `Deps` ext4-fields comment updated — both packages can now reach `runEXT4`; explicitly calls out that candidate *selection* still differs (emmc: single device via `chooseEMMC`, no multi-class ranking or named-device selection; disk's `rank` explicitly excludes eMMC hardware partitions where emmc's selection relies on a sysfs-topology quirk, gosd-ix38) and that this divergence predates and is unrelated to the filesystem-token change. No behavioral/logic change in this package — `runEXT4` was already fs-parameterized and shared (PR #192/gosd-1c0x).

**Structural emmc-vs-disk divergences found and called out** (comments in blockmount.go's package doc, emmc.go, and CLAUDE.md — none required code changes since they predate this bean and are orthogonal to filesystem choice):
- Candidate selection: emmc addresses exactly one device (`chooseEMMC`, `Kind == "MMC"`); disk ranks multiple device classes and supports naming one explicitly (`FormatAndMountDevice`/`Devices`) — emmc has no equivalent.
- Hardware-partition exclusion: disk's `rank` explicitly regex-excludes an eMMC's boot/rpmb/gp partitions (`isMMCHardwarePartition`); emmc's `chooseEMMC` stays safe against them only via a sysfs quirk (those gendisks read `Kind == ""`, not `"MMC"`) — pre-existing (gosd-ix38), unchanged here.

**emmc/emmc_test.go**: added `TestOptionsZeroValueIsTheEXT4Default` and `TestFormatAndMountWithRejectsAnUnknownFilesystem` (mirroring disk_test.go), plus an `ext4Fake` (mirroring internal/blockmount's own fakeDeps, but driven through emmc's `storage()` helper so the Pkg/Noun wiring is proven, not just runEXT4's already-covered internal logic) backing seven new tests: fresh-format establishment order, adoption without re-format/re-grow, crash-debris reformat, the fs-mismatch upgrade story both ways (`TestFSMismatchEstablishedFAT32PlusEXT4DefaultRefusesWithoutDestructive`/`...ReformatsWhenDestructive` — an established FAT32 eMMC + the new zero-value ext4 request refuses with an actionable error naming both filesystems and `destructive=true`), the ext4-specific kernel preflight, and the 16-byte label cap. No existing emmc test needed behavioral changes — none asserted FAT32-only/ext4-unreachable as their point (that claim lived in gosd-1c0x's bean prose and blockmount.go's package doc, both updated), so this is purely additive.

**examples/usbwebsite/main.go**: pins the eMMC to FAT32 explicitly (`emmcOptions`) rather than picking up emmc's new ext4 default — this example's entire feature is presenting the volume to a connected computer via `gadget.MassStorage` for drag-and-drop editing, and ext4 is not natively readable from macOS/Windows. Documented in both the package doc and `emmcOptions`' doc comment. This is the one behavioral code change outside emmc/internal/blockmount this bean required.

**examples/emmcstorage/main.go**: `writeFileDurably`'s doc comment updated (no longer claims the eMMC is always FAT; the fsync/rename pattern applies regardless of filesystem, ext4's journal included).

**CLAUDE.md**: Public API bullet reworded — both `emmc` and `disk` now share the typed `Filesystem` token and default; the `disk/` ext4 bullet's "`emmc/` is unaffected" sentence removed and replaced with a new dedicated `emmc/` locked-decision bullet (decided 2026-08-07, bean gosd-9sc4) explicitly superseding "emmc is FAT32-only by design", stating the fs-match/upgrade-story behavior and the unchanged candidate-selection divergence.

**COMPATIBILITY.md**: two new rows ("ext4 on the eMMC (the default)" / "exFAT on the eMMC") mirroring the existing "ext4/exFAT on attached disks" rows' per-board values for the boards that have `emmc` package support, plus a `[^emmc-ext4]` footnote.

**docs/releases/UNRELEASED.md**: the disk-only breaking-change note rewritten to cover both packages (both defaults flip in the same release), including the eMMC fs-mismatch/upgrade-story behavior and what did *not* change for emmc (candidate selection).

**docs/runtime.md**: "Onboard eMMC storage" section updated (ext4 default, fs-match idempotency, label cap correction, cross-reference to the "ext4 by default, or FAT32/exFAT for removable media" subsection now flagged as applying to both packages identically).

Quality gates: `go build ./...`, `go test ./...`, `go vet ./...`, `gofmt -l .` (clean), `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...` (0 issues) all pass.

## Deviations from the locked decisions

None. `emmc.Filesystem` mirrors `disk.Filesystem` exactly as instructed (including exFAT, per "Mirror disk/'s surface exactly ... (ext4/fat32/exfat)"); implementation was pure wiring in emmc/blockmount as anticipated; the fs-match adoption rule required no blockmount changes since it was already generic. The one thing not explicitly anticipated in the bean text was `examples/usbwebsite`'s dependency on the eMMC being host-readable via `gadget.MassStorage` — fixed by pinning it to FAT32 explicitly rather than left to silently pick up the new default and break its documented "plug in, drag files, eject" workflow.

Quality gates + PR (https://github.com/jphastings/gosd/pull/224)
