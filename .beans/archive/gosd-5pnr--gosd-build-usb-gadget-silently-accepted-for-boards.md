---
# gosd-5pnr
title: gosd build --usb-gadget silently accepted for boards that cannot gadget
status: completed
type: bug
priority: normal
created_at: 2026-07-24T07:19:25Z
updated_at: 2026-07-24T07:49:27Z
---

Found during NanoPi Zero2 bring-up (gosd-odp7, 2026-07-24): 'gosd build ./examples/usbwebsite --board nanopi-zero2 --usb-gadget' succeeds and produces an image, but nanopi-zero2 is marked ❌ for USB gadget in COMPATIBILITY.md ([^nanopi-usb]: the RK3528 has no USB controller DT node at the pinned kernel tag; arrives with the fleet kernel bump, bean gosd-vcae). At runtime the app finds /sys/class/udc empty and can only log 'build with gosd build --usb-gadget' — advice the user already followed, which is confusing.

Fix: gosd build should fail (or at minimum warn loudly) when --usb-gadget is passed for a board whose profile doesn't support gadget mode at the pinned artifacts, with an actionable message naming the board, the reason, and where it's tracked — errors-must-be-actionable convention. The board profile presumably already knows (COMPATIBILITY row / rock-4se's dr_mode patch vs nanopi's absence); surface that knowledge at build time. Behavioral test: build with --usb-gadget for a non-gadget board asserts the actionable error; gadget-capable boards unaffected.

## Summary of Changes

Added `boards.GadgetSupport{Supported bool; Reason string}` and a new
`UsbGadgetSupport() GadgetSupport` method on the `boards.Board` interface
(implemented by all six boards, alongside `Arch`/`Artifacts`/etc.) so each
board profile declares, as a static fact, whether it has a USB peripheral
controller at its pinned artifacts. pi-zero-2w, pi-zero-w, radxa-zero-3e, and
rock-4se return Supported: true; nanopi-zero2 returns false with a reason
citing the missing RK3528 DT node and bean gosd-vcae; qemu-virt (internal
only) also returns false, since its fixed qemu invocation attaches no USB
device model.

`cmd/gosd/build.go` gained `validateUsbGadget(selected, usbGadget)`, called
right after board resolution in runBuild: a no-op unless --usb-gadget is set
and at least one selected board is incapable, otherwise it fails with an
actionable error naming the incapable board(s), their reason,
COMPATIBILITY.md, and — when other selected boards do support it — a
--board restriction suggestion. Verified the no---board (all public boards)
case correctly lists only nanopi-zero2.

Updated nanopizero2/board.go BootFiles comment to point at
UsbGadgetSupport as the actual gate; added a COMPATIBILITY.md footnote
sentence and a docs/runtime.md bullet noting the new build-time failure.
No board boot-file/kernel/DT behavior changed.
