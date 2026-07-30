---
# gosd-lrxz
title: 'Rockchip audio: rock-4se analog (ES8316) and Rockchip HDMI-over-DRM'
status: in-progress
type: task
priority: normal
created_at: 2026-07-29T21:45:25Z
updated_at: 2026-07-30T09:40:00Z
parent: gosd-qkbl
---

Deferred from gosd-qkbl: Rockchip HDMI audio needs the DRM dw-hdmi i2s codec,
so it drags in DRM; the 4SE analog path needs ASoC but no DRM.

JP's ask, verbatim: *"Let's add optional support for sound (particularly for
the rock 4se, but make sure it's easy and documented for all boards, you can
choose whether it's easier to explain in docs/sound.md or whether it needs an
example project too)"* — with the answer to "which of those" being **all
three**: board enablement, a public package, and the doc. Sound stays opt-in
via `gosd build-kernel` (Route A); Route B remains JP's open choice in
`gosd-ette` and is not pre-empted here.

## Findings that changed the plan

**1. Neither Rockchip board needs a device-tree patch.** This is the big one,
and it inverts the epic's assumption. `rk3399-rock-4se.dts` includes
`rk3399-rock-pi-4.dtsi`, which already:

- enables `i2c1` and hangs the codec off it —
  `es8316: codec@11 { compatible = "everest,es8316"; reg = <0x11>;
  clocks = <&cru SCLK_I2S_8CH_OUT>; }` with an audio-graph `port` endpoint;
- enables `i2s0` with the matching endpoint (`dai-format = "i2s"`,
  `mclk-fs = <256>`);
- declares the analog card itself:
  `sound { compatible = "audio-graph-card"; label = "Analog"; dais = <&i2s0_p0>; }`;
- and sets `&hdmi`, `&hdmi_sound` and `&i2s2` to `"okay"` for the HDMI path.

`rk3566-radxa-zero-3.dtsi` does the same for the Zero 3E (`&hdmi`,
`&hdmi_sound`, `&i2s0_8ch` all `"okay"`). `rk3399-t.dtsi` touches nothing
audio- or display-related (CPU/GPU OPP tables only). So the enablement is
**Kconfig-only on both boards**.

The codec's bus is `i2c1`, *not* one of the header buses (i2c2/i2c6/i2c7) our
existing DTS patches enable — i2c1 is already `"okay"` in mainline because the
codec and PMIC live there, so nothing to do.

**2. Consequence — the recipe belongs in the example, not in
`build/boards/rock-4se/kernel/patches/`.** The question the plan posed (board
dir vs example recipe) is moot in the direction that made it easy: with no DTS
change there is nothing to put in the board directory, so there is no artifacts
release, no `internal/artifacts.Version` bump, and no tag-first dance. The
Kconfig fragments live in `examples/chime/kernel/`, exactly like the Pi
fragment, and stay consistent with Route A. (Had a patch been needed, the same
answer would still have held: an opt-in recipe *can* carry a DTS patch, since
`gosd-kernel.toml`'s `patches` are applied as a plain `patch -p1` at the kernel
root.)

**3. `CONFIG_SND_SOC_ROCKCHIP` no longer exists** at the pinned tag
(`v6.18.37`): `sound/soc/rockchip/Kconfig` opens with a plain
`menu "Rockchip"`, not a gating symbol. The arm64 defconfig still carries
`CONFIG_SND_SOC_ROCKCHIP=m` — a stale line — which is a good reminder that a
defconfig entry is not proof a symbol exists.

**4. RK3399 and RK3566 need different I2S drivers.** RK3399's `i2s0/1/2` are
`compatible = "rockchip,rk3399-i2s", "rockchip,rk3066-i2s"` → `rockchip_i2s.c`
→ `CONFIG_SND_SOC_ROCKCHIP_I2S`. RK3566's `i2s0_8ch` is
`"rockchip,rk3568-i2s-tdm"` → `rockchip_i2s_tdm.c` →
`CONFIG_SND_SOC_ROCKCHIP_I2S_TDM`. Likewise the display controller:
`ROCKCHIP_VOP` for RK3399, `ROCKCHIP_VOP2` for RK3566. Both boards' HDMI is the
classic Synopsys dw-hdmi (`rockchip,rk3399-dw-hdmi` /
`rockchip,rk3568-dw-hdmi`), **not** the RK3588 "QP" variant, so
`DRM_DW_HDMI_I2S_AUDIO` is right for both.

