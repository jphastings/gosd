# chime — audio out of a GoSD board, in pure Go

A GoSD example that plays a two-note boot chime and then a rising test tone
every 30 seconds, out of the board's HDMI port (or its analog jack, where it
has one). No alsa-lib, no cgo, no bundled audio file: the tones are
synthesised with `math.Sin`, and playback talks the kernel's ALSA PCM ioctl
interface directly.

Beyond the noise, this example demonstrates two things:

- **Audio apps are a custom-kernel recipe, not a base-kernel feature.**
  GoSD's stock kernels set `# CONFIG_SOUND is not set` (see
  `docs/custom-kernels.md`), so this example ships the `gosd build-kernel`
  recipe (`kernel/`) that compiles the ALSA core and the Pi's firmware audio
  driver back in — and, just as importantly, a deny-list that stops the
  raspberrypi defconfig's entire audio ecosystem coming along for the ride.
- **A GoSD image has no userspace, so the kernel ABI is the API.** There is no
  `libasound.so.2` to link or `dlopen`, and no `/usr/share/alsa` config tree.
  `pcm_linux.go` is the whole client: about 200 lines of `HW_PARAMS` →
  `SW_PARAMS` → `PREPARE` → `WRITEI_FRAMES`, which is all a blocking playback
  path actually needs.

## Running it on a Raspberry Pi

```sh
gosd build-kernel --board pi-zero-2w \
  --config examples/chime/kernel/gosd-kernel.toml -o ./chime-artifacts

gosd build ./examples/chime --board pi-zero-2w \
  --artifacts-dir ./chime-artifacts -o chime.img
```

Kernel builds need Docker/Podman running (see `docs/custom-kernels.md`);
everything else is plain Go. `pi-zero-w` and `pi-3b` work the same way — all
three share one fragment and one patch.

Flash `chime.img` (see `docs/flashing.md`), **connect the HDMI cable before
powering up** (see below), and listen.

## Why the HDMI cable has to be connected at boot

On the Pi, audio comes from `snd_bcm2835`, the driver that talks to the
VideoCore firmware over VCHIQ. Its HDMI ALSA cards exist only if the firmware
reports a live display when the driver probes: `probe()` asks for
`FRAMEBUFFER_GET_NUM_DISPLAYS`, then creates a card per HDMI display it finds.
Plug the monitor in afterwards and there is nothing to play to — the app will
log that it found no usable audio device and keep retrying, but the card will
not appear until the next boot.

Two more consequences of that firmware-owned path, both good:

- **No DRM.** `snd_bcm2835` needs neither the DRM subsystem nor ASoC, so audio
  costs a fraction of what `examples/sattrack`'s display recipe does. It works
  precisely *because* GoSD doesn't load the vc4 KMS driver: upstream disables
  the firmware's HDMI audio when vc4 is present, because both fighting over one
  HDMI output goes badly.
- **No device tree change.** `snd_bcm2835` binds to a VCHIQ bus device that
  `vchiq_arm` registers unconditionally, not to a DT node, so the recipe needs
  no device-tree patch at all — which matters on pi-zero-w, whose
  mainline-style DTB has no `audio` node and no `__overrides__` block for
  `dtparam=audio=on` to work through.

What the recipe *does* need is a one-line patch, and the reason is worth
knowing if you ever need to set a module parameter from a custom kernel.
`snd_bcm2835`'s `enable_hdmi` parameter defaults to off, GoSD kernels are
monolithic (so the only way to set a parameter is the kernel command line), and
nothing available to a recipe can get it there on all three boards:

- `dtparam=audio=on` in `config.txt` works on **pi-zero-2w and pi-3b** — their
  downstream DTBs carry an `__overrides__` entry that rewrites `chosen`'s
  `bootargs` to exactly `snd_bcm2835.enable_hdmi=1` — but an example can't add
  lines to `config.txt` (bean `gosd-mf3a`), and pi-zero-w's mainline-style DTB
  has no `__overrides__` node at all for it to work through.
