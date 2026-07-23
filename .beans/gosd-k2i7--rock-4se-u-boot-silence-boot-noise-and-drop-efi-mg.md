---
# gosd-k2i7
title: 'rock-4se U-Boot: silence boot noise and drop efi_mgr detour'
status: completed
type: task
priority: normal
created_at: 2026-07-23T11:56:59Z
updated_at: 2026-07-23T14:49:40Z
---

Found during first real-hardware boot (gosd-sz6p, 2026-07-23). Serial log shows our v0.5.0 rock-4se U-Boot spending ~0.5 s and several scary-looking lines before reaching extlinux:

- Scans global bootmeth 'efi_mgr' FIRST: 'Cannot persist EFI variables without system partition', 'Loading Boot0000 mmc 1 failed', 'EFI boot manager: Cannot load any image', 'Boot failed (err=-14)' — then falls through to extlinux, which works. Disabling the EFI bootmeth (or reordering bootmeths so extlinux is tried first) removes the detour and the noise.
- 'Card did not respond to voltage select! : -110' twice (probing the empty eMMC slot).
- 'Reading from MMC(1)... *** Warning - bad CRC, using default environment' — we never save an env; CONFIG_ENV_IS_NOWHERE would make the default env intentional and silent.
- Also emits vidconsole cursor-position escape queries to serial.

All cosmetic/latency, not correctness — extlinux boot works. Likely applies to nanopi-zero2's U-Boot too (check its config when touching this). U-Boot config change = compiled-artifact change: tag-first release dance per docs/artifacts.md, no Version bump in the same PR. Boot-time context: full baseline table in gosd-sz6p (efi_mgr detour is ~0.46 s of a ~9.2 s boot).


## Summary of Changes

Extended `bootdelay0.config` on rock-4se, nanopi-zero2, and radxa-zero-3e with two U-Boot Kconfig options (rock-4se) / one (the other two), each verified against the exact pinned U-Boot tag's source, not assumed:

**rock-4se** (`build/boards/rock-4se/uboot/bootdelay0.config`, tag v2026.04):
- `# CONFIG_BOOTMETH_EFI_BOOTMGR is not set` — stops the global 'efi_mgr' bootmeth scan that produced every noisy line in the serial log (Cannot persist EFI variables / Loading Boot0000 'mmc 1' failed / EFI boot manager: Cannot load any image / Boot failed err=-14), plus its 'mmc 1' Boot0000 probe was the source of the "Card did not respond to voltage select!" pair (that warning is otherwise legitimate empty-eMMC probing elsewhere and was NOT touched directly, per the task's instruction — it just also stops as a side effect of removing this one probe). Verified: `boot/Kconfig` at v2026.04 — `BOOTMETH_EFI_BOOTMGR` is `default y`, `depends on EFI_BOOTMGR`, `select BOOTMETH_GLOBAL`, referenced nowhere else (no other `select`/`imply`); `lib/efi_loader/Kconfig` — `EFI_BOOTMGR` is `default y`, nested in `if EFI_LOADER`; `EFI_LOADER` is `default y if !ARM || SYS_CPU = armv7 || SYS_CPU = armv8` (true for arm64). `BOOTMETH_EXTLINUX` (`boot/Kconfig`) is `default y`, unconditional, and not selected-by/dependent-on `BOOTMETH_EFI_BOOTMGR` — extlinux is unaffected.
- `# CONFIG_ENV_IS_IN_MMC is not set` + `CONFIG_ENV_IS_NOWHERE=y` — stops the "bad CRC, using default environment" warning by making the fallback-to-default-env intentional. Verified: rock-4se-rk3399_defconfig (fetched at v2026.04) explicitly sets `CONFIG_ENV_IS_IN_MMC=y`; `env/Kconfig`'s `ENV_IS_DEFAULT` is `def_bool y if !ENV_IS_IN_EEPROM && ... && !ENV_IS_IN_MMC && ...` and `select`s `ENV_IS_NOWHERE` — with `ENV_IS_IN_MMC` off and no other `ENV_IS_IN_*` set, `ENV_IS_NOWHERE` is selected automatically (the explicit `=y` is redundant but documents intent). Confirmed `scripts/kconfig/merge_config.sh` (fetched at v2026.04, read in full) deletes any prior line for a symbol before appending the fragment's line, so this correctly overrides the defconfig's explicit `=y`, unlike a bare Kconfig-default override.

**nanopi-zero2** (tag v2026.07-rc5) and **radxa-zero-3e** (tag v2026.04): same `# CONFIG_BOOTMETH_EFI_BOOTMGR is not set` line only. Verified the identical Kconfig shape at each board's own pinned tag (fetched `boot/Kconfig` + `lib/efi_loader/Kconfig` separately for v2026.07-rc5, diffed against v2026.04 — only unrelated SCSI/NAND-env lines differ). Neither board's defconfig sets `CONFIG_ENV_IS_IN_MMC=y` (fetched and read both defconfigs in full), so `ENV_IS_DEFAULT` already resolves to `ENV_IS_NOWHERE` there without any fragment change — no "bad CRC" fix needed/added for these two. Neither board has had real hardware bring-up yet (gosd-odp7, gosd-nlzf); this is a preventative fix from verified Kconfig defaults, not direct serial-log evidence for those two boards.

**Decisions recorded, not implemented:**
- `CONFIG_BOOTMETH_EFILOADER` left untouched — the serial log shows no lines attributable to it (only the global 'efi_mgr'/EFI_BOOTMGR scan produced noise; EFILOADER is a per-bootdev method and isn't evidenced as running or emitting anything), so disabling it would be an unverified guess against the actual symptom.
- "Card did not respond to voltage select!" (empty eMMC probing) left alone per the task's explicit instruction — legitimate probing of an empty socket, users may fit eMMC modules. It happens to also disappear on rock-4se as a side effect of removing the efi_mgr 'mmc 1' Boot0000 probe (same underlying probe, not a separately-targeted fix).
- vidconsole cursor-position escape queries (candidate c) — skipped. No low-risk, source-verified Kconfig option was found that isolates just this behavior without broader risk to console/video init; guessing here would violate the task's minimal-verified-change bar.

Quality gates all clean: `go test ./...`, `go vet ./...`, `gofmt -l .` (empty), `golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...`.

No `internal/artifacts.Version` bump in this PR (per project convention — artifact releases are tag-first, bump-second). Behavioral verification on real hardware happens at the next artifacts release bench pass, not in this PR.
