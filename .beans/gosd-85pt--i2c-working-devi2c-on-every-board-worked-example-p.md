---
# gosd-85pt
title: 'I2C: working /dev/i2c-* on every board, worked example, pin docs'
status: todo
type: feature
created_at: 2026-07-07T21:13:35Z
updated_at: 2026-07-07T21:13:35Z
parent: gosd-jge2
---

Make I2C actually usable on all four boards. Kernel drivers are already =y everywhere (I2C_BCM2835 + I2C_CHARDEV on Pis, I2C_RK3X on Rockchips); the gap is device-tree enablement and documentation.

LOCKED DECISIONS:
- I2C is enabled BY DEFAULT on every board (hardware-app platform; the Pis GPIO2/3 pin claim is the standard I2C role and gets documented). No opt-out flag in this bean — note it as a possible future gosd.toml knob only.
- The worked example adds NO new module dependencies: raw I2C_RDWR/I2C_SLAVE ioctl via golang.org/x/sys in the example itself (chip-id read of a common sensor e.g. BME280 at 0x76/0x77, graceful "no device found on the bus" output). Docs point real apps at periph.io.
- COMPATIBILITY footnote currently CLAIMS /dev/i2c-* appears at boot — false on the Pis today; the fix must correct history honestly.

Per-board work:
- [ ] Pi Zero W + Zero 2W: add dtparam=i2c_arm=on to both config.txt templates (+ render tests). Verify against rpi firmware docs that this is sufficient with our =y drivers (expect /dev/i2c-1 on GPIO2/3, header pins 3/5).
- [ ] Radxa Zero 3E: read rk3566-radxa-zero-3e.dts at v6.18.37 — which i2c buses route to the 40-pin header and their status. If okay: document bus number + pins. If disabled: build a minimal .dtbo in the kernel pipeline, ship it as a boot file, add fdtoverlays to the extlinux template (verify our U-Boot v2026.04 build has OF_LIBFDT_OVERLAY / distro-boot overlay support before choosing this path — if unsupported, patching status=okay via a committed kernel-build DTS patch is the fallback; document whichever is chosen and why).
- [ ] NanoPi Zero2: same analysis for rk3528-nanopi-zero2.dts — which i2c reaches the 30-pin FPC connector, status, pins. Same mechanism decision. (No USB caveat interaction; I2C is unaffected.)
- [ ] examples/i2cscan: small example that opens each /dev/i2c-*, scans addresses politely (read-byte probing, skipping reserved ranges), prints findings; works on all boards; compiles in CI.
- [ ] docs/runtime.md peripherals section: per-board bus/pin table (incl. FPC pin numbers for nanopi), the GPIO2/3 claim note, periph.io pointer.
- [ ] COMPATIBILITY.md: I2C row/footnote corrected (code-complete-pending-hardware phrasing, per-board notes).
- [ ] Bench verification (hardware): a real sensor responds on each board — leave unchecked.

If a kernel/DTB rebuild is needed for Rockchip overlays, the artifact consequence must be stated in the PR (new artifacts release required before real builds see it — same tag-then-bump dance as v0.2.0).

## Acceptance
Fake-artifact integration tests show the config.txt/extlinux/overlay changes in built images; example compiles for both arches; docs corrected. Real sensor reads remain bench items.
