---
# gosd-85pt
title: 'I2C: working /dev/i2c-* on every board, worked example, pin docs'
status: in-progress
type: feature
priority: normal
created_at: 2026-07-07T21:13:35Z
updated_at: 2026-07-07T22:44:20Z
parent: gosd-jge2
---

Make I2C actually usable on all four boards. Kernel drivers are already =y everywhere (I2C_BCM2835 + I2C_CHARDEV on Pis, I2C_RK3X on Rockchips); the gap is device-tree enablement and documentation.

LOCKED DECISIONS:
- I2C is enabled BY DEFAULT on every board (hardware-app platform; the Pis GPIO2/3 pin claim is the standard I2C role and gets documented). No opt-out flag in this bean — note it as a possible future gosd.toml knob only.
- The worked example adds NO new module dependencies: raw I2C_RDWR/I2C_SLAVE ioctl via golang.org/x/sys in the example itself (chip-id read of a common sensor e.g. BME280 at 0x76/0x77, graceful "no device found on the bus" output). Docs point real apps at periph.io.
- COMPATIBILITY footnote currently CLAIMS /dev/i2c-* appears at boot — false on the Pis today; the fix must correct history honestly.

Per-board work:
- [x] Pi Zero W + Zero 2W: add dtparam=i2c_arm=on to both config.txt templates (+ render tests). Verify against rpi firmware docs that this is sufficient with our =y drivers (expect /dev/i2c-1 on GPIO2/3, header pins 3/5).
- [x] Radxa Zero 3E: read rk3566-radxa-zero-3e.dts at v6.18.37 — which i2c buses route to the 40-pin header and their status. If okay: document bus number + pins. If disabled: build a minimal .dtbo in the kernel pipeline, ship it as a boot file, add fdtoverlays to the extlinux template (verify our U-Boot v2026.04 build has OF_LIBFDT_OVERLAY / distro-boot overlay support before choosing this path — if unsupported, patching status=okay via a committed kernel-build DTS patch is the fallback; document whichever is chosen and why). FOUND: i2c3 (m0 pinmux, GPIO1_A0/A1) routes to header pins 3/5, disabled by default; U-Boot v2026.04 defconfig lacks CONFIG_OF_LIBFDT_OVERLAY, so used the DTS-patch fallback.
- [x] NanoPi Zero2: same analysis for rk3528-nanopi-zero2.dts — which i2c reaches the 30-pin FPC connector, status, pins. Same mechanism decision. (No USB caveat interaction; I2C is unaffected.) FOUND: i2c5 (m0 pinmux, GPIO1_B2/B3), FPC pins 12/13, needs external 2.2k pull-up per FriendlyElec schematic; NOT i2c1 (that bus is already enabled but only for the onboard RTC, not header-routed). rk3528.dtsi doesn't pre-alias i2cN like rk356x-base.dtsi does, so the patch also adds an i2c5 alias to guarantee /dev/i2c-5. Same DTS-patch mechanism as Radxa (U-Boot v2026.07-rc5 also lacks CONFIG_OF_LIBFDT_OVERLAY).
- [x] examples/i2cscan: small example that opens each /dev/i2c-*, scans addresses politely (read-byte probing, skipping reserved ranges), prints findings; works on all boards; compiles in CI.
- [x] docs/runtime.md peripherals section: per-board bus/pin table (incl. FPC pin numbers for nanopi), the GPIO2/3 claim note, periph.io pointer.
- [x] COMPATIBILITY.md: I2C row/footnote corrected (code-complete-pending-hardware phrasing, per-board notes).
- [ ] Bench verification (hardware): a real sensor responds on each board — leave unchecked.

If a kernel/DTB rebuild is needed for Rockchip overlays, the artifact consequence must be stated in the PR (new artifacts release required before real builds see it — same tag-then-bump dance as v0.2.0).

## Acceptance
Fake-artifact integration tests show the config.txt/extlinux/overlay changes in built images; example compiles for both arches; docs corrected. Real sensor reads remain bench items.

