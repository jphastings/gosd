---
# gosd-v2w1
title: 'Spike: over-the-network app updates (A/B scheme) — design doc only'
status: in-progress
type: task
priority: low
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-04T12:23:02Z
parent: gosd-jge2
---

Design, do not build. docs/design/ab-updates.md answering:

- [x] What gokrazy does (A/B root partitions, HTTP push, boot-success watchdog) and what transfers to the GoSD initramfs-only architecture (likely: two kernel+initramfs pairs on GOSD-BOOT, a boot-slot flag file for Pi config.txt vs extlinux fallback semantics on U-Boot — investigate `sysboot`/bootcount)
- [x] Failure model: power loss mid-update, bad app that boots but crashes (watchdog + rollback), FAT corruption
- [x] How the developer pushes: `gosd push <host>` against a gosd-init update endpoint — authn story (per-image key baked at build?)
- [x] Recommendation + task breakdown for v0.4

## Acceptance
Doc reviewed; follow-up beans created for the chosen design.

## Summary of Changes

Added `docs/design/ab-updates.md`: a design-only spike answering the four required questions.

- Read gokrazy's actual source (updater.go, update.go, gokrazy.go, authenticated.go) rather than summaries: A/B rootfs swap, streaming-hash HTTP PUT, the two-phase testboot/switch commit pattern, and constant-time-compared Basic auth. Identified what transfers (streaming+hash, mark-then-boot-mediated-commit, closed-by-default auth posture) and what doesn't (there is no rootfs to swap in GoSD — the update unit is the kernel+initramfs pair itself, and the two boards have no shared bootloader).
- Researched Pi tryboot (autoboot.txt, boot_partition, tryboot_a_b, one-shot firmware flag, confirmed via official docs that Zero 2W is covered and that each A/B slot must be its own bootable FAT partition) and U-Boot distro-boot (bootcount/altbootcmd, extlinux.conf default/fallback labels, confirmed via the actual pinned radxa-zero-3-rk3566_defconfig that env storage/redundancy/bootcount are not yet configured in our v2026.04 U-Boot build).
- Wrote an honest failure model: power loss mid-transfer (easy, nothing committed yet), power loss mid-commit (FAT/env-save non-atomicity is real and only best-effort mitigated), FAT corruption in general (no journal, write-then-rename reduces but doesn't eliminate risk), and a crash-looping app post-update — found that gosd-init's existing Supervisor only restarts processes, never reboots the kernel, so neither tryboot nor bootcount would ever see a userspace crash loop; proposed a new bounded 'update probation' mode that escalates to a real reboot.
- Designed `gosd push <host>` and a minimal three-endpoint update surface (GET /update/info, PUT /update, POST /update/testboot), with a per-image HMAC key baked at build time (not TLS, since clocks start at 1970 until SNTP lands; not an operator password, since GoSD has no interactive setup step) as the authn story, with the LAN-trust-boundary limitation stated explicitly rather than glossed over.
- Recommended board-native mechanisms (Pi tryboot across two FAT partitions, Radxa bootcount+dual-extlinux-entry in one partition) over one shared software-only scheme, and listed nine proposed v0.4 beans with one-line scopes in the doc (not created, per the bean's acceptance requiring JP's review first). Noted as a side effect that this design resolves gosd-xelb's deferred 'does data survive an update' question for free, since GOSD-DATA is untouched by a kernel+initramfs slot swap.

Bean stays in-progress: the acceptance checklist (doc reviewed; follow-up beans created) requires JP and is left unchecked.
