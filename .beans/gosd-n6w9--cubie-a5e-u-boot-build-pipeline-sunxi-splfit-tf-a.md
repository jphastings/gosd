---
# gosd-n6w9
title: 'Cubie A5E: U-Boot build pipeline (sunxi SPL+FIT, TF-A from source)'
status: completed
type: task
priority: normal
created_at: 2026-08-06T22:33:44Z
updated_at: 2026-08-07T11:19:28Z
parent: gosd-h1wv
blocked_by:
    - gosd-jpc8
---

build/boards/cubie-a5e/{manifest.json,uboot/} mirroring rock-4se's blob-free pattern: Dockerfile + build.sh building mainline U-Boot radxa-a5e_defconfig with BL31 compiled from mainline TF-A (make PLAT=sun55i_a523), pins read from manifest.json, tag-verified against the peeled commit. Output artifact is the single u-boot-sunxi-with-spl.bin (docker cp out of a scratch stage).

Confirmed pins (gosd-jpc8): U-Boot mainline v2026.04, defconfig radxa-cubie-a5e_defconfig (board DT in-tree at that tag). TF-A from jernejsk/arm-trusted-firmware branch a523 @ b5de74a685fb73b784e45bbbd18dd9a0c528d8b2 — make PLAT=sun55i_a523 bl31, passed via BL31=; SCP=/dev/null (A523 uses no SCP). Keep the bootdelay0.config merge (same boot-time rationale as the other boards). Output artifact: u-boot-sunxi-with-spl.bin (binman FIT: SPL + BL31 + U-Boot proper + DTB), flashed at 8KiB.

## Todos

- [x] manifest.json with tfa section (repo/tag/peeled commit/license note), schema note explaining the sunxi blob-free chain
- [x] uboot/Dockerfile (TF-A bl31 stage + U-Boot stage + scratch artifacts stage) and build.sh (jq-driven pins, out/ copy)
- [x] uboot/README.md: boot-chain explanation, offset, how to bump pins
- [x] Local Docker build succeeds; record u-boot-sunxi-with-spl.bin size (must fit the pre-partition gap with room to spare)
- [ ] Quality gates + PR


## Summary of Changes

Added `build/boards/cubie-a5e/{manifest.json,uboot/}` mirroring rock-4se's blob-free U-Boot pipeline, adapted to the sunxi chain:

- `manifest.json`: `tfa` section pinning `jernejsk/arm-trusted-firmware` branch `a523` @ commit `b5de74a685fb73b784e45bbbd18dd9a0c528d8b2` (BSD-3-Clause). Unlike rock-4se's tagged-release TF-A pin, this is a moving branch on a fork (mainline TF-A has no `sun55i_a523` platform — gosd-jpc8), so the commit is authoritative and the branch name is informational only; the Dockerfile fetches the pinned commit directly (`git fetch --depth 1 origin $TFA_COMMIT`) rather than cloning the branch and verifying HEAD, so a moved branch tip cannot break the build.
- `uboot/Dockerfile`: two-stage build (TF-A `bl31` then U-Boot, then a `FROM scratch` artifacts stage). `make PLAT=sun55i_a523 bl31` needed neither `M0_CROSS_COMPILE`/`gcc-arm-none-eabi` (rk3399's Cortex-M0 PMU quirk, not applicable here) nor an `LD=aarch64-linux-gnu-ld` override (rk3399's clang-linker `.pmusram` workaround, not needed on this platform) — both were tested and confirmed unnecessary before being left out. U-Boot builds `radxa-cubie-a5e_defconfig` at `v2026.04`, merges `bootdelay0.config`, and links with `BL31=<bl31.bin>` (raw binary, not `.elf`) and `SCP=/dev/null` (A523 uses no SCP firmware).
- `uboot/bootdelay0.config`: `CONFIG_BOOTDELAY=0` fragment. Verified against U-Boot's `env/Kconfig` that, unlike rock-4se's defconfig, `radxa-cubie-a5e_defconfig` sets no `CONFIG_ENV_IS_IN_*` option, so it already resolves to `ENV_IS_NOWHERE` by Kconfig default — no environment-source fragment needed.
- `uboot/build.sh`: jq-driven pins from `../manifest.json` (`tfa.repo`/`tfa.branch`/`tfa.commit`), `docker build --target artifacts`, `docker create`/`cp` of the single `u-boot-sunxi-with-spl.bin` artifact into `./out/`.
- `uboot/README.md`: explains the sunxi single-file boot chain (BootROM loads `u-boot-sunxi-with-spl.bin` from byte 8KiB/sector 16, vs. Rockchip's two-file idbloader+itb split), the TF-A fork-pin rationale and when to revisit it (mainline TF-A gaining `sun55i_a523`), the BL31 `.bin`-not-`.elf` and `SCP=/dev/null` handoff details, and how to bump pins.

**Verified locally end-to-end**: `build/boards/cubie-a5e/uboot/build.sh` runs clean under Docker (colima) and produces `out/u-boot-sunxi-with-spl.bin` at **819137 bytes** (~800KiB) — comfortably under the 16MiB boot-partition start minus the 8KiB BootROM offset that `internal/boards/cubiea5e`'s `maxUbootEndBytes` guard enforces.

Go quality gates (`go build`, `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...` native and `GOOS=linux`) all clean — this PR touches no Go code. `shellcheck build.sh` clean.
