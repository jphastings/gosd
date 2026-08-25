---
# gosd-bntd
title: 'Board support: Turing RK1'
status: todo
type: epic
priority: normal
created_at: 2026-08-25T10:25:26Z
updated_at: 2026-08-25T10:30:33Z
---

Eighth supported board and the first board with no SD/microSD slot at all: Turing RK1 —
Rockchip RK3588 (full variant, not RK3588S: LPDDR4x up to 32GB, PCIe Gen3 x4), 8x Cortex-A76+A55
cores, 8/16/32GB RAM, 16GB onboard eMMC, arm64. A SO-DIMM-style compute module
(Jetson/CM4-pin-compatible) that plugs into a Turing Pi 2 or 2.5 cluster baseboard slot; each
slot gets a dedicated M.2 (NVMe) connector, but there is no SD reader anywhere on the boot path
— the baseboard's one microSD slot belongs exclusively to its BMC. JP has the hardware
(module + Turing Pi 2 baseboard); not yet wired up on the bench as of epic creation.

Refs: https://turingpi.com/product/turing-rk1/ , https://docs.turingpi.com/docs/turing-rk1-flashing-os ,
https://docs.turingpi.com/docs/turing-pi2-specs-and-io-ports

Decomposition mirrors the Cubie A5E epic (gosd-h1wv). New SoC family (RK3588, GoSD's first),
but the board-profile/kernel/image-writer shape is otherwise the established Rockchip pattern.

## Locked decisions

- **Board ID: `turing-rk1`** (build tag `gosd_turing_rk1`). Matches the mainline DTS
  (`rk3588-turing-rk1.dts`) and U-Boot defconfig (`turing-rk1-rk3588_defconfig`) names exactly —
  chosen over a bare `rk1` because that would be ambiguous with the SoC name itself. Reserve in
  CLAUDE.md's Board IDs list in this epic's first PR.
- **Mainline-only**, same rule as every other non-Pi board: no Rockchip BSP kernel, no vendor
  U-Boot fork. If mainline support is absent or immature at our pins, the epic WAITS.
- **Joins the mainline fleet kernel tag** (kernelspec's `fleetKernelTag`) — first RK3588 member
  of the Rockchip family already on that tag (radxa-zero-3e, nanopi-zero2, rock-4se).
- **No SD path exists — the built `.img` is still the same raw-disk shape as every other
  Rockchip board's** (MBR + idbloader/u-boot.itb raw writes at pinned offsets + FAT boot
  partition + extlinux), because the RK3588 BootROM/U-Boot chain doesn't care what block device
  it's written to. What's genuinely new: the end-user delivery mechanism. There is no Raspberry
  Pi Imager catalog entry for this board (Imager only drives card readers) — instead, document
  writing the same `.img` via `rkdeveloptool` in USB maskrom mode, or via the Turing Pi 2
  BMC/`tpi` upload (both raw-write an arbitrary `.img` to eMMC, the same way a card reader would
  write to SD — confirmed against Turing's own flashing docs). The config-tree hand-edit fallback
  is unaffected once the image is on eMMC.
- **Boot medium is eMMC only.** NVMe (each baseboard slot's M.2 connector) is **out of scope for
  boot** — it's in scope only as *additional* storage once booted, through the existing generic
  `disk/` package (no new code expected there), gated on mainline PCIe/NVMe actually working at
  our pinned kernel tag (there's a documented historical pinmux bug; the research bean confirms
  current status). USB gadget: candidate, confirm at research time whether the mainline DT pins a
  usable dr_mode the way rock-4se's does.
- **Requires Rockchip's rkbin blobs** (DDR-init TPL binary + BL31 ELF) — RK3588 has no
  open-source DRAM init in mainline U-Boot (unlike RK3399/A527), so this board follows the
  **radxa-zero-3e / nanopi-zero2 blob-fetch U-Boot pattern** (pinned URL + sha256, `ROCKCHIP_TPL`
  + `BL31` build args), not rock-4se/cubie-a5e's blob-free TF-A-from-source pattern. manifest.json
  records the blob pins the same way those boards' manifests do.
- **GPIO/peripheral header: out of scope for this epic.** Only the Turing Pi 2's Node-1 slot
  exposes a Pi-style 40-pin header at all, and that's a baseboard property of which slot the
  module sits in, not something the module or gosd's board profile controls — a fit for the
  existing cross-board GPIO/I2C/SPI epics (gosd-nyad, gosd-85pt, gosd-fnza) later, not this
  bring-up.
- **Peripheral enablement**, if/when anything needs it: kernel-build DTS patches, per the
  non-Pi convention — not a runtime overlay (our pinned U-Boots lack `OF_LIBFDT_OVERLAY`).
- Boot time: best effort; bring-up records a power-on→app baseline.

## Sequencing

research (GO/NO-GO gate) → board profile (RegisterInternal — de-facto prereq of the kernel
build, per CLAUDE.md) → U-Boot pipeline ∥ kernel build → artifacts release + activation →
hardware bring-up. End-user flashing docs can land any time after the board profile exists.

Docker builds (U-Boot, kernel) run 20-75 minutes each and must run from the orchestrating
session directly (backgrounded + polled), never handed to a subagent whose background jobs die
when its turn ends.



## Research outcome (gosd-k4w2, completed 2026-08-25): GO

Primary-source-verified against the fleet's ALREADY-pinned tags (v2026.04 U-Boot, v6.18.37 kernel) -- no fleet-wide tag bump needed. Corrections to the locked decisions above:

- Console is **serial9 @ 115200n8**, not the 1.5M baud other Rockchip boards default to.
- **RawWrites may be a single binman-composed `u-boot-rockchip.bin` at one offset (LBA64/32KiB)**, not the idbloader.img+u-boot.itb two-artifact split rock-4se/radxa-zero-3e use -- confirm which the actual build emits before locking the board profile's RawWrites().
- USB gadget is a candidate (DWC3 compiled in, OTG PHY node present) but not yet confirmed -- no explicit peripheral dr_mode found in the DTSI at this pass; board-profile bean confirms.
- rkbin DDR-TPL/BL31 blob versions: pin whatever the U-Boot pipeline bean actually builds/boots against, not the version numbers upstream docs happen to reference (they're stale relative to rkbin master).
- Ethernet (gmac1/RTL8211F) and PCIe (both controllers enabled in DT) look clean at our pin; NVMe is still a bring-up-time confirmation, not a boot-path concern (out of scope for boot either way).

Full findings: gosd-k4w2's Summary of Changes. Proceeding to the board-profile bean (gosd-jvtg).
