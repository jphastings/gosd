---
gosd: minor
---

#### `--usb-gadget` works on the Radxa Cubie A5E again

Building with `--usb-gadget` for this board now ships a device tree that
disables the USB-C port's host controllers, so the port stays with the
peripheral controller and can present itself as a USB device. Without it the
host side takes the port during boot and the board can never enumerate,
whatever the device tree's `dr_mode` says — which is why the flag has been
refused for this board since that was found on hardware.

The two roles are mutually exclusive on this hardware, which has no circuitry
to detect which is wanted: an image built with `--usb-gadget` cannot use its
USB-C port as a USB host. The USB 3.0 Type-A port is unaffected.
