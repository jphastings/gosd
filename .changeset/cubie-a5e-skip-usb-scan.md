---
artifacts: patch
---

#### Cubie A5E U-Boot no longer scans USB on every boot

The Radxa Cubie A5E's U-Boot ran an unconditional `usb start` scan before
every boot, purely to detect a USB keyboard that could interrupt autoboot —
on hardware this cost roughly 4.5 seconds of the board's ~9 second U-Boot
phase, scanning four controllers to find nothing. gosd images boot from the
SD card via extlinux and never from USB, so this fragment disables the
preboot scan while leaving USB host, storage and gadget support otherwise
untouched. The boot-time saving has not yet been re-measured on hardware;
see bean `gosd-uj4l`.