**5. Two different generic card drivers coexist on the 4SE.** The analog card
is an `audio-graph-card` (`CONFIG_SND_AUDIO_GRAPH_CARD`); `hdmi_sound` in
`rk3399-base.dtsi` is a `simple-audio-card` (`CONFIG_SND_SIMPLE_CARD`) with
`i2s2` as its CPU DAI and the `hdmi` node as its codec. The analog-only recipe
therefore denies `SND_SIMPLE_CARD`, and the HDMI recipe enables both.

**6. nanopi-zero2 has no audio path at all**, confirmed rather than assumed:
`rk3528.dtsi` at v6.18.37 defines no `i2s`, `spdif`, `hdmi` or `vop` node —
only pin-mux *groups* named `i2s0`/`i2s1`/`spdif`/`hdmi` in
`rk3528-pinctrl.dtsi` with no device to attach them to. Documented as ➖, not
chased.

**7. SPDIF is defined but disabled on the 4SE.** `rk3399-rock-pi-4.dtsi`
declares a `sound-dit` audio-graph card and a `spdif-dit` codec, but never sets
`&spdif { status = "okay" }`. So the header's SPDIF_TX pins would need a DTS
patch after all — out of scope here, noted for whoever wants it.

## Locked decisions

- **Two recipes for rock-4se, not one.** `gosd-kernel.toml` gains a `rock-4se`
  entry using `rock-4se-analog.fragment` (ASoC + ES8316, no DRM); a new
  `hdmi.toml` carries `rock-4se-hdmi.fragment` (both outputs, DRM) and
  `radxa-zero-3e-hdmi.fragment` (HDMI only). An app that wants a beep out of
  the jack must not pay for the DRM subsystem.
- **radxa-zero-3e is in scope** because the shape extends cleanly: same
  fragment-only enablement, no DTS patch, two symbol substitutions (finding 4).
- **The playback code becomes a public `sound/` package** — see below.
- **Fragments are mostly deny-list, generated from the pinned defconfig.**
  Every denied symbol traces to a `CONFIG_SND*`/`CONFIG_SOUND*` line in
  `arch/arm64/configs/defconfig` at v6.18.37, and each fragment carries the
  one-liner that regenerates that list, so it can be re-derived at a kernel
  bump instead of guessed. Unlike the Pi's fragment,
  `# CONFIG_SND_SOC is not set` cannot do the work — ASoC is what the codec
  needs — so the list is long: the arm64 defconfig is multiplatform and ships
  every vendor's ASoC platform drivers plus ~20 codecs as modules, all of which
  `olddefconfig` promotes to built-in once `CONFIG_SND_SOC=y` appears.
- **On Rockchip the deny-list is a size trim, not a correctness gate** (it is
  both on the Pi). The device tree decides which machine driver binds, so a
  stray vendor driver costs bytes, not behaviour. The `USB_MIDI_GADGET` denial
  is kept anyway: the arm64 defconfig doesn't ship it (the raspberrypi ones
  do), so it is a precaution against a future defconfig, and it documents the
  `gosd-spjt` failure mode where a legacy gadget driver claims the only UDC.

## New public API surface (semver commitment)

`sound/` joins `cmd/gosd`, `gadget/`, `emmc/` and `disk/` as semver-relevant
public API, added to CLAUDE.md's bullet in this PR. This closes the "a public
audio package" half of `gosd-nxm4` (decoders stay deferred there).

```go
func Open() (Device, error)
func OpenWith(Options) (Device, error)

type Options struct { Path string; Prefer Output; Format Format }
type Output int // Any | HDMI | Analog
type Format struct { Rate, Channels int }   // + FrameBytes(), String()
type Device interface { Play([]byte) error; Format() Format; Name() string; Close() error }
var ErrNoDevice = errors.New("no audio playback device")
```

