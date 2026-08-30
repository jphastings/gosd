---
# gosd-hycf
title: 'Turing RK1: hardware bring-up and boot-time measurement'
status: completed
type: task
priority: normal
created_at: 2026-08-25T10:26:48Z
updated_at: 2026-08-30T09:26:17Z
parent: gosd-bntd
blocked_by:
    - gosd-wf58
---

Real-hardware verification on JP's bench (module + Turing Pi 2 baseboard) once the artifacts release + activation lands: flash via rkdeveloptool or the TP2 BMC, confirm serial console output (UART/baud from the research bean), boot to app, eMMC data-partition durability (four-step fsync pattern), and a power-on-to-app boot-time baseline. Not yet wired up on the bench as of epic creation -- JP is getting it ready in parallel with the software work.


## Bench session (2026-08-30): boot + networking verified, real bug found and fixed

Hardware: Turing RK1 + Turing Pi 2 (v2.4 board, BMC firmware upgraded
2.0.5->2.3.4 during this session -- see below), currently in node 4.

**Flashing path found**: the BMC-driven flash (both `tpi flash` CLI and its
direct REST API) failed universally on the original v2.0.5 BMC firmware
with "No supported devices found", on every node tried (1-4). Root cause:
v2.0.5 was the *first* BMC release to add RK1 support at all (Nov 2023) and
had known rockusb-driver bugs; upgrading to v2.3.4 (Feb 2025 release, fixes
rockusb READ_CAPACITY handling) fixed it completely. `tpi`'s own CLI proved
unreliable to drive from this environment (a local Keychain-access issue,
not a BMC problem) -- ended up driving the whole flash/power/UART flow via
direct curl against the BMC's REST API (`https://turingpi.local/api/bmc`,
Basic Auth), which worked cleanly throughout. A working recipe:
`opt=set&type=flash&file=X&length=N&sha256=H&node=N-1` (0-indexed!) to get
a handle, `POST .../upload/{handle}` multipart to send the file, poll
`opt=get&type=flash` for Done/Error, `opt=set&type=power&node{1-indexed}=1`
to boot, `opt=get&type=uart&node=N-1` to read the console (16KB ring
buffer -- can evict early boot lines under noisy drivers).

