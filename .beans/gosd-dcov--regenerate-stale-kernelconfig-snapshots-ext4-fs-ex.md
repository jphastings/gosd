---
# gosd-dcov
title: Regenerate stale kernel.config snapshots (EXT4_FS, EXFAT_FS, MMC_SDHCI_IPROC missing; qemu-virt's forbidden USB_MASS_STORAGE/BTRFS_FS still =y)
status: todo
type: task
priority: normal
created_at: 2026-08-20T05:12:05Z
updated_at: 2026-08-20T05:12:05Z
---

Discovered by bean gosd-ilv8's new TestKernelConfigSnapshotMatchesAssertions (internal/kernelspec/kernelconfigsnapshot_test.go): six of the eight committed build/boards/*/kernel.config snapshots already disagree with kernelspec's current RequiredY/ForbiddenY assertions, i.e. they were never regenerated after the assertion that now contradicts them landed.

## Verified

- pi-zero-2w, pi-zero-w, pi-3b: kernel.fragment requires CONFIG_EXT4_FS=y (disk's ext4 default for attached USB mass storage, bean gosd-19kw) but each committed kernel.config still shows '# CONFIG_EXT4_FS is not set'.
- pi-zero-w additionally: kernel.fragment requires CONFIG_MMC_SDHCI_IPROC=y but the committed kernel.config shows it unset.
- radxa-zero-3e, nanopi-zero2: kernel-fragment.config requires CONFIG_EXFAT_FS=y but each committed kernel.config shows '# CONFIG_EXFAT_FS is not set'.
- qemu-virt: kernel-fragment.config's ForbiddenY forbids CONFIG_USB_MASS_STORAGE (bean gosd-sz6p) and CONFIG_BTRFS_FS (bean gosd-10fn), but the committed kernel.config still has both =y.

Only rock-4se and cubie-a5e's committed snapshots are currently consistent with their assertions.

This has NO effect on real builds (gosd build-kernel always regenerates .config from defconfig + the fragment; kernel.config is never fed back in - every board README says so) and does not itself contradict CLAUDE.md's ext4/exfat/qemu-virt decisions, which are already correctly reflected in the fragments and RequiredY/ForbiddenY. The risk is purely CLAUDE.md's own documented one: a reader (human or agent) who greps the committed kernel.config instead of the fragment or a released artifact draws a stale, wrong conclusion - exactly what happened once already in bean gosd-95yu.

## Fix

Regenerate each affected board's committed kernel.config via a real 'gosd build-kernel --board <id> -o out/' run (20-75 minutes, Docker-backed - cannot run from a subagent, see CLAUDE.md) and copy out/kernel.config over the committed one, per each board's README.md. Once regenerated, delete that board's entry from internal/kernelspec/kernelconfigsnapshot_test.go's knownKernelConfigSnapshotDrift map - the test fails loudly if an entry is left in place after the snapshot no longer disagrees, so this is self-checking.

## Todos

- [ ] Regenerate pi-zero-2w, pi-zero-w, pi-3b's kernel.config (fold into the next real Pi kernel build/artifact bump if one's already planned, to avoid a build purely for this)
- [ ] Regenerate radxa-zero-3e, nanopi-zero2's kernel.config
- [ ] Regenerate qemu-virt's kernel.config
- [ ] Remove each regenerated board's entry from knownKernelConfigSnapshotDrift
