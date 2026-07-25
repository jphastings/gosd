---
# gosd-xhc3
title: 'Board support: Raspberry Pi 3B'
status: todo
type: epic
created_at: 2026-07-25T23:20:08Z
updated_at: 2026-07-25T23:20:08Z
---

Add the Raspberry Pi 3B (board ID pi-3b, BCM2837 SoC, arm64) as a GoSD board. Same Broadcom family and GPU-ROM boot flow as pi-zero-2w; joins the existing Pi fleet kernel pin. Headline feature vs the Zeros: onboard wired Ethernet (LAN9514 USB hub + 100Mbit). Planned 2026-07-26; decomposition mirrors the ROCK 4SE epic (gosd-cuym) adapted to the Pi family (no U-Boot, no viability-research bean: mainline-tree presence was verified up front, see below).

## Children

- gosd-ypg1 — board profile, kernelspec + fragment, firmware manifest (internal registration; first PR)
- gosd-0nl7 — trimmed arm64 kernel build (kernel8.img) + CI artifacts job
- gosd-7wv9 — artifacts release + board activation (batches into the next release window, see gosd-36yy cross-ref below)
- gosd-f5xm — hardware bring-up and boot-time measurement

## Locked decisions

- **Board ID `pi-3b`** (build tag `gosd_pi_3b`), following the pi-zero-2w/pi-zero-w
  hyphenation convention (model name hyphenated, lowercase). Reserve it in
  CLAUDE.md's Board IDs list in the activation PR.
- **Arch bucket arm64**: BCM2837 is the same SoC family as the Zero 2W (the 2W's
  DTB is bcm2710-\*). Kernel from raspberrypi/linux at the SAME Pi fleet pin
  (`piZeroCommitRef` = 63598c83153e19b1f99067ab6df7409de2c111f8 on rpi-6.18.y,
  see `internal/kernelspec`), `bcm2711_defconfig` base, outputs `kernel8.img` +
  DTB — fleet consistency across the Broadcom boards, never a single-board pin.
- **Registration sequencing (gosd-wskc / gosd-0vvh precedent)**: register via
  `boards.RegisterInternal` in the first PR — public registration before the
  artifacts release exists would 404 on real artifact fetches and break CI's
  image-smoke job (which builds all PUBLIC boards from the pinned release;
  gosd-et0q's early public-flip-plus-Version-bump turned that job red and
  cemented this rule). Flip to public `boards.Register` + `internal/artifacts.Version`
  bump + catalog entry + COMPATIBILITY column in ONE activation PR after JP
  pushes the artifacts tag (gosd-h8a8 pattern; tag-first/bump-second per
  docs/artifacts.md).
- **Ethernet is the headline feature** and must be asserted, not inherited: the
  LAN9514 (USB2 hub + 100Mbit eth) hangs off the SoC's only USB port. The stock
  rpi DTS routes it via `bcm283x-rpi-smsc9514.dtsi`, and the USB controller node
  is `compatible = "brcm,bcm2708-usb"`, which binds the rpi tree's downstream
  dwc_otg driver (`CONFIG_USB_DWCOTG`) — NOT dwc2 (its of_match table has no
  bcm2708-usb entry; both verified at the pin, see Verified facts). The fragment
  asserts `CONFIG_USB_DWCOTG=y`, `CONFIG_USB_NET_DRIVERS=y`, `CONFIG_USB_USBNET=y`,
  `CONFIG_USB_NET_SMSC95XX=y` (all already =y in the bcm2711_defconfig baseline —
  asserting them promotes them into RequiredY so a future trim can't cut them
  silently, the same reasoning as rock-4se asserting NVMe/exFAT).
- **USB gadget: NOT possible on the 3B** — the SoC's USB is hard-wired through
  the onboard LAN9514 hub, so the port can never be a peripheral.
  `UsbGadgetSupport{Supported: false}` with an actionable reason (gosd-5pnr makes
  `gosd build --usb-gadget` fail fast); the config.txt template has no
  dwc2-overlay branch; COMPATIBILITY.md's gadget cell gets ➖ not-applicable with
  a hardware-limitation footnote (the [^pi-no-eth]/[^no-m2] style), at activation.
- **WiFi: BCM43438 (brcmfmac "43430" family), WPA2-PSK/open only** per project
  scope. Blobs pinned from RPi-Distro/firmware-nonfree at the SAME commit as the
  other Pi manifests (9794282eb9f4a2de1f23b41a738926740e975d83) — see Verified
  facts. Bluetooth is out of scope (GoSD has no BT anywhere; the fragment keeps
  CONFIG_BT cut).