- `CONFIG_CMDLINE` + `CONFIG_CMDLINE_EXTEND` works on **pi-zero-w** and is
  absent on arm64: `arch/arm64/Kconfig` offers only `CMDLINE_FROM_BOOTLOADER`
  and `CMDLINE_FORCE`, and forcing would throw away the
  `console=`/`init=`/`gosd.board=` arguments gosd-init needs. A first version
  of this recipe used it and failed the pi-zero-2w build outright.

So `kernel/patches/0001-default-hdmi-audio-on.patch` changes the driver's
default instead, which behaves identically everywhere. It doesn't force a card
into existence: the driver still asks the firmware which displays are live, so
a board with nothing plugged in still gets no HDMI card. Pass
`snd_bcm2835.enable_hdmi=0` to get the upstream default back.

(Recipe patches are applied with a plain `patch -p1` at the kernel tree root,
so they aren't limited to device trees despite `docs/custom-kernels.md`
describing them that way.)

## Which output it picks

Boards can expose more than one PCM. The app reads `/proc/asound/pcm`, prefers
anything whose name contains `HDMI` or `IEC958`, and logs what it chose:

```
playing to bcm2835 HDMI 1 (/dev/snd/pcmC0D0p) at 48000 Hz, 2 channels
```

On a Pi Zero W or Zero 2 W there is no audio jack at all, so HDMI is the only
real output — but `snd_bcm2835` still creates a `bcm2835 Headphones` card
whose PWM pins go nowhere on those boards, which is exactly why the HDMI
preference exists. On a Pi 3B, the same card drives the 4-pole 3.5mm jack, so
unplug HDMI (and reboot) to hear it there instead. Force a specific device
with `CHIME_DEVICE=/dev/snd/pcmC1D0p`.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `CHIME_EVERY` | `30s` | How often the test tone plays (any Go duration) |
| `CHIME_DEVICE` | unset | Play to this PCM instead of choosing one |

Set them at build time via `gosd.toml`'s `[env]` table or
`gosd build --env CHIME_EVERY=5m`.

## No audio device?

On a stock GoSD kernel there is no `/dev/snd` at all. The app logs one
actionable message pointing here and at `docs/custom-kernels.md`, then retries
every 60 seconds. Like `examples/sattrack`, it deliberately never exits, so
gosd-init's supervisor doesn't restart-churn it.

## Status

- **pi-zero-w and pi-zero-2w: build-proven.** This exact fragment was built by
  `gosd build-kernel` against a real Docker daemon for both boards; the
  resulting `kernel.config` carries `CONFIG_SND_BCM2835=y` and none of the
  denied symbols, and the HDMI-default patch applied cleanly.
- **pi-3b: recipe only.** Same fragment, same defconfig as pi-zero-2w, same
  driver — but not compiled here.
- **No board has been hardware-verified.** No GoSD board has had audio on a
  bench yet (see `COMPATIBILITY.md`), so nothing here has actually been heard.
  The first thing to check when one does: whether the HDMI card appears at all,
  which is the firmware-display-enumeration dependency described above.
- **Rockchip boards are not supported by this recipe** and are tracked
  separately (bean `gosd-lrxz`): their HDMI audio codec hangs off the DRM
  dw-hdmi bridge, so it needs the whole DRM subsystem plus ASoC plus a DTS
  patch. The ROCK 4SE's analog ES8316 output needs ASoC but no DRM; the NanoPi
  Zero2 has no HDMI connector and no audio controller in mainline at all.

One honest wrinkle found while building the recipe, recorded in bean
`gosd-y9hc`: turning sound on wakes GoSD's *dormant USB MIDI gadget function*,
because it depends on `SND_RAWMIDI`, which can't exist while `CONFIG_SOUND` is
off. Legacy gadget drivers claim the board's only UDC at probe — the same way
"Gadget Zero" broke `--usb-gadget` in bean `gosd-spjt` — so the fragment
denies it explicitly.
