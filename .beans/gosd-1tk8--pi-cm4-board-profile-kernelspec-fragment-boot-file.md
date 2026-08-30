---
# gosd-1tk8
title: 'Pi CM4: board profile, kernelspec + fragment, boot files manifest (internal registration)'
status: completed
type: task
priority: normal
created_at: 2026-08-30T10:25:55Z
updated_at: 2026-08-30T11:08:16Z
parent: gosd-7676
---

## What

`internal/boards/picm4` (board ID `pi-cm4`), modeled directly on
`internal/boards/pi3b`: Arch arm64, GPU-ROM boot chain (no U-Boot),
config.txt/cmdline.txt templates, `bcm2711-rpi-cm4.dtb`.

- `internal/kernelspec`: new `pi-cm4` entry — same `piZeroRepo`/
  `piZeroCommitRef`/`piZeroCommitDate`, `bcm2711_defconfig`, arm64
  toolchain. DTB source path
  `arch/arm64/boot/dts/broadcom/bcm2711-rpi-cm4.dtb`.
- `build/boards/pi-cm4/manifest.json` (+ `manifest.go` embed): GPU boot
  firmware only (bootcode.bin/start.elf/fixup.dat, same pin as pi-3b) — no
  wifiFirmware group, this module has no wireless.
- Kernel fragment (`build/boards/pi-cm4/kernel.fragment`): start from
  pi-3b's as a base and adjust for CM4's actual USB/Ethernet wiring
  (native GENET, not a USB-attached LAN chip — don't carry over pi-3b's
  CONFIG_USB_DWCOTG/smsc95xx/lan78xx assertions unless CM4 turns out to
  need them too). No dwc2/gadget stack assertions either way — see the
  epic's "?" decision on USB gadget.
- `UsbGadgetSupport()` returns `Supported: false` with a Reason marking it
  uncharacterized (explicitly not a proven hardware limitation — see epic).
- Register internal-only in `internal/boardset/boardset.go`.

Note: registering the board (even internal-only) makes
`internal/kernelspec`'s `TestKernelConfigSnapshotCoversEveryBoard` /
`TestKernelConfigSnapshotMatchesAssertions` require a real, committed
`kernel.config` immediately — per the turing-rk1 precedent (PR #372), this
means the board-profile and kernel-build work land together in one PR,
even though they're tracked as separate beans.


## Summary of Changes

Shipped in PR (branch bean/gosd-1tk8-pi-cm4-board-profile), landed together
with the real kernel build (gosd-u5yz) per the turing-rk1 precedent (a new
board's kernelspec entry needs a real committed kernel.config the moment
it's registered, even internal-only).

- `internal/boards/picm4`: board profile mirroring pi-3b's shape (BCM2711,
  arm64, GPU-ROM boot, no U-Boot). `UsbGadgetSupport()` returns
  `Supported: false` with an "uncharacterized, not proven unsupported"
  reason (epic gosd-7676's "?" decision), unlike pi-3b's hardware-fact
  reason.
- `build/boards/pi-cm4/`: manifest.json (GPU firmware only, no
  wifiFirmware group), kernel.fragment (native GENET, BCM2711 iProc SDHCI
  storage, dwc2 gadget compiled in but not claimed supported, no WiFi/BT).
- `internal/kernelspec`: new "pi-cm4" entry, DTB
  `bcm2711-rpi-cm4.dtb` (the official CM4 IO Board's DTS).
- Registered internal-only in `internal/boardset/boardset.go`.
