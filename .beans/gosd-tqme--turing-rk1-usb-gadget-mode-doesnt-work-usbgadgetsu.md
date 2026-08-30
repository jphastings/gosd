---
# gosd-tqme
title: 'Turing RK1: USB gadget mode doesn''t work — UsbGadgetSupport wrongly claims true'
status: completed
type: bug
priority: normal
created_at: 2026-08-30T08:27:33Z
updated_at: 2026-08-30T08:30:08Z
parent: gosd-bntd
blocked_by:
    - gosd-jvtg
---

Hardware bring-up (bean gosd-hycf) found gosd build --usb-gadget produces an image whose app crashes: 'applying USB gadget failed: gadget: no USB peripheral controller found under /sys/class/udc'. Confirmed not a cable/mux-timing issue: routed the BMC's USB mux to device mode for this node (tpi usb device equivalent) before power-on, same result on a fresh boot.

Root cause, confirmed against the actual mainline DTS (rk3588-turing-rk1.dtsi at the pinned v6.18.37 tag): the port the DTS itself comments '/* USB 0: USB 2.0 only, OTG-capable */' is bound to &usb_host0_xhci, an XHCI node -- Linux's xhci-hcd driver is HOST-ONLY by design (XHCI is a host controller interface spec with no gadget-mode implementation at all), regardless of dr_mode. No dr_mode is even set on this node (unlike usb_host1_xhci, which explicitly pins dr_mode = "host"). So while the PHY/hardware genuinely is OTG-capable, the mainline DTS's chosen driver binding for it is host-only. There is no dwc3/dwc2-style dual-role controller node anywhere in this DTS.

Fix: internal/boards/turingrk1/board.go's UsbGadgetSupport() -> Supported: false, matching this project's established rule (see CLAUDE.md/COMPATIBILITY.md history -- gosd build --usb-gadget silently accepted for boards that cannot gadget was itself a bug class this project guards against). Update the epic's locked decisions and gosd-k4w2's research finding accordingly (DT-only research called this 'a candidate,' hardware disproves it).

Open question for a future bean, NOT blocking this fix: is gadget mode achievable at all on this hardware via a DTS patch (a different mainline binding, if the RK3588 IP actually supports one under a gadget-capable driver), or is this a genuine SoC-integration-level host-only limitation? Needs real RK3588 TRM/driver research before attempting any patch -- don't guess.

## Summary of Changes

internal/boards/turingrk1/board.go: UsbGadgetSupport() -> Supported: false,
with a Reason naming the missing dual-role controller node. Updated the
kernel-fragment.config comment (the Kconfig lines themselves left as-is --
harmless dead weight, untangling them from genuinely-needed host-mode USB
config is the kernel-bloat cleanup bean's job, not this one), and corrected
BootFiles' now-wrong comment about ignoring cfg.UsbGadget. Verified: `gosd
build --usb-gadget` now refuses with an actionable error naming the real
cause, instead of silently producing an image whose app crashes at
runtime. Updated the epic (gosd-bntd) and research bean (gosd-k4w2) per the
"stop and say so" rule.
