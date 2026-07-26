---
# gosd-f5xm
title: 'Pi 3B: hardware bring-up and boot-time measurement'
status: in-progress
type: task
priority: normal
created_at: 2026-07-25T23:22:33Z
updated_at: 2026-07-26T09:51:15Z
parent: gosd-xhc3
blocked_by:
    - gosd-7wv9
---

Bench bring-up on a real Raspberry Pi 3B (gosd-sz6p/gosd-m9dj pattern; use
the sdwire rig). Stock `gosd build` image from the published release — no
--artifacts-dir, closing the loop on the real download path.

## Checklist

- [ ] Flash + first boot: GPU firmware loads kernel8.img + bcm2710-rpi-3-b.dtb
      (DTB present on the boot partition — gosd-f59k class of bug is
      integration-tested but verify on bench)
- [ ] Serial console: readable boot log on GPIO14/15 at 115200 (mini-UART;
      this is the RUNTIME_UARTS=1 fix's bench proof — record /proc/cmdline
      to see whether firmware injected 8250.nr_uarts anyway, evidence
      gosd-md4w asked for)
- [ ] **Ethernet (headline)**: link up, DHCPv4 lease, app reachable over
      wired — first GoSD Pi with onboard Ethernet, exercises netup's eth*
      path on a Pi for the first time
- [ ] WiFi: WPA2-PSK join with the Cypress 43430 blobs; hello.local resolves
      (mDNS) on both interfaces; note any brcmfmac firmware-load errors
- [ ] gosd.toml fallback edit cycle works (FAT edit loop per the bring-up
      memory notes)
- [ ] I2C/SPI/GPIO enumerate (/dev/i2c-1, /dev/spidev0.0+0.1, gpiochip;
      dtparam path — no artifacts dependency)
- [ ] Boot-time measurement: power-on → /app baseline recorded (best effort,
      dedicated optimization is out of scope — epic convention)
- [ ] COMPATIBILITY.md: flip code-complete caveats to hardware-verified where
      proven, in the same PR as this bean's summary

## Notes

- USB gadget: nothing to test — not possible on this board (hub-wired), the
  build refuses --usb-gadget.
- Boot files/WiFi-state diagnostics from the other Pi bring-ups apply
  (memory: pi-bringup-fat-partition-tricks; gosd-anyp WiFi-drop watch item).

## Pre-release session record: 2026-07-26 maiden boot (kept in-progress — the full checklist above still runs post-activation on the published release)

Not the checklist run: this was an early hardware session with a locally-built kernel (`gosd build-kernel --board=pi-3b` + `--artifacts-dir`), before any pi-3b artifacts release exists. Findings:

- **First pi-3b-profile boot on hardware: full chain to hello.local HTTP 200 over WIRED Ethernet** — the family's headline feature, working first try. The Ethernet checklist item's driver path is bench-proven early (on lan78xx, see below); it still re-runs against the published release.
- **The bench board is a 3B+** (rev a020d3, "RPI3BP" silkscreen), not a 3B: firmware requested `bcm2710-rpi-3-b-plus.dtb` first, fell back to our shipped `bcm2710-rpi-3-b.dtb`, and booted fine on the fallback. JP's locked decision follows: pi-3b covers the whole family in one image (bean gosd-oq0z ships both DTBs — post-activation, verify the firmware loads the -plus blob directly, no fallback line in the firmware log).
- **Ethernet came up via lan78xx** (the 3B+'s LAN7515 GbE), not smsc95xx (the 3B's LAN9514) — it was enabled only by defconfig luck until gosd-oq0z asserted CONFIG_USB_LAN78XX=y in the fragment. Both chips self-enumerate on USB (DTB-agnostic), which is why the wrong-model DTB still had working Ethernet.
- **WiFi watch item for the checklist**: this bench board's radio is the 3B+'s BCM43455, which our manifest's 43430 blob set does not cover — expect the WiFi item to need the 43455 blobs + `raspberrypi,3-model-b-plus` aliases (recorded in gosd-oq0z as an epic-level follow-up), or a real 3B on the bench.
