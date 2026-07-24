# usbwebsite — a static website you edit over USB

A GoSD example that turns a board with onboard eMMC into a tiny website
appliance:

- **Powered on its own**, it serves whatever's on the eMMC as a static
  website over HTTP on port 80.
- **Plugged into a computer**, it presents that same eMMC as a removable USB
  drive labelled `WEBSITE`, so you can drag your HTML/CSS/images straight on,
  eject, and power it standalone again to serve them.

It's the worked example for [`gadget.MassStorage`](../../gadget), built on the
[`emmc`](../../emmc) package: `emmc.FormatAndMount` formats the eMMC on first
boot and hands back the block device behind the mount, and `emmc.Unmount`
releases that device so the USB drive can take it exclusively.

## What it demonstrates

- **Expose *or* mount, never both.** A mass-storage LUN and a local mount of
  the same device must not be live at once — the host writes raw blocks with
  no idea of our filesystem. The app decides once per boot which one to be.
- **Detecting a connected computer.** After presenting the drive it watches
  the USB controller state (`/sys/class/udc/<udc>/state`): a real computer
  enumerates and *configures* the gadget within a second; a plain USB power
  supply never does, so the app falls back to serving.

## Boards

Needs a board with **both** onboard eMMC and a USB gadget controller — the
**Radxa Zero 3E** today (the Raspberry Pi boards have USB gadget but no eMMC;
the NanoPi Zero2 has eMMC but no USB gadget). On a board with no eMMC it logs
that and exits; with no gadget controller it just serves.

### A board whose eMMC already holds other content

Real hardware often ships with something already on the eMMC — vendor
firmware, a prior project. `usbwebsite` refuses to touch that without
explicit consent: set `WEBSITE_WIPE_EMMC = "yes"` in the `[env]` table of
`gosd.toml` on the `GOSD-BOOT` partition (see docs/runtime.md's "App
environment variables"), then reboot. Without it, the app logs what to do
and idles rather than exiting, since `gosd-init` restarts exited apps
regardless of exit code and would otherwise crash-loop it forever.

## Build & run

```sh
# --usb-gadget puts the board's USB port in peripheral mode.
gosd build ./examples/usbwebsite --board radxa-zero-3e --usb-gadget -o usbwebsite.img
```

Flash `usbwebsite.img` (see `docs/flashing.md`) and provision WiFi as usual.

- **To add content:** connect the board to a computer with a USB cable. A
  drive named `WEBSITE` appears; drop your site's files on it (an `index.html`
  at the top level is the home page), then eject it.
- **To serve:** power the board on its own (a wall charger, or a power-only
  input) and browse to `http://<hostname>.local` — the default hostname is
  `usbwebsite` unless you override it. A brand-new board serves a starter
  page explaining these same steps.

## Power topology note

The board decides it's "plugged into a computer" only when a USB *host*
configures the drive. If you power the board through the same port a computer
would use, plugging into a computer → drive mode, and a dumb charger → website
mode. Boards with a separate power input (so the gadget port is free) behave
the most predictably. This example has not been run on hardware yet (no GoSD
board has completed bring-up — see `COMPATIBILITY.md`), so treat the USB
state-machine behaviour as code-complete, not verified.
