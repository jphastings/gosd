# sattrack — a satellite ground track on an HDMI display

A GoSD example that turns a Raspberry Pi Zero W with a mini-HDMI cable into
a live satellite tracker: a fullscreen world map — NASA's Black Marble
city-lights imagery where it's night, the Blue Marble day texture where
the sun is up, blended through a soft twilight band at the live-computed
terminator — with the satellite's current position (red circle with a thin
black stroke), its ground track over the past 30 minutes (solid red line,
fading out over its oldest ~5 minutes), the coming 30 minutes (dashed red
line — 16px dashes measured along the track, phase-anchored so painted
dashes never crawl), and the satellite's name (black text, white stroke)
fixed to the right of the circle, updating once per second. The terminator
advances via a once-a-minute strip relight (a pure-Go solar ephemeris
finds the twilight band analytically per column), so even the day/night
cycle never costs a full-frame repaint.

Beyond the pretty picture, this example demonstrates two things:

- **Display apps are a custom-kernel recipe, not a base-kernel feature.**
  GoSD's stock kernels cut all of DRM/video (see `docs/custom-kernels.md`),
  so this example ships the `gosd build-kernel` recipe (`kernel/`) that
  compiles the display driver back in — vc4 for the Pi Zero W, virtio-gpu
  for qemu-virt.
- **armv6 is enough.** The whole stack — SGP4 propagation, software
  rendering, DRM dumb-buffer scanout — is pure Go on a single 1GHz BCM2835
  core, kept cheap by strictly partial repaints: after the first full
  paint, a tick repaints only the satellite marker/label, the track tips,
  and the fade window, never the frame.

## Running it under qemu (no hardware needed)

Kernel builds need Docker/Podman running (see `docs/custom-kernels.md`);
everything else is plain Go.

```sh
# One-off (~30-90 min): build the qemu-virt kernel with virtio-gpu DRM.
gosd build-kernel --board qemu-virt \
  --config examples/sattrack/kernel/gosd-kernel.toml -o ./sattrack-artifacts

# Build and boot, with the display in a host window (Cocoa/GTK):
gosd run --display --artifacts-dir ./sattrack-artifacts ./examples/sattrack
```

The map appears in the qemu window as soon as gosd-init has brought
networking up and the first TLE fetch lands (serial console logs stay on
your terminal). For an already-built image, `QEMU_DISPLAY=1
scripts/qemu-run.sh <image.img>` does the same.

## Running it on a Raspberry Pi Zero W

```sh
gosd build-kernel --board pi-zero-w \
  --config examples/sattrack/kernel/gosd-kernel.toml -o ./sattrack-artifacts

gosd build ./examples/sattrack --board pi-zero-w \
  --artifacts-dir ./sattrack-artifacts -o sattrack.img
```

Flash `sattrack.img` (see `docs/flashing.md`), provision WiFi as usual,
connect a mini-HDMI adapter to a monitor, and power up.

## Choosing the satellite

`SATTRACK_NORAD_ID` selects the satellite by NORAD catalog id (default
`68498`); set it at build time via `gosd.toml`'s `[env]` table or
`gosd build --env SATTRACK_NORAD_ID=25544`. TLEs come from
[tle.ivanstanojevic.me](https://tle.ivanstanojevic.me) at startup
(retrying with backoff until the network is up) and refresh every 6 hours;
a failed refresh keeps the previous element set. Propagation waits until
the clock is plausibly SNTP-set — these boards have no RTC.

## No display?

On a stock GoSD kernel (or with the cable unplugged) the app logs one
actionable message pointing here and at `docs/custom-kernels.md`, then
retries every 60 seconds. It deliberately never exits, so gosd-init's
supervisor doesn't restart-churn it.

## Status

- **qemu-virt: run-proven.** Custom kernel built via `gosd build-kernel`,
  image booted with `gosd run --display`, map + moving satellite verified
  on screen (virtio-gpu needs the `DRM_IOCTL_MODE_DIRTYFB` damage flushes
  this example issues per update; real scanout hardware ignores them).
- **pi-zero-w: build-proven only.** The kernel fragment merges, the DTS
  patch applies against the pinned raspberrypi/linux commit, and the built
  DTB carries the vc4 pipeline enabled — but like every GoSD board today
  it awaits hardware bring-up (see `COMPATIBILITY.md`), so HDMI output has
  not been exercised on a physical Pi Zero W.

Two honest wrinkles found while building the recipe, recorded in bean
gosd-e9fy: `DRM_VC4` hard-depends on `SND && SND_SOC` at the pinned
commit, so the fragment re-enables a minimal sound core despite GoSD's
"no sound" trim; and on the upstream-style `bcm2835-rpi-zero-w.dts` GoSD
builds, most of the vc4 pipeline carries no status gate, so the DTS patch
pins `status = "okay"` explicitly (defensive) rather than flipping
anything that was hard-disabled.

## Map imagery attribution

`bluemarble.jpg` is NASA's **Blue Marble Next Generation** (December 2004,
*world.topo.bathy.200412*), courtesy NASA Earth Observatory / Visible
Earth — <https://visibleearth.nasa.gov/images/73909> — a public-domain
NASA image, downscaled from the 5400×2700 original to 2048×1024 with
Catmull-Rom resampling.

`blackmarble.jpg` is NASA's **Black Marble** (Earth at Night 2016 global
composite), courtesy NASA Earth Observatory / Visible Earth —
<https://visibleearth.nasa.gov/images/144898> — a public-domain NASA
image, downscaled from the 3600×1800 original to 2048×1024 with the same
pipeline.
