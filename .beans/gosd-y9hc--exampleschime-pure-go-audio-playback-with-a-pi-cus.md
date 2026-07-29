---
# gosd-y9hc
title: 'examples/chime: pure-Go audio playback with a Pi custom-kernel recipe'
status: in-progress
type: task
priority: high
created_at: 2026-07-29T21:45:24Z
updated_at: 2026-07-29T21:46:40Z
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
- **HDMI:** enabled by `CONFIG_CMDLINE="snd_bcm2835.enable_hdmi=1"` +
  `CONFIG_CMDLINE_EXTEND=y` in the fragment, because the module parameter
  defaults to off and an example cannot edit the board's `cmdline.txt`
  (gosd-mf3a).
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

- [ ] Write the Pi kernel fragment + `gosd-kernel.toml` recipe
- [ ] `gosd build-kernel --board pi-zero-w` against a real Docker daemon; record the byte sizes in the epic
- [ ] Verify the built `kernel.config` carries the sound core, `SND_BCM2835=y`, the cmdline params, and none of the denied symbols
- [ ] Pure-Go ALSA PCM playback behind an interface seam, with `_linux.go`/`_other.go` split
- [ ] Struct-size/offset test that pins the ioctl ABI on both word widths
- [ ] Tone/chime synthesis with behavioural tests that run on macOS
- [ ] Graceful degradation when there is no audio device, with an actionable message
- [ ] `README.md` in the sattrack shape, and a `docs/runtime.md` pointer
- [ ] COMPATIBILITY.md: footnote, not a row (justified in the summary)
- [ ] Cross-compile for `GOARCH=arm64` and `GOARCH=arm GOARM=6`
- [ ] All quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...` for darwin and linux

