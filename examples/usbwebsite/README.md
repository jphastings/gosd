# usbwebsite — a static website you edit over USB

A GoSD example that turns a board into a tiny website appliance:

- **Powered on its own**, it serves the site's files as a static website over
  HTTP on port 80.
- **Plugged into a computer**, it can present the volume holding them as a
  removable USB drive, so you can drag your HTML/CSS/images straight on,
  eject, and power it standalone again to serve them.

The volume is the **onboard eMMC** on boards with one fitted (a drive
labelled `WEBSITE`), and otherwise the **SD card's data partition** (labelled
`usbweb-data` by default — derived from this app's name, see
`gosd build --label-prefix`) — which is how eMMC-less boards like the
Raspberry Pi Zero W and Zero 2W work. The SD partition is created
pre-formatted by `gosd build --data-size`, so the app never formats anything
on that path.

It's the worked example for [`gadget.MassStorage`](../../gadget), built on
the [`emmc`](../../emmc) package (`emmc.FormatAndMount` formats the eMMC on
first boot and hands back the block device behind the mount) and on
gosd-init's data-partition auto-mount at `/data`.

## Only publish a volume that is yours alone

This is the part worth copying, and the part that is easy to get wrong: this
app publishes files two ways, and **both are scoped to a directory it owns**
rather than to whatever the volume happens to hold.

The two volumes are not equivalent, and the difference drives everything
below:

- **The eMMC is this app's alone.** It formats it, and nothing else on the
  device ever writes to it. Everything on it is the website.
- **The SD card's data partition is shared with `gosd-init`**, which keeps
  this device's own settings there so that re-flashing the card doesn't lose
  them (see [how settings survive a re-flash](../../docs/design/upgrade-path.md)).
  That includes, in plain text, the passphrase of the WiFi network the board
  is sitting on, any ingress token, and the Tailscale node's private key.

### Serving: a folder, never the mount point

`http.FileServer` has no notion of a hidden file: a leading dot means nothing
to it, so `http.Dir("/data")` will happily serve
`/.gosd/config/values/wifi/passphrase` to anyone who can reach port 80. On
the SD-card path the site therefore lives in a `website` folder of its own,
and only that folder is served. On the eMMC there is nothing else on the
volume, so its root is the site.

Point a file server at a directory your app owns. Never at a mount point
something else also writes to.

### Sharing over USB: the whole volume, or nothing

A mass-storage LUN is a **block device**. There is no way to share a folder
of one — the host gets the volume, every file on it, and (unless you set
`ReadOnly`) may write to all of it. So "share a subdirectory" is not an
option the way it is for the file server, and the decision is all-or-nothing
per volume:

- **eMMC:** shared, read-write, with no ceremony. The whole volume is the
  website; there is nothing else on it to disclose.
- **SD data partition:** **not shared by default.** Offering it would hand
  the WiFi passphrase and any ingress token to whatever computer the cable
  reaches — no case to open, no card to remove — and, being read-write, would
  let that computer write to `/data/.gosd` too, which survives your next
  re-flash. To share it anyway (bench work, or a board you only ever plug
  into your own computer), write `yes` into `config/env/WEBSITE_SHARE_DATA`
  on the boot partition and reboot; the app logs what it is exposing when you
  do. Without it, the board serves the site and says why no drive appeared.

That leaves eMMC-less boards editing their site by card rather than by cable:
power the board down, put the SD card in a computer — the data partition is
FAT32, so it mounts anywhere — and edit the `website` folder. If you want
cable editing on such a board, `WEBSITE_SHARE_DATA` is the informed way to
ask for it.

## What else it demonstrates

- **Expose *or* mount, never both.** A mass-storage LUN and a local mount of
  the same device must not be live at once — the host writes raw blocks with
  no idea of our filesystem. The app decides once per boot which one to be,
  releasing its own mount (`emmc.Unmount`, or unmounting `/data`) before the
  gadget takes the device.
- **Write `ReadOnly` out.** The example sets `ReadOnly: false` explicitly
  rather than letting the zero value mean it. How much a host may do to a
  volume is not a thing to leave implicit — and read-write is only defensible
  here because the volume was already established as shareable.
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
explicit consent: create `config/env/WEBSITE_WIPE_EMMC` on the boot
partition with `yes` in it (see docs/runtime.md's "App environment
variables"), then reboot. Without it, the app logs what to do and idles
rather than exiting, since `gosd-init` restarts exited apps regardless of
exit code and would otherwise crash-loop it forever.

The SD path never formats or relabels anything, so it needs no wipe consent —
its knob, `WEBSITE_SHARE_DATA`, gates disclosure rather than destruction.

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

- **To add content on an eMMC board:** connect the board to a computer with a
  USB cable. A drive named `WEBSITE` appears; drop your site's files on it
  (an `index.html` at the top level is the home page), then eject it.
- **To add content on an eMMC-less board:** power the board down, put its SD
  card in a computer, and edit the `website` folder on the `usbweb-data`
  volume (`index.html` there is the home page). Or set
  `WEBSITE_SHARE_DATA` — see above for what that exposes — and edit the same
  folder over USB.
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
code-complete, not verified. A Pi Zero bench test of the drive path needs
`WEBSITE_SHARE_DATA` set on the card.
