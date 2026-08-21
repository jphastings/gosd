---
# gosd-jge2
title: 'Peripherals: GPIO, USB OTG gadget, persistence'
status: completed
type: epic
priority: normal
created_at: 2026-07-02T20:50:27Z
updated_at: 2026-08-21T03:20:37Z
parent: gosd-p3zw
---

The 'Go hardware application' capabilities: GPIO/I2C/SPI access from the user app, USB OTG gadget modes (device presents as USB serial / USB Ethernet), and a writable data partition.

Locked decisions: GPIO via the character device (/dev/gpiochipN) using github.com/warthog618/go-gpiocdev — never sysfs. I2C/SPI via periph.io or direct /dev nodes; document, don't wrap. USB gadget via configfs + libcomposite (kernel configs already enabled by the v0.1 kernel tasks: dwc2 on Pi, dwc3 on RK3566); we ship a small pure-Go configfs gadget library as part of the gosd runtime package — this is a headline feature, design its API carefully.


## Summary of Changes

Closed as the v0.3 epic it was. Ten of its fifteen children delivered inside
v0.3; the five that did not were re-parented to a v0.7 epic (gosd-q6g6) and
are not claimed below. The epic is closed here rather than moved, so the ten
that shipped stay attributed to the release that shipped them.

**Shipped**

- *Persistence.* A writable /data partition surviving reboot and
  reflash-of-app (gosd-xelb), on top of the on-device FAT32 format spike that
  made it possible (gosd-0s0m). Onboard eMMC format-and-mount (gosd-tdcc,
  after gosd-899s ruled it viable without on-device formatting), the generic
  `disk` package (gosd-yggd), and exFAT within it (gosd-1ici).
- *USB gadget.* The pure-Go configfs gadget library and CDC-ACM serial
  function (gosd-uo9f), the mass-storage function (gosd-k2fs), and the
  eMMC-website-over-USB example that exercises both together (gosd-f1je).
- *A/B updates, scoped.* The spike (gosd-v2w1) produced the merged A/B design
  doc, which is what v0.3 actually required of it.
- *GPIO, I2C and SPI.* Enabled by default and documented on every board — not
  just the two the epic named — with worked examples (examples/i2cscan,
  examples/spiloopback, examples/gpioinfo), per-board pin tables in the
  runtime docs, and the two artifact releases the Rockchip DTS patches needed
  (gosd-xshg for artifacts/v0.3.0, gosd-jphp for artifacts/v0.4.0), each
  verified against the published DTBs.

**Did not ship — now v0.7, under gosd-q6g6**

- USB Ethernet gadget (ECM + RNDIS) with a built-in DHCP server (gosd-30jz):
  unstarted. This epic's own scope line — "device presents as USB serial /
  USB Ethernet" — is therefore only half delivered, and that is the single
  largest thing v0.3 promised and did not land.
- Hardware verification of I2C, SPI and GPIO (gosd-85pt, gosd-fnza,
  gosd-nyad). All three are code-complete with exactly one deliberately
  unchecked bench todo each; none of the three has been proven against real
  silicon.
- gosd-rsrd, the superseded umbrella bean for the per-bus work, which closes
  when those three do.
