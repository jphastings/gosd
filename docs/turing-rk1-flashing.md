# Putting an app onto your Turing RK1

The Turing RK1 is different from every other board GoSD supports: it has no
memory card slot at all. It boots from an eMMC chip built into the module
itself, so [Raspberry Pi Imager's usual custom-repository flow](flashing.md)
— which only knows how to write to a memory card reader — can't put an app
onto it. This guide covers the two ways that actually work. Both write the
exact same `.img` file GoSD builds for every other board; nothing about the
image itself is different, only how it gets onto the device.

You'll need:

- A Turing RK1 module, plugged into a Turing Pi 2 or 2.5 cluster baseboard
- The `.img` file your app's developer gave you (or built yourself with
  `gosd build`)
- Either a USB-C cable and a Linux PC (Method A), or just a browser on the
  same network as your Turing Pi 2 (Method B)

## Method A: rkdeveloptool (USB, from a Linux PC)

This is the most reliable method, and works even if your Turing Pi 2's BMC
isn't set up yet.

1. Install [`rkdeveloptool`](https://github.com/rockchip-linux/rkdeveloptool)
   on a Linux PC (build from source, or use your distro's package if it has
   one).
2. Set the node's boot-mode jumper/switch to Flash/Maskrom mode (check your
   Turing Pi 2's manual for its exact location — it differs between v2.4 and
   v2.5 boards), then power the node on. It won't boot normally in this
   mode — that's expected.
3. Connect a USB-C cable from your PC to the RK1 node's USB-OTG port.
4. Confirm the PC sees it:
   ```sh
   rkdeveloptool ld
   ```
   You should see a device listed as `Maskrom`.
5. Write the image (this writes the whole file starting at the beginning of
   the eMMC, the same way a card reader writes an SD card — replace
   `myapp-turing-rk1.img` with your actual file):
   ```sh
   rkdeveloptool wl 0 myapp-turing-rk1.img
   ```
   This takes 10-15 minutes depending on the image size. Don't disconnect
   the cable or power while it runs.
6. Once it finishes, set the boot-mode jumper/switch back to normal (eMMC)
   boot, and power-cycle the node.

## Method B: Turing Pi 2 BMC (network, no cable)

If your Turing Pi 2's BMC is already set up and reachable on your network,
you can flash over the network instead:

1. Open the BMC's web interface in a browser (check your Turing Pi 2's
   manual for its address, or use the `tpi` command-line tool if you have
   it installed).
2. Select the node your RK1 is in, and choose to flash from a local file —
   point it at the `.img` file.
3. Start the flash and wait for it to finish. This method is slower than
   USB (often 45-90 minutes), since the image travels over the network to
   the BMC and then to the node.

## Power it on

Once flashing finishes and the node is back in normal boot mode, power it
on (or power-cycle it). You don't need a screen, keyboard, or mouse —
everything happens automatically. Give it a minute or two to start up.

## Find your device

From a phone, tablet, or computer connected to the **same network**, open a
web browser and go to:

```
http://<the app's hostname>.local
```

Your app's developer will have told you what hostname to expect (GoSD
defaults to the app's own name if nothing else was set).

## Changing settings afterwards (WiFi, hostname, etc.)

Every other board's fallback still works here, unchanged: reflash isn't
needed for a settings change. Once the app is running, its config folder
lives on the boot partition — you can reach it the same way you'd reach any
plain disk partition, by reading the eMMC contents (e.g. mounting the boot
partition over USB gadget mode, if your app enables it, or via whatever
your Turing Pi 2 setup gives you access to the filesystem with). See
[the config-tree reference](config.md) for what's in that folder and how
each setting works — nothing there is turing-rk1-specific.

## Troubleshooting

**`rkdeveloptool ld` shows nothing.** Double check the boot-mode
jumper/switch is actually set to Flash/Maskrom before powering on — the
node has to be powered on *into* that mode; toggling it after boot doesn't
work. Try a different USB-C cable or port; some cables are power-only.

**The flash finishes but the node won't boot.** Make sure you moved the
boot-mode jumper/switch back to normal boot afterward — a node left in
Flash/Maskrom mode won't run the image you just wrote.

**`http://<name>.local` doesn't load.** Give it another minute — first
boot takes a little longer. If it still doesn't work, check your router's
admin page for a list of connected devices; the node should appear there by
name, along with a numeric address you can use instead.

---

## For developers: linking your users here

If your app's board set includes `turing-rk1`, point users at this guide
rather than the [Raspberry Pi Imager one](flashing.md) — Imager cannot
flash this board. A short version to paste into your own README:

```md
## Installing <YourApp> on a Turing RK1

This board has no memory card slot — it flashes over USB or your Turing Pi
2's network BMC instead. Full guide:
https://github.com/jphastings/gosd/blob/main/docs/turing-rk1-flashing.md
```
