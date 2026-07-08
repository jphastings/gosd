---
# gosd-fnza
title: 'SPI: /dev/spidev on every board, loopback example, pin docs'
status: todo
type: feature
created_at: 2026-07-08T03:35:00Z
updated_at: 2026-07-08T03:35:00Z
parent: gosd-jge2
blocked_by:
    - gosd-nyad
---

Expose and document SPI on all four boards. Mirrors the I2C work (bean gosd-85pt). Branch this from the GPIO branch, not main, so the shared COMPATIBILITY/runtime edits stack cleanly.

DRIVER STATE (verify, do not assume): Pis have SPI_BCM2835=y + SPI_SPIDEV=y already. Rockchip fragments have SPI=y + SPI_ROCKCHIP=y but SPI_SPIDEV is NOT visibly in the fragment — CHECK the committed generated kernel.config for radxa + nanopi; if SPI_SPIDEV isn’t =y there, add CONFIG_SPI_SPIDEV=y to those fragments (forces a kernel rebuild).

LOCKED DECISIONS / per-board mechanism:
- Pis: add `dtparam=spi=on` to both config.txt templates (+ render tests) -> /dev/spidev0.0 and 0.1 on the standard header pins (19 MOSI/21 MISO/23 SCLK/24 CE0/26 CE1). No artifact release for the Pi half.
- Rockchips (radxa rk3566, nanopi rk3528): read the DTS at v6.18.37 — which SPI controller routes to the 40-pin header / 30-pin FPC, its status, and whether a `spidev` child node (compatible per current kernel policy — check what the kernel accepts; some versions warn on bare "spidev") + status="okay" is needed. Enable via a kernel-build DTS patch (SAME mechanism as I2C: our pinned U-Boots lack OF_LIBFDT_OVERLAY — reuse that established finding). Record bus + pins.
- ARTIFACT RELEASE: any Rockchip fragment/DTS change means new artifacts (v0.4.0) before real non---artifacts-dir builds see it. State prominently in the PR; the tag-then-bump follow-up (like gosd-xshg did for v0.3.0) is a SEPARATE task after merge — note it, do not do it in this PR.
- Example: examples/spiloopback — raw SPI_IOC_MESSAGE ioctl via golang.org/x/sys/unix (no new dep, consistent with i2cscan), does a MOSI->MISO loopback transfer (documented "jumper MOSI to MISO"), reports match/mismatch, graceful when no spidev present. periph.io as the pointer for real apps.

Per-board work:
- [ ] Verify/add SPI_SPIDEV=y in Rockchip fragments; Pi config.txt dtparam=spi=on + render tests
- [ ] Rockchip DTS patches (bus enable + spidev node) with per-board bus/pin findings recorded
- [ ] examples/spiloopback (x/sys ioctl, both arches, CI)
- [ ] docs/runtime.md SPI section (per-board bus/pins) + COMPATIBILITY SPI row -> code-complete
- [ ] Note the required v0.4.0 artifact release + follow-up bump task
- [ ] Bench: loopback passes on each board (leave unchecked)

## Acceptance
Fake-artifact integration tests show config.txt dtparam (Pis) and the DTS/fragment changes present; example compiles both arches; docs correct. Real loopback + the artifact re-release remain follow-ups.
