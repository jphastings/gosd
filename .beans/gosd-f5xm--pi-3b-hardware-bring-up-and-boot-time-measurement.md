---
# gosd-f5xm
title: 'Pi 3B: hardware bring-up and boot-time measurement'
status: todo
type: task
priority: normal
created_at: 2026-07-25T23:22:33Z
updated_at: 2026-07-25T23:22:42Z
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
