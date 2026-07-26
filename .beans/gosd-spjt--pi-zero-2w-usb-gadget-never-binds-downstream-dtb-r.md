---
# gosd-spjt
title: 'pi-zero-2w USB gadget never binds: downstream DTB routes the port to dwc_otg (no UDC)'
status: in-progress
type: bug
created_at: 2026-07-26T08:30:16Z
updated_at: 2026-07-26T09:05:32Z
---

Found during the gosd-4ajn usbwebsite bench test (2026-07-26): a Zero 2W image built with --usb-gadget --data-size never enumerates on a host. Serial shows the app correctly finding GOSD-DATA (/dev/mmcblk0p2) but going straight to serve mode — no UDC exists to attempt gadget mode with. First actual hardware exercise of the 2W's gadget path (v0.3's gadget hardware proof was rock-4se/dwc3); COMPATIBILITY's 2W gadget cell has never been hardware-true.

Root cause (desk-verified): the 2W ships the DOWNSTREAM-style DTB (bcm2710-rpi-zero-2-w.dtb) whose usb node compatible is `brcm,bcm2708-usb` — bound by the downstream host-oriented dwc_otg driver (CONFIG_USB_DWCOTG=y). Mainline dwc2 (CONFIG_USB_DWC2=y, DUAL_ROLE) binds `brcm,bcm2835-usb` and never sees the node → /sys/class/udc empty → gadget impossible. Contrast the Zero W: its MAINLINE-style DTB (bcm2835-rpi-zero-w.dtb) declares `brcm,bcm2835-usb`, so dwc2 should bind there — bench test imminent (the gosd-4ajn checklist's armv6 item may pass on the W before the 2W works).

Fix fork to decide:
(a) Ship the dwc2 overlay: kernel builds already compile overlays/dwc2.dtbo; extend the pi-zero-2w artifact set + boot files to include it and emit `dtoverlay=dwc2` (dr_mode choice below) in config.txt when --usb-gadget is set. Pi firmware applies overlays natively — the project's "no runtime overlays" rule is Rockchip-specific (pinned U-Boots lack OF_LIBFDT_OVERLAY) and does not apply to Pi's start.elf. Most flexible; a bigger artifact/assembly change.
(b) Kernelspec DTS patch swapping the usb node to `brcm,bcm2835-usb` (+ explicit dr_mode) — smaller mechanism, already proven by gosd-1ey5's patch, but bakes the choice into the DTB for all images of the board.
Either way: dr_mode question (peripheral = deterministic gadget, loses any future USB-host use of the port; otg + dual-role = role detection, more moving parts on a port whose ID pin behaviour on the 2W's data port needs checking). And either way the change reaches real builds via the artifacts-release batch (gosd-36yy / gosd-7wv9 window).

COMPATIBILITY.md: the 2W gadget cell needs an honest footnote with whichever PR lands first. usbwebsite itself needs NO changes — its degrade-to-serve behaviour did exactly the right thing and made the diagnosis trivial.

## Bench findings (2026-07-26, hardware-verified on the Zero W)

1. **The overlay is emitted but never shipped.** `--usb-gadget` already renders `dtoverlay=dwc2,dr_mode=peripheral` into both Pi zeros' config.txt (covered in board_test.go) — but nothing puts `overlays/dwc2.dtbo` on the boot partition, so start.elf skips the directive silently. Proven on the bench: hand-placing dwc2.dtbo (raspberrypi/firmware @ 09267f5354d40519d82fbd2193b9e211ec304055, boot/overlays/dwc2.dtbo, 801 bytes) onto a Zero W card produced firmware log `Loaded overlay 'dwc2'` — the overlay applies even to the mainline-style bcm2835 DTB (symbols present) — and dwc2 bound with a UDC appearing. This resolves the fix fork above in favour of (a), and it applies to BOTH Pi zeros: the overlay both fixes the 2W's compatible-string routing and provides the explicit dr_mode both boards need.
2. **New blocker behind it: the legacy gadget zoo claims the UDC first.** The host enumerated "Gadget Zero" (0x0525/0xa4a0) — the kernel's built-in LEGACY test gadget bound the UDC before the app could apply its configfs gadget. pi-zero-w's kernel.config carries the whole drivers/usb/gadget/legacy family built-in (defconfig `=m` promoted to `=y` by ModulesDisabled — the third instance of this trap, after mac80211_hwsim ×2): USB_ZERO, USB_ETH(+RNDIS), USB_GADGETFS, USB_G_SERIAL, USB_G_PRINTER, USB_CDC_COMPOSITE, USB_G_ACM_MS, USB_G_MULTI(+RNDIS), USB_G_HID. Checked pi-zero-2w's recorded kernel.config: the IDENTICAL zoo is =y there. pi-3b has no recorded kernel.config yet (never built) and its fragment already cuts CONFIG_USB_GADGET outright, so the zoo is structurally dead there — fragment lines added anyway for hygiene/consistency.
3. **usbwebsite needs no changes** — its degrade-to-serve behaviour was correct throughout and made both diagnoses trivial.

Gadget identity check (report only, no code change): the `gadget/` library sets no default VendorID/ProductID — the zero value would write 0x0000/0x0000; identity is entirely caller-supplied. The examples pass Linux sample IDs: examples/usbwebsite (mass storage — what rock-4se enumerated as during gosd-sz6p) uses 0x1d6b/0x0104 ("Linux Foundation Multifunction Composite Gadget"), examples/usbserial uses 0x0525/0xa4a7 (NetChip g_serial sample). Fine for examples, but a real identity story (e.g. pid.codes allocation, or a documented "bring your own IDs" stance in the gadget docs) is a follow-up bean — not changed here, gadget/ is a public v0.3 API surface.

## Bench checklist

- [ ] Rebuild the pi-zero-w kernel locally (`gosd build-kernel --board pi-zero-w`) with the zoo-evicting fragment; flash with `--usb-gadget --data-size`; the gadget presents GOSD-DATA as mass storage on a host (no more "Gadget Zero")
- [ ] Files written to that volume from the host appear via usbwebsite's HTTP serve mode after replug
- [ ] Same flow on pi-zero-2w: its kernel also needs the zoo-evicted rebuild, riding the SAME artifacts release batch (gosd-36yy / gosd-7wv9 window) — the shipped overlay (this PR's manifest change) is active in any build from this commit and needs no artifact release

## Summary of Changes

Ship the dwc2 overlay (fix fork (a), locked by bench finding 1):
- `build/boards/pi-zero-w/manifest.json` and `build/boards/pi-zero-2w/manifest.json`: new `overlays` file group pinning `dwc2.dtbo` from raspberrypi/firmware at the same commit the bootFiles group already pins (09267f5354d40519d82fbd2193b9e211ec304055), sha256 3e6f8e33e5749bcb2a5c7808b48123e65e88fc465d64ec16b74d50ffe5037e34 (computed from a fresh download), destDir `overlays`. Both boards' `manifest.go` gained the `Overlays` field.
- `internal/boards/pizerow` and `internal/boards/pizero2w`: `Artifacts()` now lists the overlay (pinned URL fetch, cached like the GPU firmware); `BootFiles()` copies it to `overlays/dwc2.dtbo` on the FAT boot partition ONLY when `BuildConfig.UsbGadget` is set, matching the config.txt `dtoverlay` line's conditionality. `internal/image` already creates boot-partition subdirectories (the Rockchip boards' `extlinux/extlinux.conf` exercises that path), so no image-writer change was needed.
- Tests: both boards' board_tests assert the dtbo artifact is pinned and that BootFiles ships `overlays/dwc2.dtbo` with the gadget flag and omits it without; `cmd/gosd/build_integration_test.go` gained a network-tripwired `--usb-gadget` build asserting the overlay lands at `overlays/dwc2.dtbo` with the fixture content (fake dwc2.dtbo added to `cmd/gosd/testdata/fake-artifacts/`), and the existing no-gadget pi-zero-2w test asserts the file is absent.

Evict the legacy gadget zoo (bench finding 2):
- `build/boards/pi-zero-w/kernel.fragment`, `build/boards/pi-zero-2w/kernel.fragment`, `build/boards/pi-3b/kernel.fragment`: `# CONFIG_... is not set` lines for USB_ZERO, USB_ETH, USB_GADGETFS, USB_G_SERIAL, USB_G_PRINTER, USB_CDC_COMPOSITE, USB_G_ACM_MS, USB_G_MULTI, USB_G_HID (the full set observed =y in the recorded configs; USB_MASS_STORAGE was already cut by gosd-z9l4), with why-comments citing this bean in each fragment's style. LIBCOMPOSITE/configfs function drivers untouched — the gadget/ library needs them. Fragment "is not set" lines don't appear in kernelspec's RequiredY derivation, so `internal/kernelspec` and its drift tests needed no changes (verified against TestPiRequiredYIsDerivedFromFragment's derivation rule). Recorded kernel.config files left alone per the gosd-z9l4 precedent (they regenerate at the next real `gosd build-kernel` run); internal/artifacts.Version NOT bumped (tag-first rule).

COMPATIBILITY.md: honest footnote on both Pi zeros' USB gadget cells covering the never-shipped overlay, the Gadget Zero claim, and what reaches users when.
