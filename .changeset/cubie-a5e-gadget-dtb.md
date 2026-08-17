---
artifacts: minor
---

#### The Cubie A5E kernel build now produces a USB-gadget variant device tree

Alongside the board's stock device tree, the build emits one with the
`ehci0`/`ohci0` host controllers disabled, so the USB-C port's phy stays with
the peripheral controller and the board can enumerate as a USB device. The
two are mutually exclusive on this hardware — it has no detection circuitry
to arbitrate between host and peripheral — so the choice is made when an
image is built rather than when it is plugged in.