- **Serial console: mini-UART** (BT holds the PL011): the DTS aliases put
  serial0 = uart1, so console=serial0,115200 → ttyS0 via
  `CONFIG_SERIAL_8250_BCM2835AUX`. The gosd-md4w lesson applies verbatim and is a
  hard requirement: the fragment MUST carry `CONFIG_SERIAL_8250_RUNTIME_UARTS=1` —
  bcm2711_defconfig ships NR_UARTS=5/RUNTIME_UARTS=0 (lines 739-740 at the pin,
  verified) and relies on firmware cmdline injection of 8250.nr_uarts, exactly
  the dependency that dead-ended the pi-zero-w bench session. Encode it from day
  one; do not wait to trip over it on the bench. (pi-zero-2w shares the gap and
  got lucky via firmware injection — fixing IT is gosd-md4w-adjacent follow-up
  territory, not this epic's.)
- **GPU boot firmware**: same raspberrypi/firmware tag 1.20260521 (commit
  09267f5354d40519d82fbd2193b9e211ec304055) pins as the other Pi manifests;
  bootcode.bin/start.elf/fixup.dat hashes re-verified by independent download
  2026-07-26 (byte-identical to both existing Pi manifests).
- **config.txt/cmdline.txt follow the pizero2w templates**: arm_64bit=1,
  kernel=kernel8.img, initramfs followkernel, enable_uart=1, disable_splash=1,
  boot_delay=0, avoid_warnings=1, dtparam=i2c_arm=on, dtparam=spi=on; cmdline
  `console=serial0,{baud} quiet init=/init gosd.board=pi-3b` (default 115200,
  --console-baud supported).
- **Artifacts release batching (cross-ref gosd-36yy)**: the 3B's first artifacts
  batch into the NEXT artifacts release window — most likely the fleet kernel
  bump waiting on Linux v7.2.0 (gosd-36yy), or any earlier release another
  board's change forces — rather than forcing their own release. The activation
  bean stays blocked until whichever `artifacts/vX.Y.Z` release first ships the
  pi-3b kernel job's outputs.

## Verified facts (2026-07-26, against the pinned sources)

- **DTB name**: `bcm2710-rpi-3-b.dtb`. At raspberrypi/linux 63598c83,
  `arch/arm64/boot/dts/broadcom/bcm2710-rpi-3-b.dts` exists (a one-line
  `#include "arm/broadcom/bcm2710-rpi-3-b.dts"`) and the directory Makefile
  lists `bcm2710-rpi-3-b.dtb` under `dtb-$(CONFIG_ARCH_BCM2835)` — so the Pi
  boards' `make dtbs` build produces it. (`bcm2837-rpi-3-b.dts` also exists —
  that's the mainline-style DT; the Pi firmware loads the bcm2710-\* name by
  board match, the same convention as the Zero 2W's bcm2710-rpi-zero-2-w.dtb.)
- **Board compatible**: `raspberrypi,3-model-b` (root compatible of the DTS),
  so brcmfmac requests firmware named
  `brcmfmac43430-sdio.raspberrypi,3-model-b.<ext>`.
- **WiFi blobs**: at RPi-Distro/firmware-nonfree 9794282e,
  `brcm/brcmfmac43430-sdio.raspberrypi,3-model-b.{bin,clm_blob}` are git
  symlinks to `../cypress/cyfmac43430-sdio.{bin,clm_blob}` and `...3-model-b.txt`
  symlinks to `brcm/brcmfmac43430-sdio.txt` — the EXACT same three underlying
  files pi-zero-w's manifest already pins (same URLs; sha256s re-verified by
  independent download 2026-07-26: bin 0717f8e7…, clm_blob 3376b9c9…, txt
  fc3949a4…). **No `43430b0` alias exists for `3-model-b` at this commit**
  (checked the full brcm/ directory listing: b0 aliases exist only for
  model-zero-2-w) — note this contradicts a passing remark in gosd-06kj's
  findings ("only model-zero-2-w, 3-model-b, and 0-compute-module have a b0
  variant"); the directory listing is authoritative, so the 3B manifest carries
  exactly three aliases, the same shape as pi-zero-w.
- **Serial**: DTS `aliases { serial0 = &uart1; serial1 = &uart0; }` — mini-UART
  console, BT on the PL011, as expected.
- **Ethernet driver chain**: `bcm283x-rpi-smsc9514.dtsi` declares the hub
  (`usb424,9514`) + ethernet (`usb424,ec00`) under `&usb`; `bcm270x.dtsi`'s usb
  node is `compatible = "brcm,bcm2708-usb"`; `drivers/usb/dwc2/params.c`'s
  of_match table has `brcm,bcm2835-usb` but NOT `brcm,bcm2708-usb`;
  `drivers/usb/host/Kconfig` defines `USB_DWCOTG` (bool, `depends on USB=y &&
  (FIQ || ARM64)` — arm64 OK). bcm2711_defconfig at the pin: `CONFIG_USB_DWCOTG=y`
  (line 1282), `CONFIG_USB_USBNET=y` (578), `CONFIG_USB_NET_SMSC95XX=y` (590);
  `drivers/net/usb/Kconfig`: USB_NET_SMSC95XX selects PHYLIB/SMSC_PHY/BITREVERSE/
  CRC16/CRC32, so no extra symbols need asserting. pi-zero-2w's committed
  kernel.config (same defconfig + a fragment that never touches these) confirms
  they all survive our trim as =y.

## Hack-boot viability probe (placeholder)

A bench probe — flashing a stock pi-zero-2w image and overriding
`device_tree=bcm2710-rpi-3-b.dtb` in config.txt on a real 3B — may produce
data imminently. Expected: kernel + WiFi blobs are compatible in principle
(same arm64 kernel, and the Zero W note in gosd-06kj shows the 3B shares the
Cypress 43430 blob set), but Ethernet cannot work (the zero-2w image's DTB
name and gosd.board are wrong, and its kernel.config — while carrying
USB_DWCOTG/SMSC95XX from the shared defconfig — was never bench-proven in
host mode), and serial console likely dies to the RUNTIME_UARTS=0 trap unless
the firmware injects 8250.nr_uarts. Record actual results here when they
arrive; they inform gosd-0nl7 (kernel) and gosd-f5xm (bring-up) but block
nothing.

RESULTS: (pending)
