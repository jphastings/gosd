---
gosd: patch
---

#### Board images are now built from artifacts v0.10.2

`gosd build` downloads the board kernels and bootloaders published as
v0.10.2, up from v0.10.0, which brings:

- Cubie A5E images now boot the 1GB RAM variant
- The Cubie A5E kernel build now produces a USB-gadget variant device tree
- Cubie A5E U-Boot no longer scans USB on every boot
