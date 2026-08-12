---
# gosd-dtpo
title: 'ROCK 4SE: U-Boot build pipeline (TF-A from source)'
status: completed
type: task
priority: normal
created_at: 2026-07-13T12:25:00Z
updated_at: 2026-07-16T21:46:07Z
parent: gosd-cuym
blocked_by:
    - gosd-je2r
---

Mirror `build/boards/radxa-zero-3e/uboot/` (Dockerfile, build.sh, bootdelay0.config, README, .gitignore) with the rkbin-fetch stage **replaced** by a TF-A build stage: clone TF-A at A1's pinned tag → make PLAT=rk3399 CROSS_COMPILE=aarch64-linux-gnu- M0_CROSS_COMPILE=arm-none-eabi- bl31 (add gcc-arm-none-eabi to the apt list) → U-Boot rock-4se-rk3399_defconfig + bootdelay0.config merge → make BL31=<path> with **no** ROCKCHIP_TPL (RK3399 DRAM init is open-source in U-Boot TPL). First blob-free Rockchip board; README documents this as the template for future RK3399-class boards.

## Locked decisions

- Outputs idbloader.img + u-boot.itb; same raw-write offsets as radxa-zero-3e (32768 / 8388608, u-boot end ≤16MiB).
- manifest.json gets a tfa section (repo, tag, license BSD-3-Clause, note we *compile* it) instead of rkbin — check whether build/artifacts/package.sh or the workflow provenance step reads the rkbin key by name; generalize if so.
- UBOOT_TAG v2026.04 (fleet U-Boot tag) unless A1 found the defconfig missing there.

## Todo

- [x] Dockerfile with TF-A stage + U-Boot stage (FROM scratch AS artifacts)
- [x] build.sh reading manifest.json's tfa pins
- [x] manifest.json (tfa section); generalize package.sh/provenance if rkbin-keyed
- [x] Real Docker build producing idbloader.img + u-boot.itb; record sizes
- [x] CI uboot job in build-artifacts.yml
- [x] README (TF-A-from-source pattern)

## Summary of Changes

Branch `bean/gosd-dtpo-rock4se-uboot`, stacked on `bean/gosd-iosp-rock4se-kernel` (PR #88).

- `build/boards/rock-4se/uboot/`: Dockerfile (TF-A stage + U-Boot stage,
  `FROM scratch AS artifacts`), build.sh (reads `../manifest.json`'s tfa
  pins), bootdelay0.config, README (documents the no-blob RK3399 template),
  .gitignore.
- `build/boards/rock-4se/manifest.json`: `tfa` section (repo, tag v2.15.0,
  peeled commit, BSD-3-Clause license note) — no `rkbin` section at all.
  **package.sh needed no generalizing**: it copies each board's source.json
  verbatim and never reads a `rkbin` key by name. The workflow provenance
  step *did* need a rock-4se stanza: it merges the grepped `UBOOT_TAG` plus
  the manifest's whole `tfa` object into `staging/rock-4se/source.json`
  (jq `--slurpfile` from the board manifest, single source of truth).
- CI: `rock-4se-uboot` job + package-and-release wiring (needs + staging
  download), and `dist/rock-4se.tar.zst` added to the release `files:` list
  (the tarball is complete now that kernel + uboot both stage).

**Real build succeeded** (2026-07-16, Docker on the remote build box):
- `idbloader.img` 194,560 B (sha256 65210913add4…), `u-boot.itb` 1,323,520 B
  (sha256 796702a7f397…) — u-boot.itb end ≈ 9.3 MiB, well under the 16 MiB
  raw-write guard.
- Content spot-check: `dtc -I dtb` on the FIT shows "FIT image for U-Boot
  with bl31 (TF-A)" with `os = "arm-trusted-firmware"` image nodes — the
  source-built BL31 is embedded. Binman warned only about optional missing
  OP-TEE (`tee-os`), same as the other Rockchip boards.

**Deviation from a naive mirror, worth knowing (locked pin kept):** TF-A
v2.15.0 fails to link BL31 for rk3399 with Debian bookworm's
`aarch64-linux-gnu` toolchain out of the box — `region 'PMUSRAM' overflowed
by 3928 bytes`. Root cause: TF-A commit `6c2e5bf68955` ("feat(build): use
clang as a linker", in v2.13+) routes linking through the compiler driver,
which mis-places `.pmusram` with Debian/Linaro GNU toolchains
(ARM-software/tf-issues#650, Debian bug #1118651 — reproduced upstream with
GCC 14/15; ours is GCC 12). Fix: `LD=aarch64-linux-gnu-ld` on the TF-A make,
restoring the direct-GNU-ld link path that same commit explicitly keeps
supported. One-line, upstream-sanctioned, pin unchanged. Commented in the
Dockerfile.

**Pin bookkeeping:** gosd-je2r recorded TF-A v2.15.0's "commit" as
`9ad327a8d124…` — that is the annotated **tag object**; the peeled commit a
clone actually checks out is `da738d5eae93…`. manifest.json pins the peeled
commit and the Dockerfile verifies the clone's HEAD against it (standing in
for the sha256 check the blob-fetching boards do).

**Not done here:** hardware serial verification (bring-up bean gosd-sz6p);
artifacts release + activation (gosd-h8a8).