Deliberately minimal and honest: open, describe, write frames, close. `Device`
is an interface because the implementation cannot exist off Linux **and** so an
app's own tests can fake it (`examples/chime` does). No decoders, no mixer, no
capture, no async pipeline — those are `gosd-nxm4` and `gosd-tjrw`. Device
enumeration (`Devices()`, à la `disk`) and any capability query were left out
too; nothing needs them yet.

`ErrNoDevice`'s message is treated as part of the API: it names
`gosd build-kernel` and `docs/sound.md`, and distinguishes "no `/dev/snd`, so
this kernel has no sound" from "sound is there but no card appeared" (HDMI
cable connected too late), because the fixes are unrelated. A unit test pins
both messages.

## Todo

- [x] Establish the ES8316's bus and address, and whether it needs enabling
      (i2c1, 0x11, already `"okay"` in mainline)
- [x] Establish the Kconfig set for the analog path, against the pinned tree
- [x] Establish the Kconfig set for the HDMI path, incl. what DRM drags in
- [x] Decide, and justify, where the enablement lives (example recipe — there
      is no DTS patch to place)
- [x] Write `rock-4se-analog.fragment`, `rock-4se-hdmi.fragment` and
      `radxa-zero-3e-hdmi.fragment` + the `hdmi.toml` variant recipe
- [x] Promote the example's ALSA playback into a public `sound/` package with
      the platform seam, actionable no-device error, and host-passing tests
- [x] `examples/chime` becomes a thin consumer; still cross-compiles for
      arm64 and `GOARCH=arm GOARM=6`
- [x] `docs/sound.md`, cross-referenced from `runtime.md`,
      `custom-kernels.md`, the root README and the example README
- [x] COMPATIBILITY.md: audio row/footnote distinguishes build-proven (Pi)
      from recipe-only (Rockchip)
- [x] All quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`,
      `golangci-lint run ./...` for darwin and linux
- [x] **Build `gosd build-kernel --board rock-4se` with the analog recipe** and
      record the byte size vs the published `artifacts/v0.8.0` kernel —
      **built 2026-07-30: 69,474,816 B vs stock 68,327,936 B = +1,146,880 B
      (+1.68%), and `# CONFIG_DRM is not set` in the result**
- [x] **Build the HDMI variant** (`--config .../hdmi.toml`) for rock-4se and
      record its delta — **built 2026-07-30: 76,282,368 B = +7,954,432 B
      (+11.64%). The variant split is vindicated: 7x the analog cost for the
      same audible result on a different connector.**
- [ ] Build `--board radxa-zero-3e --config .../hdmi.toml`
- [x] Verify each resulting `kernel.config` — **both rock-4se variants pass
      (2026-07-30)**: analog has `SND_SOC_ES8316=y`, `SND_SOC_ROCKCHIP_I2S=y`,
      `SND_AUDIO_GRAPH_CARD=y`, `# CONFIG_SND_SIMPLE_CARD is not set`,
      `# CONFIG_DRM is not set`, 61 `CONFIG_SND*=y` symbols (vs 254 un-denied);
      HDMI adds `DRM=y`, `ROCKCHIP_DW_HDMI=y`, `DRM_DW_HDMI_I2S_AUDIO=y`,
      `SND_SIMPLE_CARD=y`. Denied symbols absent in both, including
      `USB_MIDI_GADGET` (the UDC-stealing trap from gosd-spjt).
      radxa-zero-3e still unverified — not built.
- [x] Put the measured numbers in `docs/sound.md`'s table and in this bean

### Build status update (2026-07-30)

Both rock-4se recipes have since been **built and verified** (numbers above):
the fragments compile, produce the intended symbols, and the analog variant is
genuinely DRM-free. The container runtime was available after all — the earlier
session's `lima not found` was an environment artifact, not a missing daemon.
Only `radxa-zero-3e` remains unbuilt, and no board has been heard on hardware.