**Found and fixed a real bug**: default console=ttyS9 panicked on real
hardware ("unable to open an initial console"); needed ttyS9->ttyS0
(bean gosd-vh82, PR #373, merged). With that fix, a stock `gosd build
--board turing-rk1` image (the `hello` example) boots completely clean, no
kernel-param workarounds: U-Boot SPL/proper (FIT signature checks all
passed) -> kernel 6.18.37 -> gosd-init -> the app. Verified end-to-end:
- Boot partition found and mounted (`/dev/mmcblk0p1`)
- DHCP lease obtained, mDNS responder answering as hello.local
- `curl http://hello.local/` reached the app live over the network
- NTP time sync (no RTC, exactly the designed fallback)
- No status LED found (matches gosd-k4w2's research -- no led/gpio-leds
  node in the DTS)

**Benign warnings seen** (not investigated further, not blocking):
- `sdhci-dwcmshc fe2e0000.mmc: Can't reduce the clock below 52MHz in
  HS200/HS400 mode`
- `rockchip-pm-domain ...: Failed to create device link ... for spi2.0`
- Several `/dev/mmcblkXp2` / `/dev/vda2` "Can't lookup blockdev" lines during
  gosd-init candidate-probing (expected -- only one candidate exists per
  board, the others are just other boards' probes)

**Kernel bloat found**: despite the fragment's explicit DRM/video cuts,
GPU/video-codec drivers (rockchip-rga, hantro-vpu, uvcvideo) still showed
up in dmesg -- likely the same "arm64 defconfig promotes =m to =y under
no-modules" trap CLAUDE.md already warns about elsewhere in this fleet.
Not yet cut; worth a follow-up bean since it bloats the image and boot
time for no reason (this board has no display use case in scope).

**Not yet tested**: USB gadget mode (UsbGadgetSupport claims true from DT
alone, never hardware-verified), the data partition (this test build had
none -- `--data-size`/ext4 untested), NVMe-as-storage via the M.2 slot.

**Node placement note**: v2.4 Turing Pi 2 boards may have a USB-routing
limitation flashing RK1/Jetson modules in Node 1/Node 2 specifically
(Turing's own v2.5 changelog) -- not fully ruled out as a contributing
factor here since the firmware upgrade also fixed things; board currently
sits in node 4, untested whether node 1/2 now work post-upgrade.


## Bench session continued (2026-08-30): data partition verified, USB gadget disproven, NVMe inconclusive

**Data partition: both filesystems verified, including persistence across a
real power cycle.**
- FAT32 (`--data-size 256MiB`): mounted read-write, boot counter went
  1 -> 2 across a power-cycle.
- ext4 (`--data-size 512MiB --data-filesystem ext4`): golden image grown to
  fill the partition on first boot exactly as designed, mounted read-write,
  counter went 1 -> 2 across a power-cycle.

**USB gadget mode: confirmed NOT supported, fixed (bean gosd-tqme, PR #374).**
`--usb-gadget` produced an image whose app crashed ("no USB peripheral
controller found under /sys/class/udc") even with the board's USB mux
explicitly routed to device mode before power-on. Root cause confirmed
against the DTS: the OTG-capable port is bound to the host-only xhci-hcd
driver, no dwc3/dwc2 dual-role node exists. `UsbGadgetSupport()` now
returns `Supported: false`; `gosd build --usb-gadget` refuses with an
actionable error.

**NVMe (M.2 slot, node 4): inconclusive, needs a physical check.** Built a
throwaway PCI diagnostic app (dumps `/sys/bus/pci/devices` + attrs over
HTTP) since `examples/diskstorage` just reported "no usable disk attached".
Findings:
- `/sys/bus/pci/devices` shows exactly 3 devices: `0000:00:00.0` (Rockchip
  RK3588 root complex, vendor 0x1d87), `0001:30:00.0` (another Rockchip
  0x1d87/0x3588 device - a bridge), and `0001:31:00.0` -- **vendor 0x1106
  (VIA Technologies), device 0x3483, class 0x0c0330 (xHCI), driver
  xhci_hcd**. That's a VIA USB3 controller chip, NOT an NVMe drive.
  `/sys/class/nvme` and `/dev/nvme*` are both empty.
- Working theory: this board likely has two independent PCIe controllers
  (confirmed at DT level: pcie3x4 the 4-lane one, pcie2x1l1 a 1-lane one).
  The 1-lane one enumerating a VIA USB3 chip matches Turing's own v2.5
  changelog text almost exactly ("Mini PCIe USB interface... now connected
  to the USB1 interface") -- this is plausibly an onboard, board-level PCIe
  path unrelated to the user-facing M.2 NVMe slot, possibly even the same
  path the BMC uses internally for node flashing/MSD. The OTHER (4-lane)
  controller -- the one that should be wired to the M.2 slot -- shows
  *nothing* at all in sysfs, not even an empty root port, which usually
  means its link never came up.
- Asked JP to double check the NVMe drive's physical seating in the M.2
  slot -- can't verify that myself. Not yet resolved as of this bean
  update.

Kernel Kconfig for PCIe/NVMe is confirmed present and correct
(CONFIG_PCI/CONFIG_PCIE_ROCKCHIP_DW_HOST/CONFIG_PHY_ROCKCHIP_SNPS_PCIE3/
CONFIG_BLK_DEV_NVME all =y), so if this turns out to be a real gap once
seating is confirmed, it's a DTS/hardware-integration question, not a
missing-driver one -- mirrors the USB gadget finding's shape.

## NVMe resolved (2026-08-30): the original drive was faulty, not GoSD

JP swapped in a different, known-good wipeable NVMe drive. Same board,
same image, same M.2 slot (node 4) -- full success:

- PCIe link trained fully: 8.0 GT/s x4 (matches the controller's max),
  vs. the original drive's dead link (no downstream device ever answered).
- `/sys/class/nvme` showed `nvme0`, `/dev/nvme0n1` present.
- The real `disk` package end to end, via a throwaway Destructive:true
  variant of examples/diskstorage (the committed example is deliberately
  non-destructive, so a new drive with existing content refuses safely --
  confirmed that refusal firing correctly first): formatted ext4, grew to
  the disk's full size (503941173248 bytes, ~469 GiB, not the 512MiB golden
  seed), mounted at /storage. Power-cycled: boots=1 -> boots=2, filesystem
  size unchanged both times -- the volume was adopted on the second boot,
  not reformatted. Full round-trip proof, same shape as the /data
  persistence tests.

Conclusion: NVMe/PCIe on this board was correct all along -- regulator,
reset/clkreq/wake pinctrl, kernel Kconfig (CONFIG_PCIE_ROCKCHIP_DW_HOST,
CONFIG_PHY_ROCKCHIP_SNPS_PCIE3, CONFIG_BLK_DEV_NVME), and GoSD's `disk`
package all worked correctly from the start. The first drive was
genuinely faulty (or its link-layer negotiation was broken in some way
this board's controller couldn't work around) -- not a bug anywhere in
this codebase. No code changes needed; this closes out the NVMe open
question entirely.

## Boot-time baseline (best effort, per the epic's locked decision)

Power-on (BMC power API call) to app reachable over HTTP: ~17s (includes
curl/DNS/polling-interval overhead, so an upper bound not a precise
figure). The app itself reported uptime=10.9s at first successful
request, meaning roughly 6s of that 17s was bootloader+kernel+gosd-init
before the app process even started. Measured on the `hello` example, no
data partition, node 4, fresh eMMC flash.

## Summary of Changes

This bean's full scope is now verified on real hardware: boot chain
(U-Boot -> kernel -> gosd-init -> app), networking (DHCP/mDNS/NTP), the
data partition (both FAT32 and ext4, with real power-cycle persistence),
NVMe-as-storage via the `disk` package (format/mount/grow/persistent
adoption, once a working drive was used), and a best-effort boot-time
baseline. Two real bugs found and fixed in code (console=ttyS9->ttyS0,
PR #373; UsbGadgetSupport wrongly claiming true, PR #374). USB gadget mode
itself is a confirmed hardware/DTS limitation, not fixable from this repo
alone. One follow-up bean created (kernel bloat cleanup, GPU/video drivers
leaking in). No other Turing RK1 code work is blocked on further hardware
access.
