---
# gosd-y9hc
title: 'examples/chime: pure-Go audio playback with a Pi custom-kernel recipe'
status: completed
type: task
priority: high
created_at: 2026-07-29T21:45:24Z
updated_at: 2026-08-21T01:36:07Z
parent: gosd-qkbl
---

Route A from the parent epic gosd-qkbl: audio as an opt-in `gosd build-kernel`
recipe, following the `examples/sattrack` precedent exactly (CLAUDE.md names it
"the reference for a bigger example"). Strictly additive, no artifacts release,
no size cost to boards that don't want sound — so it needs no decision from JP
and ships first. If JP later picks Route B (gosd-ette), the fragment written
here is what gets promoted into `build/boards/*/kernel.fragment`.

## Locked decisions

- **Name:** `examples/chime`. It plays a short bundled chime on boot, then a
  test tone sweep on a timer, so "did audio work?" is answerable by ear from
  across the room.
- **Playback:** the kernel's ALSA PCM ioctl ABI, implemented in the example
  (~250 lines), not `github.com/yobert/alsa` — see the epic for the survey and
  why. `golang.org/x/sys/unix` only (already a dependency, and what
  sattrack's `display_linux.go` uses); no new third-party dependency.
- **Boards:** the three Pi boards, all sharing one fragment, because
  `snd_bcm2835` is a VCHIQ-bus driver needing no DTS patch and no ASoC and no
  DRM. Rockchip is gosd-lrxz; qemu-virt is gosd-aptt.
- **HDMI:** enabled by a one-line recipe patch defaulting the driver's
  `enable_hdmi` module parameter to true. The fragment-only route
  (`CONFIG_CMDLINE` + `CONFIG_CMDLINE_EXTEND`) was tried first and is
  **arch/arm-only** — arm64's command-line Kconfig has no `EXTEND` — so it
  failed the pi-zero-2w build. `dtparam=audio=on` would work on the two
  downstream-DTB boards but not pi-zero-w, and no example can write to
  `config.txt` anyway (gosd-mf3a).
- **The fragment's deny-list is not optional.** A bare
  `CONFIG_SOUND=y`/`SND=y` promotes ~60 HAT machine drivers, ~45 codecs, USB
  audio, MIDI and OSS emulation from the defconfig's `=m` to `=y` (measured;
  see the epic and gosd-df57). `# CONFIG_SND_SOC is not set` does most of the
  work.
- **Degradation:** mirror sattrack — no audio device means one actionable log
  line pointing at the README and `docs/custom-kernels.md`, retried on a
  timer, and the app never exits (so gosd-init's supervisor doesn't
  restart-churn it).
- **Audio content is synthesised, not bundled.** sattrack embeds two NASA
  JPEGs because there is no way to compute Blue Marble; a chime and a test
  tone are a few lines of `math.Sin`, so there is no asset, no licence
  question, and no image-size cost. Keeps the example stdlib-only.

## Todo

