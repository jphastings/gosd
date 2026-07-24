---
# gosd-5pnr
title: gosd build --usb-gadget silently accepted for boards that cannot gadget
status: todo
type: bug
created_at: 2026-07-24T07:19:25Z
updated_at: 2026-07-24T07:19:25Z
---

Found during NanoPi Zero2 bring-up (gosd-odp7, 2026-07-24): 'gosd build ./examples/usbwebsite --board nanopi-zero2 --usb-gadget' succeeds and produces an image, but nanopi-zero2 is marked ❌ for USB gadget in COMPATIBILITY.md ([^nanopi-usb]: the RK3528 has no USB controller DT node at the pinned kernel tag; arrives with the fleet kernel bump, bean gosd-vcae). At runtime the app finds /sys/class/udc empty and can only log 'build with gosd build --usb-gadget' — advice the user already followed, which is confusing.

Fix: gosd build should fail (or at minimum warn loudly) when --usb-gadget is passed for a board whose profile doesn't support gadget mode at the pinned artifacts, with an actionable message naming the board, the reason, and where it's tracked — errors-must-be-actionable convention. The board profile presumably already knows (COMPATIBILITY row / rock-4se's dr_mode patch vs nanopi's absence); surface that knowledge at build time. Behavioral test: build with --usb-gadget for a non-gadget board asserts the actionable error; gadget-capable boards unaffected.
