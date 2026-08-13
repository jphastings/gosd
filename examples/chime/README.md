# chime — audio out of a GoSD board, in pure Go

A GoSD example that plays a two-note boot chime and then a rising test tone
every 30 seconds, out of the board's HDMI port (or its analog jack, where it
has one). No alsa-lib, no cgo, no bundled audio file: the tones are
synthesised with `math.Sin`, and playback goes through the `sound` package,
which talks the kernel's ALSA PCM ioctl interface directly.

Beyond the noise, this example demonstrates two things:

- **Audio apps are a custom-kernel recipe, not a base-kernel feature.**
  GoSD's stock kernels set `# CONFIG_SOUND is not set`, so this example ships
  the `gosd build-kernel` recipes (`kernel/`) that compile the ALSA core and
  each board's audio driver back in — and, just as importantly, a deny-list
  that stops the defconfig's entire audio ecosystem coming along for the ride.
  **[docs/sound.md](../../docs/sound.md) is the per-board guide**; this file is
  about the example and the Pi's surprises.
- **Playing audio is four calls.** `main.go` and `tone.go` are the whole app:
  synthesise frames, `sound.Open`, `Play`, retry forever if there is no device.
  Everything hard — the uapi structs, `HW_PARAMS` → `SW_PARAMS` → `PREPARE` →
  `WRITEI_FRAMES`, underrun recovery, the two word widths GoSD boards use — is
  in the `sound` package, so your app never copies it.

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

## Running it on a Rockchip board

The ROCK 4SE has two recipes, because its two outputs cost wildly different
amounts. Its 3.5 mm jack is an ES8316 codec on I2C1, which needs ASoC but no
DRM; its HDMI audio is a codec on the DRM dw-hdmi bridge, which pulls in the
whole DRM subsystem:

```sh
# Headphone jack only — the cheap one:
gosd build-kernel --board rock-4se \
  --config examples/chime/kernel/gosd-kernel.toml -o ./chime-artifacts

# ...or jack + HDMI, at the cost of DRM:
gosd build-kernel --board rock-4se \
  --config examples/chime/kernel/hdmi.toml -o ./chime-artifacts

gosd build ./examples/chime --board rock-4se \
  --artifacts-dir ./chime-artifacts -o chime.img
```

The Radxa Zero 3E has no jack at all, so `hdmi.toml` (with
`--board radxa-zero-3e`) is its only option. The NanoPi Zero2 has no audio
hardware reachable at the pinned kernel — no `i2s`, `spdif` or `hdmi` node
exists in its SoC's device tree — so there is no recipe for it.

Neither Rockchip board needs a device-tree patch: mainline already enables the
codec, the I2S controllers and both cards. On the 4SE, set
`CHIME_OUTPUT=analog` if you built the HDMI recipe but want to listen on the
jack.

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

Boards can expose more than one PCM. The `sound` package reads
`/proc/asound/pcm` and `/proc/asound/cards`, drops the virtual cards (an
`snd-aloop` loopback plays into nothing, and usually holds card 0 — see
[the gotcha](../../docs/sound.md#a-virtual-loopback-card-swallows-the-audio-config_snd_aloop)),
and ranks what is left; this app asks for
`sound.Options{Prefer: sound.HDMI}` by default (`CHIME_OUTPUT` overrides it)
and logs what it got:

```
playing to bcm2835 HDMI 1 (/dev/snd/pcmC0D0p) at 48000 Hz, 2 channels
```

On a Pi Zero W or Zero 2 W there is no audio jack at all, so HDMI is the only
real output — but `snd_bcm2835` still creates a `bcm2835 Headphones` card
whose PWM pins go nowhere on those boards, which is exactly why the HDMI
preference exists. On a Pi 3B, the same card drives the 4-pole 3.5mm jack, so
unplug HDMI (and reboot) to hear it there instead. On a ROCK 4SE built with the
analog-only recipe there is no HDMI PCM to find, so the preference costs
nothing and it lands on the jack. Force a specific device with
`CHIME_DEVICE=/dev/snd/pcmC1D0p`.

It passes `sound.Options{Logf: log.Printf}` too, so anything surprising about
that choice — a skipped loopback, a preference that could not be honoured —
is a line in the log rather than something to deduce from silence.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `CHIME_EVERY` | `30s` | How often the test tone plays (any Go duration) |
| `CHIME_DEVICE` | unset | Play to this PCM instead of choosing one |
| `CHIME_OUTPUT` | `hdmi` | Which output to prefer: `hdmi`, `analog` or `any` |

Set them at build time via `gosd.toml`'s `[env]` table or
a `CHIME_EVERY` file in the app's `config/env/` directory.

## No audio device?

On a stock GoSD kernel there is no `/dev/snd` at all. `sound.Open` says so in
words that name the fix (and distinguishes it from a kernel that *has* sound
but produced no card — an HDMI cable connected too late), the app logs that and
retries every 60 seconds. Like `examples/sattrack`, it deliberately never
exits, so gosd-init's supervisor doesn't restart-churn it.

## Status

- **pi-zero-w and pi-zero-2w: build-proven.** This exact fragment was built by
  `gosd build-kernel` against a real Docker daemon for both boards; the
  resulting `kernel.config` carries `CONFIG_SND_BCM2835=y` and none of the
  denied symbols, and the HDMI-default patch applied cleanly.
- **pi-3b: recipe only.** Same fragment, same defconfig as pi-zero-2w, same
  driver — but not compiled here.
- **rock-4se and radxa-zero-3e: recipe only, not even compiled.** Their three
  fragments were written against the pinned kernel's Kconfig and device trees
  (bean `gosd-lrxz`) but no `gosd build-kernel` run has been made, so their
  size cost is unmeasured too — see `docs/sound.md` for how to measure it.
- **No board has been hardware-verified.** No GoSD board has had audio on a
  bench yet (see `COMPATIBILITY.md`), so nothing here has actually been heard.
  The first thing to check on a Pi: whether the HDMI card appears at all, which
  is the firmware-display-enumeration dependency described above. On the ROCK
  4SE: whether the ES8316 probed on I2C1 (the boot log will say) and whether
  the jack is audible.

One honest wrinkle found while building the recipe, recorded in bean
`gosd-y9hc`: turning sound on wakes GoSD's *dormant USB MIDI gadget function*,
because it depends on `SND_RAWMIDI`, which can't exist while `CONFIG_SOUND` is
off. Legacy gadget drivers claim the board's only UDC at probe — the same way
"Gadget Zero" broke `--usb-gadget` in bean `gosd-spjt` — so the fragment
denies it explicitly.