- [x] Write the Pi kernel fragment + `gosd-kernel.toml` recipe
- [x] `gosd build-kernel --board pi-zero-w` against a real Docker daemon; record the byte sizes in the epic
- [x] Verify the built `kernel.config` carries the sound core, `SND_BCM2835=y`, the cmdline params, and none of the denied symbols
- [x] Pure-Go ALSA PCM playback behind an interface seam, with `_linux.go`/`_other.go` split
- [x] Struct-size/offset test that pins the ioctl ABI on both word widths
- [x] Tone/chime synthesis with behavioural tests that run on macOS
- [x] Graceful degradation when there is no audio device, with an actionable message
- [x] `README.md` in the sattrack shape, and a `docs/runtime.md` pointer
- [x] COMPATIBILITY.md: footnote, not a row (justified in the summary)
- [x] Cross-compile for `GOARCH=arm64` and `GOARCH=arm GOARM=6`
- [x] All quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...` for darwin and linux

## Summary of Changes

`examples/chime` plays a chime and a periodic test-tone sweep out of a GoSD
board's HDMI (or analog, where the board has a jack) by talking the kernel's
ALSA PCM ioctl ABI directly — no alsa-lib, no cgo, no new dependency beyond
`golang.org/x/sys/unix`, which the tree already uses.

- `examples/chime/kernel/` is the `gosd build-kernel` recipe: one
  `pi.fragment` and one patch, shared by pi-zero-w, pi-zero-2w and pi-3b.
  Three fragment lines enable the ALSA core and `snd_bcm2835`; the rest of the
  fragment is a deny-list keeping the raspberrypi defconfigs' `=m` audio
  ecosystem from being promoted to `=y`; the patch defaults the driver's
  `enable_hdmi` parameter on, which is the only lever that works the same on
  all three boards.
- `alsa_linux.go` implements the minimal blocking playback path
  (`HW_PARAMS` → `SW_PARAMS` → `PREPARE` → `WRITEI_FRAMES`, auto-start via
  `start_threshold`, `EPIPE` → re-`PREPARE`), with `uintptr` for every
  `snd_pcm_uframes_t` field and the ioctl size derived from `unsafe.Sizeof`,
  so `GOARCH=arm` and `GOARCH=arm64` are both correct by construction.
  `alsa_other.go` keeps the package building and its pure logic testable on
  macOS.
- Device selection prefers an HDMI PCM (matched on the card/PCM id, the only
  signal the kernel gives) over an analog one, and logs which it picked.
- `tone.go` synthesises everything played: a two-note chime with an
  exponential decay envelope, and a log-swept sine. Tested behaviourally
  (amplitude decay, frequency content by zero-crossing count, no clipping) on
  the host.

**Measured**, published `artifacts/v0.8.0` kernel vs a local
`gosd build-kernel` run of this recipe:

| Board | Stock | With audio | Delta |
|---|---|---|---|
| pi-zero-w (`kernel.img`, armv6 zImage) | 16,484,560 | 16,588,808 | **+104,248 (+0.63%)** |
| pi-zero-2w (`kernel8.img`, uncompressed arm64 Image) | 56,150,528 | 56,551,936 | **+401,408 (+0.71%)** |

For contrast, `examples/sattrack`'s DRM recipe costs +1,276,392 (+7.7%) on
pi-zero-w — and most of *that* turns out to be an accidental audio zoo rather
than DRM (gosd-df57). An intermediate build of this fragment, before the
USB-MIDI-gadget deny-list, came out 15,424 bytes larger than the final one.
Don't baseline against an old `gosd build-kernel` cache entry: a stale cached
pi-zero-2w kernel was 6.2 MB *larger* than the shipped one, which would have
made audio look like a 10% saving.

## Three things learned that outlived the task

1. `snd_bcm2835` is a VCHIQ *bus* driver, so Pi audio needs no DT node, no
   ASoC and no DRM — and it works precisely because GoSD never loads vc4 KMS
   (upstream makes the firmware driver stand down when it is present).
2. `CONFIG_CMDLINE_EXTEND` exists on arch/arm and not on arm64. A Kconfig
   fragment that merges cleanly on one Pi board can be invalid on another.
3. `gosd-kernel.toml`'s `patches` are applied with a plain `patch -p1` at the
   kernel tree root — not restricted to device trees, despite the docs' wording
   (now clarified in `docs/custom-kernels.md`). That is the only portable way a
   recipe can change a module parameter's default in a monolithic kernel.

Boards: pi-zero-w and pi-zero-2w are **build-proven** — this exact recipe
built via `gosd build-kernel` against a real Docker daemon for both, with the
patch applying cleanly and each resulting `kernel.config` carrying
`CONFIG_SND_BCM2835=y`, exactly ten `CONFIG_SND*` symbols, and every denied
symbol absent. A pi-zero-w image was also assembled end to end
(`gosd build --artifacts-dir`) and its boot partition checked. pi-3b shares
the fragment, the patch, the defconfig and the driver, but was not compiled. No board has been hardware-verified —
no GoSD board has had audio on a bench yet, and HDMI card creation depends on
the firmware seeing a live display at boot, so that is the first thing to
check when one does.

COMPATIBILITY.md gets a footnote on the display row's neighbourhood rather
than a new feature row: like DRM, audio is not a board *capability* GoSD
ships or withholds per board — every board's stock kernel has no sound, and
any of them can have it via a recipe. A row would have to read "no" for all
seven boards, which says less than the footnote does.


Status left at in-progress pending review in PR #132, matching how gosd-yggd
rode its PR.

### Closed 2026-08-21

PR #132 merged; the "pending review" hold above no longer applies.
`examples/chime` (app, tone synthesis, kernel recipe and README) is on `main`,
and the recipe has since grown Rockchip fragments under gosd-lrxz. The ALSA
PCM path written here was subsequently promoted out of the example into the
public `sound/` package. No hardware gate belonged to this bean: audio's
first bench pass is the epic's business, and COMPATIBILITY.md carries the
footnote rather than a per-board row, as justified above.
