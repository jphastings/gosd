---
# gosd-oq0z
title: 'Pi 3B family coverage: ship both DTBs (3B / 3B+) and assert lan78xx Ethernet'
status: completed
type: task
priority: normal
created_at: 2026-07-26T09:43:20Z
updated_at: 2026-07-26T09:52:04Z
parent: gosd-xhc3
---

Fold the whole Pi 3B family into the pi-3b board, following JP's locked decision after the 2026-07-26 maiden hardware boot revealed the bench board is actually a **3B+** (rev a020d3, "RPI3BP" silkscreen): **one pi-3b image covers both the 3B and the 3B+**. The Pi firmware picks the DTB by board revision, so the image ships both DTBs to the FAT root; Ethernet needs both drivers asserted (the 3B's LAN9514 binds smsc95xx, the 3B+'s LAN7515 binds lan78xx).

## Locked decisions

- **Ship both DTBs**: extend the pi-3b kernelspec so `bcm2710-rpi-3-b-plus.dtb` is built and copied out alongside `bcm2710-rpi-3-b.dtb`; both ride the FAT boot partition root. Verified at the Pi fleet pin (raspberrypi/linux 63598c83): `arch/arm64/boot/dts/broadcom/bcm2710-rpi-3-b-plus.dts` exists (one-line include of the arm tree's DTS, root compatible `raspberrypi,3-model-b-plus`) and the directory Makefile lists `bcm2710-rpi-3-b-plus.dtb` under `dtb-$(CONFIG_ARCH_BCM2835)`, so the existing `make dtbs` target already produces it.
- **Kernelspec shape**: keep `KernelSpec.DTB` singular; add a minimal `AdditionalDTBs []DTB` field (only pi-3b sets it) plus an `AllDTBs()` helper, with `internal/kernelbuild` building each distinct make target once and copying every listed DTB out. Every other board's spec is untouched.
- **Fragment asserts lan78xx**: add `CONFIG_USB_LAN78XX=y` (keeping `CONFIG_USB_NET_SMSC95XX=y`). On the maiden boot it was present only by defconfig luck (`bcm2711_defconfig` line 577) — the fragment asserted only SMSC95XX, so a future trim could have silently cut the 3B+'s Ethernet.
- **No COMPATIBILITY.md changes**: pi-3b is internal-until-activation and has no table column yet (and a separate COMPATIBILITY refresh is running concurrently — that file is off-limits to this bean).
- **No artifacts.Version bump** (tag-first/bump-second): this changes kernel-build outputs, so it reaches real builds only via the next artifacts release (gosd-7wv9's activation batching, cross-ref gosd-36yy).

## Maiden-boot evidence backing this (2026-07-26, full record in gosd-xhc3 / gosd-f5xm)

- Firmware log requested `bcm2710-rpi-3-b-plus.dtb` FIRST, then fell back to our shipped `bcm2710-rpi-3-b.dtb` — and the fallback booted to hello.local HTTP 200 over wired Ethernet.
- Ethernet came up via **lan78xx** (LAN7515), not smsc95xx: the wrong-model DTB still worked because both chips self-enumerate on USB, DTB-agnostic.

## Notes

- The recorded `build/boards/pi-3b/kernel.config` does not exist yet — the maiden boot used a local-only `gosd build-kernel --board=pi-3b` + `--artifacts-dir` build. gosd-0nl7 owns the kernel.config provenance commit and the CI artifacts job; when it runs it will pick this bean's fragment/kernelspec changes up automatically. Cross-reference only — this bean does not do 0nl7's job.
- Discovered follow-up (out of scope here): the 3B+'s **WiFi** is a BCM43455, not the 3B's BCM43438 — full 3B+ WiFi coverage would need the 43455 blob set plus `raspberrypi,3-model-b-plus` alias names in the manifest. The maiden boot's headline was wired Ethernet; record the WiFi gap where the epic decides to handle it.

## Todo

- [x] kernelspec: `AdditionalDTBs` field + pi-3b entry for `bcm2710-rpi-3-b-plus.dtb`; kernelbuild builds/copies/caches it
- [x] kernel.fragment: `CONFIG_USB_LAN78XX=y` with the maiden-boot why-comment
- [x] board profile: Artifacts()/BootFiles() carry both DTBs; docs/package comment say "Raspberry Pi 3B / 3B+"
- [x] tests: kernelspec drift/board-map tests, pi3b board tests, kernelbuild script/cache tests, build integration fixture
- [x] quality gates green (test, vet, gofmt, golangci-lint darwin + GOOS=linux)

## Summary of Changes

- `internal/kernelspec`: new `KernelSpec.AdditionalDTBs []DTB` field + `AllDTBs()` helper; pi-3b lists `bcm2710-rpi-3-b-plus.dtb` (same "dtbs" make target, extra copy-out). All other boards untouched.
- `internal/kernelbuild`: the generated build script builds each distinct DTB make target once and installs every listed DTB; `outputFilenames` includes additional DTBs, so the cache key, cache-completeness check, missing-output check, and flat/staging collection all cover the new blob (an old cache entry without it is a cache miss, not a broken hit).
- `build/boards/pi-3b/kernel.fragment`: `CONFIG_USB_LAN78XX=y` asserted with the maiden-boot why-comment; Ethernet block comments now name both family chips. RequiredY picks it up mechanically (fragment-derived).
- `internal/boards/pi3b`: `Artifacts()`/`BootFiles()` carry both DTBs to the FAT root; package/const comments and `build/boards/pi-3b/README.md` document the one-image family coverage (Raspberry Pi 3B / 3B+) and the lan78xx finding.
- Tests: new kernelspec `TestAdditionalDTBsOnlyOnExpectedBoards` drift guard; `TestKernelSpecOutputsMatchBoardArtifacts` checks AdditionalDTBs against `Artifacts()`; kernelbuild script test for the shared-make-target family shape + cache-miss-on-added-DTB case; pi3b board tests and the `cmd/gosd` integration test/fixture assert the -plus blob on the boot partition.
- Bookkeeping: maiden-boot evidence recorded in gosd-xhc3's RESULTS and gosd-f5xm (left in-progress); 3B+ WiFi (BCM43455 blob gap) noted here as an epic-level follow-up. No COMPATIBILITY.md or artifacts.Version changes, per locked decisions.