## Summary of Changes

I2C is now enabled by default on all four boards, code-complete and
fake-artifact-tested (bench verification with real hardware remains the one
unchecked item, as scoped).

- **Pi Zero 2W / Zero W**: `dtparam=i2c_arm=on` added to both `config.txt`
  templates (unconditional, no opt-out). `/dev/i2c-1` on header pins 3
  (SDA) / 5 (SCL) — the standard `GPIO2`/`GPIO3` position.
- **Radxa Zero 3E**: `i2c3` (pinmux `i2c3m0_xfer`, GPIO1_A0/A1) routes to the
  40-pin header's pins 3/5, confirmed against Radxa's schematic and pinout
  docs — mainline leaves it disabled. `rk356x-base.dtsi` already aliases
  every `i2cN`, so it's guaranteed to enumerate as `/dev/i2c-3`.
- **NanoPi Zero2**: `i2c5` (pinmux `i2c5m0_xfer`, GPIO1_B2/B3) routes to the
  30-pin FPC's pins 12 (SCL)/13 (SDA), confirmed against FriendlyElec's
  schematic (`NanoPi_Zero2_2407_SCH.pdf`) — **not** `i2c1`, which this board
  already enables but only for its onboard RTC, not the header. The
  schematic notes an external ~2.2kΩ pull-up is needed on this bus (no
  onboard pull-ups, unlike `i2c1`'s RTC bus). `rk3528.dtsi` doesn't
  pre-alias `i2cN` the way the Radxa's SoC dtsi does (this board's own
  `.dts` already has to alias `i2c1` explicitly for the same reason), so the
  patch adds an `i2c5` alias too, to guarantee `/dev/i2c-5`.
- **Mechanism for both Rockchip boards**: a kernel-build-time DTS patch
  (`build/boards/{radxa-zero-3e,nanopi-zero2}/kernel/patches/`), applied by
  `docker-build.sh` right after cloning, rather than a `.dtbo` overlay —
  checked both pinned U-Boot defconfigs directly
  (`radxa-zero-3-rk3566_defconfig` at v2026.04,
  `nanopi-zero2-rk3528_defconfig` at v2026.07-rc5) and neither sets
  `CONFIG_OF_LIBFDT_OVERLAY`, so extlinux's `fdtoverlays` isn't available.
  **This needs a new artifacts release** (DTB rebuild) before a real,
  non-`--artifacts-dir` `gosd build` picks up the change on either Rockchip
  board — same tag-then-bump dance as v0.2.0. Documented prominently in both
  kernel READMEs and the U-Boot READMEs' "Known gaps".
- `examples/i2cscan`: opens every `/dev/i2c-*`, does a polite single-byte-read
  scan of the full non-reserved address range, and separately checks
  0x76/0x77 for a BME280/BMP280 chip-ID response via a raw `I2C_RDWR`
  ioctl — `golang.org/x/sys/unix` only, no new module dependency. Builds
  clean under `go vet`/`golangci-lint` on both `arm64` and `arm`/`GOARM=6`,
  added to the CI smoke-build job's cross-compile lists.
- `docs/runtime.md` gained an "I2C is on by default" section: per-board
  device/pin table, the GPIO2/3 claim note, the NanoPi's external pull-up
  requirement, and a pointer at `examples/i2cscan` / periph.io.
- `COMPATIBILITY.md`: split the combined "GPIO / I2C / SPI" row into "I2C"
  (now ✅, with a footnote correcting the previous false claim that
  `/dev/i2c-*` already appeared at boot) and "GPIO / SPI" (still 🚧,
  unchanged scope, still tracked by bean `gosd-rsrd`).

Deviation from the bean's literal per-board checklist wording: the NanoPi
Zero2 bus is `i2c5`, not the `i2c1` an initial read of the board `.dts`
might suggest (that bus is real and enabled, but only serves the onboard
RTC) — corrected after cross-referencing the FriendlyElec schematic's GPIO
table, which is the only source that actually states which signals reach
the 30-pin FPC connector.
