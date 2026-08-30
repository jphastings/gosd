---
# gosd-7676
title: 'Board support: Raspberry Pi CM4'
status: todo
type: epic
created_at: 2026-08-30T10:25:34Z
updated_at: 2026-08-30T10:25:34Z
---

## What

Add GoSD board support for the Raspberry Pi Compute Module 4 (CM4), Lite
(no eMMC)/no-wireless variant, running in a Turing Pi 2 node slot (node 1,
this bring-up).

## Locked decisions

- **Board ID:** `pi-cm4`. Display name: "Raspberry Pi CM4".
- **SoC/kernel reuse:** BCM2711, the same family pi-3b and pi-zero-2w
  already build for. Same kernel commit pin (`piZeroCommitRef`), same
  `bcm2711_defconfig`, same toolchain (internal/kernelspec). pi-3b's
  committed kernel.config already carries CONFIG_BCMGENET,
  CONFIG_BROADCOM_PHY, CONFIG_MDIO_BCM_UNIMAC as bare defconfig defaults
  (not fragment additions) — CM4's native GENET Ethernet needs no new
  Kconfig work, just don't cut it in the fragment.
- **Boot chain:** GPU-ROM boot, same as pi-3b/pi-zero-2w — no U-Boot.
  config.txt/cmdline.txt + kernel8.img + DTB in the FAT boot partition.
- **DTB:** `bcm2711-rpi-cm4.dtb`, built from the same tree/commit (an
  additional copy-out target in KernelSpec, same shape as pi-3b's
  AdditionalDTBs).
- **No WiFi firmware section** — this module is Lite/no-wireless, so
  manifest.json carries boot firmware only. First Pi board GoSD ships with
  no wireless firmware group at all.
- **Out of scope for this pass:**
  - eMMC boot — unknown/irrelevant for this module; SD-boot only, same
    carve-out shape as Turing RK1's NVMe-boot exclusion (`gosd-bntd`).
  - GPIO header — Turing Pi 2 nodes are a compute-module form factor with
    no breakout header, same reasoning as RK1.
  - NVMe — not wired/supported for this node (JP confirmed).
- **USB gadget mode: explicitly left as "?" (unknown), not best-effort.**
  JP: "Let's ignore gadget mode, and treat it as a '?' feature for now."
  `UsbGadgetSupport()` returns `Supported: false` with a Reason naming it
  uncharacterized (not a proven hardware limitation like RK1's) — refuse
  safely rather than claim support that may not exist. Follow-up
  characterization happens if/when it matters, not blocking this epic.
- Registered internal-only (`boards.RegisterInternal`) until an artifacts
  release publishes its kernel — same carve-out as every other recent
  board (turing-rk1, rock-4se before activation, etc).

## Bench

Turing Pi 2 (v2.4), CM4 in node 1, SDWire on its SD card, Ethernet via the
baseboard. No USB OTG dock connected this round (gadget mode untested,
per the "?" decision above).
