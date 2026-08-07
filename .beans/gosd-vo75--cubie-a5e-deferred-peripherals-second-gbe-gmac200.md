---
# gosd-vo75
title: 'cubie-a5e deferred peripherals: second GbE (GMAC200) and header I2C'
status: todo
type: feature
priority: low
created_at: 2026-08-07T19:04:27Z
updated_at: 2026-08-07T19:04:27Z
parent: gosd-h1wv
---

Both out of scope at fleet tag v6.18.37 (bean gosd-jpc8's research): the second Ethernet port's GMAC200 driver/DT landed upstream after v6.18, and header SPI has no controller nodes in sun55i-a523.dtsi at all; header I2C nodes exist but enabling them follows the kernel-build DTS-patch convention and was deferred to keep the board's first pass to verified ground. Unlock path: the next FLEET kernel tag bump (see gosd-36yy for the bump's other driver: RK3528 USB gadget) — when that happens, re-survey what the new tag gives cubie-a5e (gmac1 node + driver? spi nodes?) and enable: second GbE in-tree if driver present, header I2C via a status=okay DTS patch, header SPI + spidev if nodes exist by then. COMPATIBILITY.md rows updated with whatever lands.
