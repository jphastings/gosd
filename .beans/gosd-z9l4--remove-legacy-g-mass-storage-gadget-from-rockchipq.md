---
# gosd-z9l4
title: Remove legacy g_mass_storage gadget from Rockchip/qemu kernel configs
status: todo
type: bug
priority: normal
created_at: 2026-07-23T11:56:48Z
updated_at: 2026-07-23T14:28:31Z
---

Found during first real-hardware boot (gosd-sz6p, rock-4se, 2026-07-23): kernel logs show g_mass_storage probing at boot and failing — 'no file given for LUN0', 'udc fe800000.usb: failed to start g_mass_storage: -22'.

Cause: CONFIG_USB_MASS_STORAGE=y (the LEGACY mass-storage gadget, drivers/usb/gadget/legacy/) is built into four boards' kernels: rock-4se, nanopi-zero2, radxa-zero-3e, qemu-virt (the Pi boards don't have it). Built-in legacy gadgets auto-bind the UDC at boot; besides the boot noise this can claim the UDC and conflict with the configfs-based gadget stack (gadget/ library) that GoSD actually uses — we already carry CONFIG_USB_CONFIGFS_MASS_STORAGE=y for that path.

Fix: unset CONFIG_USB_MASS_STORAGE in all four kernel configs (via kernelspec fragments as appropriate). This is a compiled-artifact change: fleet kernel rebuild + the artifacts tag-first release dance (docs/artifacts.md) — do NOT bump internal/artifacts.Version in the same PR.

Verify: boot log free of g_mass_storage errors on rock-4se serial + qemu boot-to-HTTP CI; USB gadget examples (usbserial/usbwebsite) still enumerate.

Also fold into this PR (it already rebuilds the rock-4se kernel artifacts): update build/boards/rock-4se/kernel/patches/0003-usb-dwc3-peripheral.patch's 'WHICH PHYSICAL PORT (UNRESOLVED, best guess)' comment — hardware-verified 2026-07-23 (gosd-sz6p): usbdrd_dwc3_0 IS the top/upper blue USB 3.0 port; CDC-ACM gadget enumerated and echoed on macOS. The unmarked OTG switch's away-from-Ethernet position = device mode. Comment-only change to the patch (DTB output unchanged), safe to ship with the config fix.
