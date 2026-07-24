---
# gosd-delc
title: Bump internal/artifacts.Version to v0.6.0
status: in-progress
type: task
priority: normal
created_at: 2026-07-24T04:39:44Z
updated_at: 2026-07-24T06:33:26Z
---

Follow-up to the artifacts/v0.6.0 release (tagged & built by JP 2026-07-23/24; contains all six board tarballs, verified via gh release view). Bump internal/artifacts.Version from v0.5.0 to v0.6.0 so stock builds pick up the g_mass_storage removal (gosd-z9l4), the U-Boot efi_mgr/env fixes (gosd-k2i7), and first-ever nanopi-zero2 artifacts consumption.

Three-way verification per docs/artifacts.md, recorded here:
- [x] Clean-machine build: fresh HOME, no --board/--artifacts-dir — all public boards build from a real v0.6.0 download (all five images built 2026-07-24, incl. first nanopi-zero2)
- [x] Offline re-run: unreachable proxy — build succeeds entirely from cache (all five images rebuilt with dead proxy + GOPROXY=off, 2026-07-24)
- [x] Content spot-check: real-hardware boot logs from BOTH boards (2026-07-24). NanoPi Zero2 (direct card slot, first RKNS-path hardware boot): full clean boot to app + DHCP + mDNS + HTTP + NTP; probes for g_mass_storage/LUN0/efi_mgr/'Cannot persist EFI'/'Boot failed'/'bad CRC'/'voltage select' ALL absent. ROCK 4SE (one successful U-Boot phase through the flaky SDWire rig): v0.6.0 U-Boot shows 'Loading Environment from nowhere... OK', bootflow scan goes straight to extlinux (no efi_mgr), no voltage-select lines — all gosd-k2i7 fixes visible

Content spot-check note (2026-07-24): clean-machine and offline legs are done. The hardware-boot spot-check is blocked on the SDWire bench rig (mux currently prevents any target from booting — rock-4se control experiment, see gosd-odp7). Complete it via a manual card swap on the rock-4se (flash hello-rock-4se.img from the v0.6.0 clean build, expect: no g_mass_storage probe error, no efi_mgr/EFI noise, no env bad-CRC warning, ~0.5s faster U-Boot phase vs the gosd-sz6p baseline) or wait for the rig fix.
