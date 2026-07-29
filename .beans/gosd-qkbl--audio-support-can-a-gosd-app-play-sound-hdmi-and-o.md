---
# gosd-qkbl
title: 'Audio support: can a GoSD app play sound (HDMI and otherwise)?'
status: in-progress
type: epic
priority: normal
created_at: 2026-07-29T21:45:08Z
updated_at: 2026-07-29T23:04:26Z
---

JP asked, verbatim:

> Does gosd support playing audio? (Particularly over the HDMI port, if there
> is one; but also over other audio devices)

**Today: no.** Every board's stock kernel carries `# CONFIG_SOUND is not set`
(`build/boards/{pi-zero-w,pi-zero-2w,pi-3b}/kernel.fragment` state it
explicitly; the Rockchip boards' recorded `kernel.config`s have it unset too),
there is no audio code anywhere in the tree, and no doc mentions sound. So a
GoSD app has no `/dev/snd/*` to open — exactly the position display was in
before `examples/sattrack`.

It is, however, cheap to add per-app, and on the Pi boards HDMI audio needs
**no DRM at all** — which was the open question. This epic records the
research, the measured size numbers, and the one decision that is JP's:
whether sound stays an opt-in custom-kernel recipe (Route A) or goes into the
stock released kernels (Route B).

## Three layers, and where each one bites

1. **Kernel**: the ALSA core plus a driver for the specific audio path. Cut
   from every stock kernel today.
2. **Device plumbing**: does the driver bind without a DT change? Does it need
   a module parameter? On the Pi the answers are surprising (below).
3. **Userspace**: ALSA's `libasound` is C, and GoSD images have no userspace
   at all — no `/usr/lib`, no `libasound.so.2`, no `/usr/share/alsa`. Playback
   has to talk the kernel PCM ioctl ABI directly from Go.

## Per-board audio hardware (verified against vendor docs)

| Board | HDMI (audio-capable?) | Analog out | Codec | I2S/other |
|---|---|---|---|---|
| pi-zero-w | mini-HDMI, carries audio | **none** (no jack) | — | PCM on GPIO18-21 (pins 12/35/38/40) |
| pi-zero-2w | mini-HDMI, carries audio | **none** (no jack) | — | same PCM pins |
| pi-3b | full-size HDMI, carries audio | **yes** — 4-pole 3.5mm jack, PWM-driven from the SoC (not a DAC) | — (PWM) | same PCM pins |
| radxa-zero-3e | micro-HDMI 2.0, carries audio | none — Radxa: "due to space constraint, Radxa Zero does not have a 3.5mm headphone jack" | none onboard | I2S3 (+I2S1/2 alt) on the 40-pin header |
| nanopi-zero2 | **no HDMI connector at all** — headless by design | none | none | 2x I2S + SPDIF-Tx on the 30-pin FPC header |
| rock-4se | full-size HDMI 2.0 (CEC, 4Kp60), carries audio | **yes** — 4-ring 3.5mm jack, drives 32R headphones, doubles as mic in | **ES8316** (ALSA card `rockchip-es8316`) | I2S1 + 2x SPDIF_TX (header pins 15, 32) |
| qemu-virt | n/a | n/a | n/a | no audio device by default; `-device virtio-sound-pci` (needs `CONFIG_SND_VIRTIO`) or `intel-hda`+`hda-output` over the PCIe root complex |

Sources: raspberrypi.com product pages + Zero 2 W product brief (RP-008359-DS-1)
for the three Pis; docs.radxa.com `zero/zero3/hardware-design/hardware-interface`
and `wiki.radxa.com/Zero/hardware/audio` for the 3E; docs.radxa.com
`rock4/rock4ab-se/...` (its headphone-jack page's own asset is named
`rock-4se-headphoneJack.webp`) for the 4SE; BCM2835 ARM Peripherals §6.2 for the
PCM pin mux; qemu docs `system/arm/virt.rst` + `system/devices/virtio/virtio-snd.html`.

**nanopi-zero2 caveat:** FriendlyElec's wiki returns HTTP 403 to automated
fetches, so "no HDMI" rests on CNX Software's write-up quoting FriendlyElec's
spec sheet plus reseller listings of the same spec line, and on mainline having
no RK3528 display stack to speak of. Strongly corroborated, not
primary-verified — treat the "no HDMI, I2S/SPDIF only" row as the working
assumption and confirm on the bench.

## Kernel enablement, per family

### Raspberry Pi — HDMI audio without DRM (the good news)

The path is `snd_bcm2835`, the VideoCore firmware audio driver
(`drivers/staging/vc04_services/bcm2835-audio`, pinned commit
`63598c83153e19b1f99067ab6df7409de2c111f8`). Three facts, each read from the
pinned source rather than assumed:

- **Kconfig:** `depends on (ARCH_BCM2835 || COMPILE_TEST) && SND`,
  `select SND_PCM`, `select BCM2835_VCHIQ if HAS_DMA`. **No `SND_SOC`
  dependency** — no ASoC, no codec drivers, no DRM. `CONFIG_BCM2835_VCHIQ=y`
  and `CONFIG_RASPBERRYPI_FIRMWARE=y` are already in every stock GoSD Pi
  kernel, so the sound core is the only thing missing.
- **No device tree needed.** `snd_bcm2835_alsa_probe(struct vchiq_device *)`
  is a **VCHIQ bus** driver, not an OF/platform driver, and
  `vchiq_arm.c` calls `vchiq_device_register(&pdev->dev, "bcm2835-audio")`
  unconditionally. So `dtparam=audio=on` is *not* required at this commit, and
  no DTS patch is needed — which matters most for pi-zero-w, whose
  mainline-style `bcm2835-rpi-zero-w.dts` chain has no `audio` node and no
  `__overrides__` block for the firmware's `dtparam` mechanism to patch (the
  same lineage trap CLAUDE.md records for DMA and USB).
