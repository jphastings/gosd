# usbwebsite — a static website you edit over USB

A GoSD example that turns a board into a tiny website appliance:

- **Powered on its own**, it serves its storage volume's contents as a static
  website over HTTP on port 80.
- **Plugged into a computer**, it presents that same volume as a removable
  USB drive, so you can drag your HTML/CSS/images straight on, eject, and
  power it standalone again to serve them.

The volume is the **onboard eMMC** on boards with one fitted (a drive
labelled `WEBSITE`), and otherwise the **SD card's data partition**
(a drive labelled `usbweb-data` by default — derived from this app's name,
see `gosd build --label-prefix` — and carrying gosd-init's `.gosd-data`
marker file) — which is how eMMC-less boards like the Raspberry Pi Zero W and
Zero 2W work. The SD partition is created pre-formatted by
`gosd build --data-size`, so the app never formats anything on that path; it
only mounts the partition to serve, or unmounts it to hand the raw device to
a connected computer.

It's the worked example for [`gadget.MassStorage`](../../gadget), built on
the [`emmc`](../../emmc) package (`emmc.FormatAndMount` formats the eMMC on
first boot and hands back the block device behind the mount) and on
gosd-init's data-partition auto-mount at `/data`.

## What it demonstrates

- **Expose *or* mount, never both.** A mass-storage LUN and a local mount of
  the same device must not be live at once — the host writes raw blocks with
  no idea of our filesystem. The app decides once per boot which one to be,
  releasing its own mount (`emmc.Unmount`, or unmounting `/data`) before the
  gadget takes the device.
- **Detecting a connected computer.** After presenting the drive it watches
  the USB controller state (`/sys/class/udc/<udc>/state`): a real computer
  enumerates and *configures* the gadget within a second; a plain USB power
  supply never does, so the app falls back to serving.
- **Storage fallback.** eMMC is preferred when fitted; `errors.Is(err,
  emmc.ErrNoEMMC)` falls through to the SD card's data partition, found via
  the mount table (`/data`'s device, or partition 2 of the disk `/boot`
  mounted from).

## Boards

Needs a board with a USB gadget controller (see `COMPATIBILITY.md`'s USB
gadget row) and **either** onboard eMMC fitted **or** an image built with
`--data-size` (eMMC is a build-to-order option on some boards, so having the
right board model isn't the same as having eMMC fitted). With neither volume
it logs what to do and idles rather than exiting; with no gadget controller
it just serves.

### A board whose eMMC already holds other content

Real hardware often ships with something already on the eMMC — vendor
firmware, a prior project. `usbwebsite` refuses to touch that without
explicit consent: set `WEBSITE_WIPE_EMMC = "yes"` in the `[env]` table of
`gosd.toml` on the boot partition (see docs/runtime.md's "App
environment variables"), then reboot. Without it, the app logs what to do
and idles rather than exiting, since `gosd-init` restarts exited apps
regardless of exit code and would otherwise crash-loop it forever.

The SD path needs no consent knob: the data partition exists solely
for app data, and the app never reformats or relabels it.

## Build & run

```sh
# Pi Zero 2W / Pi Zero W (no eMMC): give the website SD-card space.
# --usb-gadget puts the board's USB port in peripheral mode.
gosd build ./examples/usbwebsite --board pi-zero-2w --usb-gadget --data-size 256MiB -o usbwebsite.img

# Boards with onboard eMMC fitted (e.g. a Radxa Zero 3E built to order with
# it): no data partition needed — the eMMC is preferred when present.
gosd build ./examples/usbwebsite --board radxa-zero-3e --usb-gadget -o usbwebsite.img
```

`--data-size` works on any board, so it also serves as the fallback on an
eMMC-capable board whose eMMC isn't fitted.

Flash `usbwebsite.img` (see `docs/flashing.md`) and provision WiFi as usual.

- **To add content:** connect the board to a computer with a USB cable. A
  drive named `WEBSITE` (eMMC) or `usbweb-data` (SD, by default) appears;
  drop your site's files on it (an `index.html` at the top level is the home
  page), then eject it.
- **To serve:** power the board on its own (a wall charger, or a power-only
  input) and browse to `http://<hostname>.local` — the default hostname is
  `usbwebsite` unless you override it. A brand-new board serves a starter
  page explaining these same steps.

## Power topology note

The board decides it's "plugged into a computer" only when a USB *host*
configures the drive. If you power the board through the same port a computer
would use, plugging into a computer → drive mode, and a dumb charger → website
mode. Boards with a separate power input (so the gadget port is free) behave
the most predictably — note the Pi Zeros' PWR-marked micro-USB is power-only,
so their *inner* (data) port is the gadget port. This example has not been
run on hardware yet (no board has completed a gadget mass-storage bring-up —
see `COMPATIBILITY.md`), so treat the USB state-machine behaviour as
code-complete, not verified.
