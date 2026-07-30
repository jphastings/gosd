# Sound: playing audio from a GoSD app

Two halves, and you need both:

1. **A kernel with sound in it.** Every published GoSD kernel is built with
   `# CONFIG_SOUND is not set`, so a stock image has no `/dev/snd` at all.
   Audio is an opt-in `gosd build-kernel` recipe, like display/DRM — see
   [Which kernel do I need](#which-kernel-do-i-need) below.
2. **Code to play frames.** The `sound` package
   ([pkg.go.dev](https://pkg.go.dev/github.com/jphastings/gosd/sound)) talks
   the kernel's ALSA PCM interface directly — no cgo, no alsa-lib, nothing to
   install in the image — see [Playing a sound](#playing-a-sound).

`examples/chime` is the end-to-end worked example: a boot chime and a periodic
test tone, plus the kernel recipes both halves of this page describe.

## Which kernel do I need

| Board | HDMI audio | Analog out | `gosd-kernel.toml` recipe | DRM pulled in? | Kernel growth |
|---|---|---|---|---|---|
| pi-zero-w | mini-HDMI ✅ | **none** (no jack) | `examples/chime/kernel/gosd-kernel.toml` → `pi.fragment` + patch | no | +104,248 B (+0.63%) |
| pi-zero-2w | mini-HDMI ✅ | **none** (no jack) | same | no | +401,408 B (+0.71%) |
| pi-3b | HDMI ✅ | 3.5 mm jack (PWM from the SoC) | same | no | not measured; same fragment |
| rock-4se | HDMI ✅ (needs DRM) | 3.5 mm jack via an ES8316 codec | `gosd-kernel.toml` → `rock-4se-analog.fragment` (jack only), or `hdmi.toml` → `rock-4se-hdmi.fragment` (jack + HDMI) | analog: **no**; HDMI: **yes** | not measured |
| radxa-zero-3e | micro-HDMI ✅ (needs DRM) | none — Radxa omitted the jack | `hdmi.toml` → `radxa-zero-3e-hdmi.fragment` | yes | not measured |
| nanopi-zero2 | **no HDMI connector** | none | — | — | — |

The two Pi figures are real bytes, measured against the published
`artifacts/v0.8.0` kernels; see [Measuring it yourself](#measuring-the-cost-yourself)
for the Rockchip rows, which nobody has built yet.

**nanopi-zero2 has no audio path.** Its RK3528 has I2S and SPDIF pins on the
30-pin FPC header, but at the pinned kernel tag `arch/arm64/boot/dts/rockchip/rk3528.dtsi`
defines no `i2s`, `spdif`, `hdmi` or `vop` node at all — only pin-mux groups
with nothing to attach them to. There is no driver to enable, so there is no
recipe: this board is ➖ rather than 🚧.

### Building one

```sh
# Pi boards (HDMI and, on the 3B, the jack):
gosd build-kernel --board pi-zero-2w \
  --config examples/chime/kernel/gosd-kernel.toml -o ./sound-artifacts

# ROCK 4SE headphone jack, no DRM:
gosd build-kernel --board rock-4se \
  --config examples/chime/kernel/gosd-kernel.toml -o ./sound-artifacts

# ROCK 4SE with HDMI audio as well, or a Radxa Zero 3E (HDMI-only board):
gosd build-kernel --board rock-4se \
  --config examples/chime/kernel/hdmi.toml -o ./sound-artifacts

# Then build the image from those artifacts instead of a published release:
gosd build ./examples/chime --board rock-4se \
  --artifacts-dir ./sound-artifacts -o chime.img
```

Kernel builds need a local Docker or Podman daemon and take 20–60 minutes the
first time; they are content-addressed and cached, so a re-run with unchanged
inputs is instant. See [custom-kernels.md](custom-kernels.md) for the general
mechanism, the caching rules, and the host requirements.

You can point a recipe of your own at these fragments — copy the file, don't
write a three-line "enable sound" fragment (see
[the deny-list trap](#the-deny-list-is-the-load-bearing-half)).

### Why HDMI costs DRM on Rockchip but not on the Pi

On the Pi, audio is `snd_bcm2835`: a driver that asks the VideoCore firmware to
play, over VCHIQ. The firmware owns HDMI on a GoSD image (we never load vc4
KMS), so HDMI audio needs no DRM, no ASoC, and — because the driver binds to a
VCHIQ *bus* device rather than a device-tree node — no DTS patch. Three
Kconfig lines and a one-line patch that defaults the driver's `enable_hdmi`
module parameter on, which is the only lever that behaves identically on all
three Pi boards.

On RK3399 and RK3566, HDMI audio is an I2S codec hanging off the Synopsys
DesignWare HDMI bridge (`DRM_DW_HDMI_I2S_AUDIO`), and that bridge is a
component of the Rockchip DRM driver. Asking for HDMI sound therefore compiles
in DRM, the display controller and the KMS helpers — exactly the subsystem
GoSD's stock kernels cut. That is why the ROCK 4SE has two recipes: its
**analog** path (ES8316 codec on I2C1 at 0x11, fed by I2S0) needs ASoC but no
DRM at all, so an app that just wants a beep out of the headphone jack pays
nothing for a display stack it will never use.

Neither Rockchip path needs a device-tree patch either, which surprised us:
mainline already wires both up. `rk3399-rock-4se.dts` includes
`rk3399-rock-pi-4.dtsi`, which enables `i2c1` with its `codec@11` node,
enables `i2s0` with an audio-graph port pointing at the codec, declares the
analog card (`sound { compatible = "audio-graph-card"; label = "Analog"; }`),
and sets `&hdmi`, `&hdmi_sound` and `&i2s2` to `"okay"`. Same story on the
Radxa Zero 3E via `rk3566-radxa-zero-3.dtsi`. So these recipes are
Kconfig-only, they live in the example rather than in
`build/boards/<board>/kernel/patches/`, and they need no artifacts release.

## Gotchas

### HDMI audio only exists if the display was connected before power-up

On the Pi this is absolute: `snd_bcm2835`'s probe asks the firmware
`FRAMEBUFFER_GET_NUM_DISPLAYS` and creates one ALSA card per HDMI display it
finds, once. Plug the monitor in afterwards and there is no card to play to
until the next boot. Connect the cable, *then* power up.

Expect the same rule to bite on Rockchip for a different reason — the sink's
capabilities come from the display's EDID — though there the card is created by
a device-tree machine driver rather than by display enumeration. Unverified
either way on Rockchip; see [Verification status](#verification-status).

### The Pi Zero W and Zero 2 W have no analog output

Neither board has a jack. `snd_bcm2835` still registers a
`bcm2835 Headphones` card on them, and it plays into nothing — which makes
"my app says it is playing but I hear nothing" a very easy trap. Prefer the
HDMI PCM on those boards (`sound.Options{Prefer: sound.HDMI}`, which is what
`examples/chime` does), or wire up an I2S DAC on the header (out of scope
here). The pi-3b's 4-pole jack is real, but it is PWM-driven straight from the
SoC rather than a DAC, so it is noisy by design.

### The deny-list is the load-bearing half

Enabling `CONFIG_SND` does not add "sound": it un-hides every audio driver the
board's defconfig ships as a module, and GoSD kernels are monolithic
(`CONFIG_MODULES` is always off), so `make olddefconfig` promotes all of them
to built-in. Measured, on a Pi: a three-line "enable sound" fragment silently
compiled in ~60 HAT machine drivers, ~45 codecs, USB audio, the MIDI sequencer
and OSS emulation. On arm64 the defconfig is multiplatform, so the equivalent
haul is every SoC vendor's ASoC platform drivers plus about twenty codecs.

Both of this example's fragment families are therefore mostly deny-list, and
each denied symbol traces to a line the pinned defconfig really ships (the
fragments carry the one-liner that regenerates the list). On the Pi,
`# CONFIG_SND_SOC is not set` does nearly all the work; on Rockchip it cannot,
because ASoC is precisely what the codec needs.

### Sound wakes the USB MIDI gadget, which can break `--usb-gadget`

`USB_MIDI_GADGET` and `USB_CONFIGFS_F_MIDI` depend on the raw-MIDI core, which
cannot exist while `CONFIG_SOUND` is off — so they sit dormant in every stock
GoSD kernel and become reachable the moment sound appears. Legacy gadget
drivers claim the board's only UDC at probe, which is exactly how "Gadget Zero"
stopped `--usb-gadget` working once before (bean `gosd-spjt`). The raspberrypi
defconfigs do ship them as modules, so on the Pi this is a real, measured
15,424 bytes and a real risk; the arm64 defconfig does not, so on Rockchip it
is only a precaution. Every fragment here denies them explicitly.

### Sound is not in the stock kernels, and that is a decision, not an oversight

Whether it should stay that way is bean `gosd-ette` (a recipe keeps the cost
off the boards that don't want audio, but the measured cost turned out to be
under 1% of the Pi kernels). Epic `gosd-qkbl` holds the full research and the
argument. Until that is decided, audio means `gosd build-kernel`.

## Playing a sound

The `sound` package finds a playback device, takes interleaved signed 16-bit
little-endian frames, and blocks until they have been played:

```go
package main

import (
	"log"
	"math"

	"github.com/jphastings/gosd/sound"
)

func main() {
	dev, err := sound.Open() // or OpenWith(sound.Options{Prefer: sound.Analog})
	if err != nil {
		// The message names the fix when the kernel has no sound in it.
		log.Fatal(err)
	}
	defer func() { _ = dev.Close() }()

	f := dev.Format() // what the device accepted: rate and channel count
	log.Printf("playing to %s at %s", dev.Name(), f)

	// One second of A440, at whatever rate and channel count we got.
	frames := make([]byte, f.Rate*f.FrameBytes())
	for i := 0; i < f.Rate; i++ {
		s := int16(math.Sin(2*math.Pi*440*float64(i)/float64(f.Rate)) * 16000)
		for c := 0; c < f.Channels; c++ {
			frames[i*f.FrameBytes()+c*2] = byte(uint16(s))
			frames[i*f.FrameBytes()+c*2+1] = byte(uint16(s) >> 8)
		}
	}
	if err := dev.Play(frames); err != nil {
		log.Fatalf("playback failed: %v", err)
	}
}
```

That is the whole API surface: `Open`/`OpenWith`, and a `Device` you `Play`
to, ask the `Format` and `Name` of, and `Close`. `Options` lets you force one
device path, prefer HDMI or analog on a board that has both, and ask for a
specific rate or channel count.

**Handle "no audio device" as a normal state.** It is the overwhelmingly
common case — a stock kernel has no `/dev/snd` — so `Open` returns an error
wrapping `sound.ErrNoDevice` whose text names the fix, and distinguishes a
kernel without sound from a kernel with sound but no card (an HDMI cable
plugged in too late). An appliance should log it and carry on rather than
exit, so gosd-init's supervisor has nothing to restart-churn; `examples/chime`
retries on a slow timer forever.

**S16_LE frames only, and no decoders.** WAV/raw PCM is an
`encoding/binary` header parse away, so synthesise your audio or ship raw
frames. MP3/Vorbis/FLAC decoding, capture (the ROCK 4SE's jack is also a mic
input), mixing and non-blocking pipelines are deliberately absent — beans
`gosd-nxm4` and `gosd-tjrw`.

**Why not alsa-lib, or a library that wraps it?** A GoSD image has no
userspace: no `libasound.so.2` to link or `dlopen`, no `/usr/share/alsa`
config tree. Even the pure-`purego` wrappers (`ebitengine/oto`) need
alsa-lib's shared object on disk at runtime. So this package speaks the
kernel's PCM ioctl ABI itself — `HW_PARAMS` → `SW_PARAMS` → `PREPARE` →
`WRITEI_FRAMES`, with underrun recovery — which is what alsa-lib's `hw:`
plugin does underneath anyway. Epic `gosd-qkbl` records the survey of the
alternatives.

## Verification status

Honest state of play, because none of this has made a noise yet:

| Board | Recipe compiled? | Heard on hardware? |
|---|---|---|
| pi-zero-w, pi-zero-2w | ✅ real `gosd build-kernel` runs; resulting `kernel.config` checked | ❌ not yet |
| pi-3b | ❌ (shares the fragment, patch, defconfig and driver with the Zeros) | ❌ not yet |
| rock-4se, radxa-zero-3e | ❌ — the fragments were written against the pinned kernel's Kconfig and device trees, but never compiled | ❌ not yet |

The playback code itself is exercised by unit tests on the host (the ALSA
struct layouts are pinned for both 64- and 32-bit word widths, which the ioctl
numbers depend on), and cross-compiles for every board architecture.

### Measuring the cost yourself

`gosd build-kernel` prints where it wrote each artifact; compare that file's
size with the same board's published artifact, not with an older cache entry
(a stale cached kernel once made audio look like a 10% *saving*):

```sh
# Stock: build the image the normal way, which downloads and caches the
# published kernel under <user cache dir>/gosd/artifacts/<version>/<board>/
# (~/Library/Caches on macOS, ~/.cache or $XDG_CACHE_HOME on Linux).
gosd build ./examples/chime --board rock-4se -o /tmp/stock.img
ls -l ~/Library/Caches/gosd/artifacts/*/rock-4se/Image

# With sound:
gosd build-kernel --board rock-4se --config examples/chime/kernel/gosd-kernel.toml -o /tmp/snd
ls -l /tmp/snd/Image
```

Kernel builds take tens of minutes: run them in the background and poll the
log rather than waiting on a foreground shell.