- **HDMI needs one module parameter.** `probe()` only calls
  `set_hdmi_enables()` — which asks the firmware
  `FRAMEBUFFER_GET_NUM_DISPLAYS`/`GET_DISPLAY_ID` and enables the
  `bcm2835 HDMI 1`/`HDMI 2` cards for display ids 2/7 — `if (enable_hdmi &&
  !of_property_read_bool(dev->of_node, "brcm,disable-hdmi"))`. `enable_hdmi`
  defaults to **false**; `enable_headphones` defaults to true. GoSD kernels are
  monolithic, so the parameter has to arrive on the kernel command line as
  `snd_bcm2835.enable_hdmi=1`, and an *example* cannot edit `cmdline.txt`
  (the board package renders it). Three levers were tried; only the third
  works on all three boards:
    1. `dtparam=audio=on` in `config.txt`. This *is* how the downstream DTBs
       implement it — `bcm2710-rpi-3-b.dts`'s `__overrides__` has
       `audio = <&chosen>,"bootargs{on='snd_bcm2835.enable_headphones=1
       snd_bcm2835.enable_hdmi=1',...}"`, and the Zero 2 W's the same without
       the headphones half. So it works on pi-zero-2w and pi-3b, but not on
       pi-zero-w (no `__overrides__` at all), and no example can write to
       `config.txt` anyway (gosd-mf3a).
    2. `CONFIG_CMDLINE="snd_bcm2835.enable_hdmi=1"` + `CONFIG_CMDLINE_EXTEND=y`
       in the fragment. Works on pi-zero-w and **does not exist on arm64**:
       `arch/arm64/Kconfig`'s command-line choice offers only
       `CMDLINE_FROM_BOOTLOADER` and `CMDLINE_FORCE` (no `EXTEND`), and forcing
       would discard the `console=`/`init=`/`gosd.board=` arguments gosd-init
       needs. The first version of gosd-y9hc's recipe used this and failed the
       pi-zero-2w build outright with "CONFIG_CMDLINE_EXTEND=y did not survive
       olddefconfig" — a useful reminder that a fragment which merges on one
       Pi board can be invalid on another.
    3. A one-line patch defaulting the driver's `enable_hdmi` to true. Behaves
       identically on all three boards, and still creates no card unless the
       firmware reports a live display. This is what shipped. Worth knowing:
       `gosd-kernel.toml`'s `patches` are applied with a plain `patch -p1` at
       the kernel tree root, so despite being documented as device-tree
       patches they can carry any in-tree change — which is the only portable
       way a recipe can set a module parameter in a monolithic kernel.

