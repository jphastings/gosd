---
gosd: patch
---

#### `--usb-gadget` now refuses for the Radxa Cubie A5E instead of building an image that can't work

Hardware testing showed the Cubie A5E cannot present itself as a USB device
at the currently pinned board artifacts: the USB-C port's host controllers
share a phy with the peripheral controller, and with nothing on the board to
arbitrate between them the host side wins every boot, so the port never
enumerates. The board's device tree pins peripheral mode, which is what
GoSD's earlier support claim was based on, but that is not enough on its own.

Building with `--usb-gadget` for this board now fails with an explanation
rather than producing an image that looks correct and cannot work. Support
returns once a board artifacts release carries the variant device tree that
disables those host controllers — at which point USB-C host mode becomes the
trade-off, since an image can serve one role or the other but not both. The
USB 3.0 Type-A port is unaffected.