### Why the builds weren't done in the original PR

The session that wrote it had **no container runtime available**: `docker`
resolves but no daemon socket exists, Docker Desktop and OrbStack are not
installed, `podman` is absent, and `colima status` fails with
`lima not found`. CLAUDE.md rules out the remaining option (the `mini` SSH
docker context mounts empty directories). Rather than invent numbers, the
measurement todos above are left unchecked and `docs/sound.md` says
"not measured" per Rockchip row, with the commands to produce them. The Pi
figures quoted everywhere are the real ones from `gosd-y9hc`.

## Bench checklist (nothing here has been heard)

- [ ] ROCK 4SE, analog recipe: chime + test tone audible on the 3.5 mm jack
- [ ] ROCK 4SE: `dmesg`/serial shows the ES8316 probing on i2c1, and
      `/proc/asound/pcm` lists a card labelled `Analog`
- [ ] ROCK 4SE, HDMI recipe: tone audible over HDMI **with the cable attached
      before power-up**; check whether the card exists without a display
      attached (on the Pi it does not — unknown on Rockchip)
- [ ] ROCK 4SE, HDMI recipe: the jack still works from the same kernel, and
      `CHIME_OUTPUT=analog` picks it
- [ ] Radxa Zero 3E: tone audible over micro-HDMI
- [ ] A Pi board still plays after the `sound` package extraction (pi-zero-2w
      is the cheapest re-check)
- [ ] With `--usb-gadget`, confirm the gadget still binds on a sound-enabled
      Rockchip kernel (the MIDI-gadget precaution)

## Summary of Changes

Sound on the Rockchip boards, as an opt-in `gosd build-kernel` recipe, plus the
public package and the doc that make it usable anywhere.

- **`sound/`** — new public package: the ALSA PCM ioctl client extracted from
  `examples/chime` (`platform_linux.go` + `platform_other.go`, pure selection
  logic and error construction in `sound.go`, behavioural tests that pass on
  macOS, ABI struct-layout test pinned for both word widths). API above.
- **`examples/chime/kernel/`** — `rock-4se-analog.fragment` (ASoC + ES8316 +
  audio-graph card, no DRM), `rock-4se-hdmi.fragment` (adds simple-card + DRM +
  Rockchip VOP + dw-hdmi + `DRM_DW_HDMI_I2S_AUDIO`),
  `radxa-zero-3e-hdmi.fragment` (RK3566 variants of the same), and `hdmi.toml`
  as the second recipe so the DRM cost is opt-in within the opt-in.
- **`examples/chime`** — now a thin consumer of `sound`; new `CHIME_OUTPUT`
  env var exercises `Options.Prefer`.
- **`docs/sound.md`** — the single home: per-board table (outputs that
  physically exist, recipe to build, whether DRM comes along, size cost),
  build commands, the gotchas, the package with a worked snippet, an honest
  verification-status table, and how to measure the missing numbers.
- **Cross-references** from `docs/runtime.md` (its audio section now points
  here instead of half-duplicating it), `docs/custom-kernels.md`, the root
  README and `examples/chime/README.md`.
- **COMPATIBILITY.md** — the audio row keeps 🚧 for both Rockchip boards, and
  the footnote now spells out what each cell means: ✅ = a recipe a real
  `gosd build-kernel` run compiled (pi-zero-w, pi-zero-2w; pi-3b on shared
  fragment), 🚧 = written against the pinned source but never compiled
  (Rockchip), ➖ = no hardware path (nanopi-zero2). No board has been heard.
- **CLAUDE.md** — `sound/` added to the public-API bullet.

Branched from `gosd-y9hc`'s work, which merged as PR #132 while this was being
written, so it rebased onto `main` and targets `main` directly — no stack.

**Bench update 2026-07-30**: the analog path is hardware-verified — a tone from `examples/chime` was heard from the ROCK 4SE's 3.5 mm jack, after the audibility pass added in [[gosd-cfkd]] unmuted the ES8316 (both playback volumes were at 0 and the DAC->headphone mixer switches were off). HDMI variant still unheard.