Because the firmware — not the kernel — owns HDMI on GoSD Pi images (no
`dtoverlay=vc4-kms-v3d`, DRM cut), this path is exactly the supported
configuration: raspberrypi/linux commit `1a2b5dca0575` ("snd_bcm2835: disable
HDMI audio when vc4 is used") adds `brcm,disable-hdmi` precisely so the
firmware driver *stands down* when KMS is loaded, "things don't work too well
when both the vc4 driver and the firmware driver are trying to control the same
audio output". GoSD never loads KMS, so `snd_bcm2835` keeps HDMI. The
corollary is a real constraint: an HDMI display must be **connected and
detected at boot**, since card creation depends on the firmware's live display
enumeration.

Analog: `bcm2835 Headphones` appears unconditionally, but only the pi-3b has a
jack wired to it. On pi-zero-w/2w it is a card that plays into nothing.

Minimal fragment (three lines) plus a deny-list — see below for why the
deny-list is the load-bearing half.

### Rockchip — HDMI audio *does* need DRM

HDMI audio on RK3399/RK3566 is a codec hanging off the Synopsys DesignWare
HDMI bridge (`CONFIG_DRM_DW_HDMI_I2S_AUDIO`, under
`drivers/gpu/drm/bridge/synopsys/`), driven through `SND_SOC` +
`SND_SOC_HDMI_CODEC` and wired in the DT as an `hdmi-sound`/`hdmi_sound` graph
node. That means the whole DRM subsystem plus ASoC plus a DTS patch (our pinned
U-Boots have no `OF_LIBFDT_OVERLAY`, so runtime overlays are not an option —
CLAUDE.md's Rockchip rule). The 4SE's **analog** ES8316 path needs ASoC +
`SND_SOC_ES8316` + `SND_SOC_ROCKCHIP_I2S` + a simple-card/graph-card node, but
**no DRM**. nanopi-zero2 (RK3528) has neither HDMI nor an analog jack, so its
only audio is I2S/SPDIF off the FPC header. Scoped out of the first example and
tracked as gosd-lrxz.

## Measured size evidence (the crux of the fork)

All numbers are real bytes. The "stock" column is the **published**
`artifacts/v0.8.0` kernel — literally what `gosd build` puts on a card today —
and the "with audio" column is a local `gosd build-kernel` run of
gosd-y9hc's recipe against a real Docker daemon.

| Board | Stock (v0.8.0) | With audio | Delta |
|---|---|---|---|
| pi-zero-w (`kernel.img`, armv6 zImage) | 16,484,560 | 16,588,808 | **+104,248 (+0.63%)** |
| pi-zero-2w (`kernel8.img`, uncompressed arm64 Image) | 56,150,528 | 56,551,936 | **+401,408 (+0.71%)** |

The two agree at about two thirds of one percent; the absolute figures differ
because the Pi's armv6 `kernel.img` is a self-compressing zImage while arm64's
`kernel8.img` is an uncompressed Image. For scale, the whole published
`pi-zero-w.tar.zst` is 16,497,070 bytes — on that board the kernel *is* the
artifact.

Two comparisons that put that in context:

| Also measured, pi-zero-w | Bytes | vs stock |
|---|---|---|
| this recipe before the USB-MIDI-gadget deny-list | +15,424 on top | +119,672 |
| `examples/sattrack` (DRM + vc4 + its "minimal" sound) | 17,760,952 | +1,276,392 (+7.7%) |

The 15,424 bytes are the surprise of the exercise, and are not about sound at
all: they are the *USB MIDI gadget*. `USB_MIDI_GADGET` and
`USB_CONFIGFS_F_MIDI` depend on the raw-MIDI core, which cannot exist while
`CONFIG_SOUND` is off, so they sit dormant in every stock GoSD Pi kernel (the
gadget stack itself is on) and wake up the moment sound appears. Legacy gadget
drivers claim the board's only UDC at probe — precisely how "Gadget Zero" broke
`--usb-gadget` in bean gosd-spjt — so any Pi kernel that gains sound must deny
them or risk breaking USB gadget mode. Route B would have to carry that
deny-list into `build/boards/*/kernel.fragment`.

**A measurement trap worth recording:** the `gosd build-kernel` cache holds
outputs from months of bring-up work, and an old cache entry for a board is
*not* a valid stock baseline — the first attempt at this comparison used a
cached pi-zero-2w kernel that was 6.2 MB larger than the shipped one, which
would have made audio look like it *shrank* the kernel by 10%. Compare against
the published artifact, or build stock from current `main` yourself.

**The sattrack number is a cautionary tale, not the cost of sound.** Its
fragment enables `CONFIG_SOUND=y`/`SND=y`/`SND_SOC=y` to satisfy `DRM_VC4`'s
hard dependency, with a comment claiming "no codec, machine, or USB audio
drivers come with it". That is **false**: reading the built `kernel.config` out
of the cache, that three-line re-enable silently compiled in the entire
raspberrypi/linux audio ecosystem — ~60 HAT machine drivers (HiFiBerry x8,
IQaudio, JustBoom, Allo x5, AudioInjector x3, Pisound, Cirrus, DionAudio,
FE-Pi...), ~45 ASoC codec drivers, USB audio (`SND_USB_AUDIO`, UA101, Caiaq,
6fire, HiFace, Line6), the MIDI sequencer stack, OSS emulation,
`SND_DUMMY`/`SND_ALOOP`, and `SND_BCM2835` itself. Cause: the raspberrypi
defconfigs ship all of that as `=m` — 75 `CONFIG_SND*` lines in
`bcmrpi_defconfig`, 79 in `bcm2711_defconfig` — and GoSD kernels are monolithic
(`CONFIG_MODULES` always off), so `make olddefconfig` promotes every `=m` to
`=y` the moment `CONFIG_SND=y` appears. Precisely CLAUDE.md's "audit what a Pi
defconfig hands you" trap, hit a fourth time. Tracked as gosd-df57.

By contrast this recipe's built config carries exactly **ten** `CONFIG_SND*=y`
symbols on each board: the core (`SND`, `SND_TIMER`, `SND_PCM`,
`SND_PCM_TIMER`, `SND_DYNAMIC_MINORS`, `SND_PROC_FS`, `SND_VERBOSE_PROCFS`,
`SND_CTL_FAST_LOOKUP`), `SND_BCM2835`, and one empty menu symbol
(`SND_PCI` on arm64, with no PCI sound driver under it).

So the honest comparison is stock vs a *deliberate* sound config: ALSA core +
`snd_bcm2835` + an explicit deny-list (`# CONFIG_SND_SOC is not set` alone
kills every codec and machine driver, since ASoC gates them all).

## Pure-Go playback: chosen approach

**Chosen: talk the kernel's ALSA PCM ioctl ABI directly**, in ~250 lines
behind a small interface seam, using `golang.org/x/sys/unix` (already a
dependency) and `unsafe` only for the ioctl argument pointers. The minimal
blocking path is genuinely small: open `/dev/snd/pcmC<card>D<dev>p`, one
`SNDRV_PCM_IOCTL_HW_PARAMS` (which runs the same refine step internally, so
the separate `HW_REFINE` round-trip is optional), `SW_PARAMS`, `PREPARE`, then
`WRITEI_FRAMES` in a loop — playback auto-starts once `start_threshold` frames
are queued, so even `START` is unnecessary. `EPIPE` (xrun) recovery is a
re-`PREPARE`. No mmap, no poll. ABI pinned at `SNDRV_PCM_VERSION` 2.0.18
(`include/uapi/sound/asound.h`), which has been stable for over a decade.

The one real portability trap: `snd_pcm_uframes_t` is `unsigned long`, so
`snd_pcm_hw_params` is 608 bytes on arm64 and 604 on armv6 — **and the ioctl
number encodes `sizeof(struct)`**, so a hardcoded command number is wrong on
one of our two architectures. Fix: `uintptr` for every `*_t` field (Go's
`uintptr` is the native word width) and derive the ioctl size from
`unsafe.Sizeof`, so both `GOARCH=arm64` and `GOARCH=arm GOARM=6` are right by
construction; a unit test asserts the struct sizes/offsets on both widths.

**Rejected: `github.com/yobert/alsa`** (the only real pure-Go ALSA library —
no cgo, ioctl-direct, per-arch `alsatype/types_arm{,64}.go`, 33 importers, MIT).
It works and it was invaluable as a cross-check on the ABI, but: **no tagged
release ever** (consumers pin `@master` or a pseudo-version), last commit
2023-01-26, and its README self-discloses "makes syscalls with pointers to
memory buffers that are in garbage collectible memory... I have a feeling this
isn't safe, but it hasn't crashed on me yet". Same shape of dependency the disk
work turned down, for a surface we need only a fraction of.

**Rejected: OSS emulation** (`CONFIG_SND_PCM_OSS`, plain `write()` to
`/dev/dsp`). Still in-tree and still buildable, and genuinely simpler at the
call site — but it is a deprecated in-kernel translation shim doing
format/rate conversion for us, it adds kernel size rather than removing Go
code, and raspberrypi/linux has open issues about its interactions. Not worth
trading the native ABI for.

**Rejected: `ebitengine/oto` (formerly `hajimehoshi/oto`), `gen2brain/malgo`,
`ecobee/goalsa` (formerly `cocoonlife`).** malgo and goalsa are cgo. oto v3 is
subtler and worth recording: its Linux backend uses `purego` to `dlopen`
`libasound.so.2`, so it *does* compile with `CGO_ENABLED=0` — and is still
unusable here, because a GoSD image has no `libasound.so.2` on disk to open.
The disqualifier is "needs alsa-lib at runtime", not "uses cgo".

Decoding (MP3/Vorbis/FLAC) is deliberately out of scope: WAV/raw PCM is a
`encoding/binary` header parse, whereas decoders mean third-party deps
(`hajimehoshi/go-mp3` — pure Go but **archived** 2023, `jfreymuth/oggvorbis`,
`mewkiz/flac`). Tracked as gosd-nxm4.

## The fork — JP to choose

**Route A — audio stays an opt-in `gosd build-kernel` recipe.** Exactly the
precedent DRM set (`docs/custom-kernels.md`, `examples/sattrack`): the app that
wants sound ships a fragment, builds a kernel once, and uses
`gosd build --artifacts-dir`. No artifacts release, no size cost for the
apps that don't want audio, strictly additive, reversible.

**Route B — sound in the stock released kernels.** Any app gets `/dev/snd`
with no custom kernel. Costs: every board's image grows (measured above), an
`artifacts/vX.Y.Z` release plus the three-way verification dance in
`docs/artifacts.md`, and per-family judgement calls — on the Pis it would also
need `snd_bcm2835.enable_hdmi=1` baked into the boards' `cmdline.txt`
templates, and on Rockchip HDMI audio would drag in the whole DRM subsystem,
which is the very thing GoSD cut and put behind a recipe.

**Recommendation: Route A** — but with the size argument conceded, because
the measurement came out about ten times cheaper than the DRM precedent would
suggest. 0.64% of the most size-sensitive kernel we ship is not a reason to
say no to anything, and unlike DRM it buys a capability every board's hardware
actually has. If the decision rested on size alone, Route B would win on the
Pis.

It does not rest on size. Two things keep me on A:

1. **Rockchip makes Route B incoherent.** Their HDMI audio *requires* DRM, so
   a stock-kernel Route B either ships the whole DRM subsystem to every board
   — contradicting the decision that put DRM behind a recipe in the first
   place — or ships "audio" that means HDMI on Pi, analog-only on ROCK 4SE,
   nothing at all on radxa-zero-3e (HDMI-only hardware, so DRM or silence) and
   nothing possible on nanopi-zero2. A feature row that needs four different
   footnotes to explain what it means is worse than an opt-in recipe.
2. **It is not just three Kconfig lines per board.** Route B on the Pis also
   needs `snd_bcm2835.enable_hdmi=1` (or `dtparam=audio=on`, which is how the
   downstream `bcm2710-*` DTBs implement exactly that) in the boards'
   `cmdline.txt`/`config.txt` templates, the USB-MIDI-gadget deny-list above
   or `--usb-gadget` breaks, and an artifacts release with the three-way
   verification in `docs/artifacts.md`.

If JP wants Route B anyway, the honest scope is "Pi boards get `snd_bcm2835`
in stock kernels, Rockchip stays recipe-only", and the fragment written for
gosd-y9hc is exactly what gets promoted into `build/boards/*/kernel.fragment`,
plus the cmdline change. Nothing in Route A has to be undone for that: it is a
promotion, not a rewrite.

**Route B is JP's call, tracked as gosd-ette.** Route A needs no decision, so
it ships first.

## Children

- gosd-y9hc — `examples/chime`: the example + Pi custom-kernel recipe (Route A). **Implemented first.**
- gosd-ette — **JP to choose**: Route B, sound in the stock kernels.
- gosd-df57 — bug: sattrack's fragment silently compiles in the whole Pi audio zoo, and says it doesn't.
- gosd-lrxz — Rockchip audio coverage: rock-4se analog (ES8316) and Rockchip HDMI-over-DRM.
- gosd-aptt — qemu-virt audio (virtio-sound) + a boot-to-sound CI smoke test.
- gosd-mf3a — no surface for extra `config.txt` lines / kernel cmdline params in `gosd build`.
- gosd-nxm4 — a public `audio/` package and decoders, if audio outgrows one example.
- gosd-tjrw — audio capture (mic in on the 4SE jack, I2S mics).

## Found on the way, unrelated to audio

- gosd-dkqb (bug, high) — pi-zero-w's shipped DTB has `spi@7e204000`
  `status = "disabled"` and **no `__overrides__` node at all**, so
  `dtparam=spi=on` in its `config.txt` is a silent no-op: `/dev/spidev0.*`
  never appears and COMPATIBILITY.md's SPI ✅ for that board is wrong. Turned
  up because the same missing `__overrides__` block is why `dtparam=audio=on`
  could not have been the mechanism for audio on that board.
