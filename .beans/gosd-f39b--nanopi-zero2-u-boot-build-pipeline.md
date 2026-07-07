---
# gosd-f39b
title: 'NanoPi Zero2: U-Boot build pipeline'
status: in-progress
type: task
priority: low
created_at: 2026-07-05T05:34:03Z
updated_at: 2026-07-07T14:46:53Z
parent: gosd-cwjf
blocked_by:
    - gosd-vcae
---

Mirror build/boards/radxa-zero-3e/uboot/: Dockerized build.sh, pinned mainline U-Boot tag, rkbin blob manifest.json (pinned commit + sha256, no re-hosting), CONFIG_BOOTDELAY=0 fragment, outputs idbloader.img + u-boot.itb to gitignored out/. Follow whatever defconfig the research task identified. Hardware serial verification stays with the bring-up task.

## Gate note (2026-07-06, from gosd-vcae research)
Mainline U-Boot support (configs/nanopi-zero2-rk3528_defconfig, with USB_GADGET/ROCKUSB enabled) lands in v2026.07, which is NOT yet released (rc only). Per project preference for stable pins, wait for the v2026.07 release tag before building; rkbin blobs needed: rk3528_ddr_1056MHz_v1.13.bin + rk3528_bl31_v1.21.elf (same redistribution license as the RK3566 blobs). Recheck the U-Boot release page early August 2026.

## Gate amended (2026-07-07, JP: wants hardware-testable ASAP)
Do NOT wait for the v2026.07 release: pin the LATEST v2026.07-rc tag now (rcs are pinnable and conventionally fine for bring-up; our stable-pin preference is about reproducibility, which an rc tag preserves).
- [ ] Re-pin to the final v2026.07 release tag when it ships (~Aug 2026) and rebuild/re-release artifacts


## Summary of Changes (2026-07-07)

Implemented, mirroring build/boards/radxa-zero-3e/uboot/:

- Verified `configs/nanopi-zero2-rk3528_defconfig` exists at the LATEST
  v2026.07-rc tag (rc5, checked via both source.denx.de and the GitHub
  mirror) before doing anything else -- confirmed present, with
  `CONFIG_DEFAULT_DEVICE_TREE="rockchip/rk3528-nanopi-zero2"`,
  `CONFIG_USB_GADGET=y`, `CONFIG_USB_FUNCTION_ROCKUSB=y`,
  `CONFIG_BAUDRATE=1500000`, `CONFIG_DEBUG_UART_BASE=0xFF9F0000` all as
  the research predicted.
- `build/boards/nanopi-zero2/uboot/{Dockerfile,build.sh,bootdelay0.config,README.md,.gitignore}`,
  built the same way as the Radxa Zero 3E pipeline (debian:bookworm-slim,
  aarch64-linux-gnu- cross, build-essential + crossbuild-essential-arm64 +
  python3-dev + libgnutls28-dev, multi-stage FROM scratch artifacts stage).
  `UBOOT_TAG="v2026.07-rc5"`.
- `build/boards/nanopi-zero2/manifest.json`: pinned rkbin commit
  `ecb4fcbe954edf38b3ae037d5de6d9f5bccf81f4` (same commit already pinned for
  the Radxa Zero 3E -- it also carries the RK3528 blobs), blobs
  `bin/rk35/rk3528_ddr_1056MHz_v1.13.bin` (sha256
  `ab88df0d98a882a5ebb50b82f4481a2506448a2ce6e0e6efaf759e4438cc372e`) and
  `bin/rk35/rk3528_bl31_v1.21.elf` (sha256
  `ab0b30d59e81b8a14ec6f45316d1dd9befc23fae02372a989ec63c90a4790bab`), both
  hashes computed from the real fetched files (not placeholders), license
  file noted as the rkbin repo-root LICENSE.
- Ran the real Docker build: succeeded, producing `idbloader.img` (176,128
  bytes) and `u-boot.itb` (1,025,024 bytes, valid FIT).
- Added `nanopi-zero2-uboot` job to `.github/workflows/build-artifacts.yml`
  (mirrors the radxa-zero-3e-uboot job), added it to `package-and-release`'s
  `needs` + download-artifact + provenance (source.json) steps -- kernel and
  U-Boot now share one staging/nanopi-zero2 dir and one released tarball,
  same pattern as radxa-zero-3e. actionlint clean.
- Updated COMPATIBILITY.md's `[^nanopi-artifacts]` footnote to reflect the
  new U-Boot job and the rc pin (board-level 🚧 status unchanged -- that's
  gated on the board profile, gosd-wskc, which this task does not touch).
- README prominently documents the rc pin and points the re-pin work at
  this bean's open checklist item.

Did NOT touch internal/boards or internal/artifacts (owned by the board
profile / flip-to-public work, gosd-wskc). Hardware serial verification and
the re-pin-to-final-release checklist item both remain open, as directed.
