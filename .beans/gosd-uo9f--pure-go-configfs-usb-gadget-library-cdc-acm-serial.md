---
# gosd-uo9f
title: Pure-Go configfs USB gadget library + CDC-ACM serial gadget
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-06T08:45:20Z
parent: gosd-jge2
blocked_by:
    - gosd-m9dj
    - gosd-nlzf
---

Headline feature: a GoSD app can present the device as USB hardware. Start with serial (CDC-ACM).

Design first (this API is public, take care): package `gadget` in the gosd module. Declarative: define a Gadget{VendorID, ProductID, Manufacturer, Product, Serial, Functions []Function}, call Apply() → it materializes the configfs tree under /sys/kernel/config/usb_gadget/gosd/, links functions into config c.1, and binds the UDC (first entry in /sys/class/udc). Close() unbinds and tears down. configfs is all file writes — no cgo, no exec.

Plumbing prerequisites: gosd-init mounts configfs at /sys/kernel/config; Pi needs `dtoverlay=dwc2,dr_mode=peripheral` in config.txt — make this a builder flag (`--usb-gadget`) that board profiles translate (Pi: overlay line; Radxa: none needed, the OTG port is the power port and dwc3 handles role)

- [x] gadget package: core tree builder + ACM function; unit tests against a fake filesystem (io/fs abstraction over the configfs writes)
- [x] examples/usbserial: echoes lines over /dev/ttyGS0
- [ ] Hardware test both boards: plug the OTG port into a laptop, get /dev/ttyACM0 (macOS: /dev/cu.usbmodem*), record results here — note the Pi Zero 2W data port is the inner micro-USB (USB, not PWR); Radxa is the USB-C OTG/power port

## Acceptance
Both boards enumerate as a serial device on macOS and Linux hosts; echo works; teardown/re-Apply works without reboot.


**Note on `blocked_by`:** the software portion of this bean (package +
example) was deliberately started ahead of `gosd-m9dj`/`gosd-nlzf`, both of
which are themselves blocked on acquiring a hardware kit (`gosd-s4t4`) and
so are not close to resolving. It's fully verifiable against a fake
filesystem per this bean's own plan — only the "Hardware test both boards"
item genuinely needs those to resolve first, and stays unchecked below.

## Summary of Changes

- Added the `gadget` package (`gadget/gadget.go`, `fs.go`, `acm.go`):
  a declarative `Gadget{VendorID, ProductID, Manufacturer, Product, Serial,
  Functions}` whose `Apply()`/`Close()` materialize/tear down the configfs
  gadget tree via a small writable-filesystem seam (`writableFS`, real
  `osFS` vs. an in-memory fake), with `ACM` as the first `Function`.
  `gadget_test.go`/`fakes_test.go` cover identity/function writes, UDC
  binding (including "no controller found"), the double-Apply guard, and
  that `Close()` fully unwinds what `Apply()` created (including a
  Close-then-re-Apply round trip) — the fake's `Remove` enforces real
  configfs rmdir semantics (fails on non-empty child dirs/symlinks, but
  lets attribute files vanish for free), which caught a real ordering bug
  in `Close()` during development (missing intermediate directory removals).
- Added `examples/usbserial`, applying an ACM gadget and echoing lines over
  `/dev/ttyGS0`.
- `cmd/gosd-init/internal/boot/mounts.go`: gosd-init now mounts configfs at
  `/sys/kernel/config` unconditionally as part of its early mounts.
- Added a `--usb-gadget` flag to `gosd build` threaded through
  `boards.BuildConfig`; the Pi Zero 2W board profile adds
  `dtoverlay=dwc2,dr_mode=peripheral` to `config.txt` when set (additive to
  the gosd-eu2x-locked default content, verified byte-identical when unset);
  Radxa Zero 3E ignores it (dwc3 negotiates role automatically), with a
  regression test asserting its boot files are unaffected either way.
- CI: `examples/usbserial` is now cross-compiled alongside the other
  arm64 targets.
- Documented USB gadget mode in `docs/runtime.md`.

Not done: the "Hardware test both boards" checklist item, which needs real
hardware (see the note above).
