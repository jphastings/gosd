---
# gosd-e9fy
title: 'examples/sattrack: satellite ground-track HDMI display (pi-zero-w + qemu-virt custom-kernel recipe)'
status: completed
type: feature
priority: normal
created_at: 2026-07-12T06:10:08Z
updated_at: 2026-07-13T00:13:24Z
---

A worked HDMI-display example: `examples/sattrack` renders a fullscreen Blue
Marble world map with a live satellite ground track, updating once per second
with partial redraws, plus the custom-kernel recipe (via [[gosd-47rm]]'s
`gosd build-kernel`) that makes it run on a Raspberry Pi Zero W and under
qemu-virt. Demonstrates the "display apps as a custom-kernel recipe, not base
kernel" stance (JP, 2026-07-11) and that armv6-class hardware is enough.

## Locked decisions — app behavior

- **Satellite**: NORAD id **68498** by default, overridable via env
  `SATTRACK_NORAD_ID` (settable through `gosd.toml [env]`). TLE fetched from
  `https://tle.ivanstanojevic.me/api/tle/<id>` (JSON: name, line1, line2)
  with retry/backoff at startup (network may come up after the app starts —
  gosd-init supervises; just keep retrying) and refreshed every 6h; on
  refresh failure keep the old TLE and log. SGP4 propagation via the pure-Go
  `github.com/joshuaferrara/go-satellite` (JP: use the appropriate package
  for clarity + real function). Wait until the clock is plausible (SNTP may
  land after start; e.g. time after 2024-01-01) before first propagation.
- **Map**: NASA Blue Marble, embedded via `go:embed` as a ~2048×1024 JPEG
  (~1.5MB), downscaled from a public-domain NASA original with
  `golang.org/x/image/draw`; source URL + attribution in README and a code
  comment. Equirectangular projection: lon −180→180 maps left→right, lat
  90→−90 top→bottom, letterboxed (2:1) centered on the actual mode, black
  bars.
- **Current position**: filled red circle (radius ≈ max(6px, height/90)),
  drawn over the track lines.
- **Name label**: satellite name from the TLE, rendered with the embedded Go
  font (`golang.org/x/image/font/gofont` + opentype, ~height/34 px, white
  with 1px black outline for legibility), **always on top** (draw order: map
  → lines → circle → label). Placement (JP's rule, 2026-07-11): offset from
  the circle **perpendicular to the direction of travel on screen**, on the
  trailing (solid-line) side — with screen-space velocity (vx, vy) (y down),
  the offset direction is the normal **n = (vy, −vx)** normalized; anchor =
  circle center + n·(circleRadius + labelGap). Horizontal alignment follows
  the offset: n.x > 0 → left-aligned (text extends rightward, away from the
  circle), n.x < 0 → right-aligned. Worked example locked by JP: path going
  down-right, having come from top-left → label above-right, left-aligned.
- **Past 1 hour**: solid red line, thickness ≈ max(3px, height/240),
  sampled every 10s (360 points). It **fades out only at the end**: full
  opacity except the oldest ~10 minutes, whose alpha ramps 255→0 toward the
  oldest tip (fade as a function of sample age). Consequence for partial
  updates: per tick only (a) the segment nearest the satellite (new) and
  (b) the fade window at the tail change — the long middle is static.
- **Future 1 hour**: red **dashed** line, same thickness, sampled every 10s.
  Dash on/off is a function of **screen x only** (locked: e.g. 16px dash,
  12px gap → pixel drawn iff (x mod 28) < 16), NOT of arc distance from the
  circle — so as the track advances, existing dashes don't crawl and never
  need repainting; only the far tip (t+1h, new segment) and the region near
  the satellite (dashed→solid conversion as the satellite consumes the
  path) change per tick. Accepted tradeoff (JP): near-vertical track
  portions render with degenerate dashing.
- **Update cadence**: 1s tick. **Partial updates only**: keep a full-screen
  RGBA backbuffer; each tick recompute, repaint only dirty regions (around
  the old+new circle and label, the future line's far tip, the dashed→solid
  handoff near the circle, the past line's fade window + dropped oldest
  segment), then flush exactly those rects to the display. Full redraw only
  at startup, on TLE refresh (paths shift), and on mode discovery.
- **Antimeridian**: split any polyline segment whose endpoints differ by
  >180° of longitude (track wraps east↔west edge); no drawing across the
  wrap.
- **No display present** (stock kernel): log one actionable message pointing
  at this example's README + `docs/custom-kernels.md`, then retry every 60s
  (don't exit — avoids supervisor restart churn).

## Locked decisions — display stack

- **DRM dumb buffers via `github.com/NeowayLabs/drm`** (JP's rule: use the
  external package if good — verified 2026-07-11: compiles for linux/arm64
  AND linux/arm GOARM=6, complete legacy-modeset API: GetResources,
  GetConnector, CreateFB, MapDumb, AddFB, SetCrtc; stale (2019) but it
  targets the frozen legacy-KMS ABI). fb0/fbdev is NOT used.
- **Hand-rolled `DRM_IOCTL_MODE_DIRTYFB` helper** (~30 lines on
  `x/sys/unix`, in the example): NeowayLabs/drm lacks it, and **virtio-gpu
  does not scan out continuously** — without damage flushes the qemu display
  never updates. The example's dirty rects ARE the damage clips; flush them
  per tick. vc4 ignores excess flushes harmlessly. Single dumb FB, drawn
  into directly (1 Hz small-rect updates make tearing a non-issue).
- Pick the first connected connector + its preferred mode; no hotplug
  handling.

## Locked decisions — kernel recipe (in `examples/sattrack/kernel/`)

- `gosd-kernel.toml` with `[kernel.pi-zero-w]` (fragment + DTS patch) and
  `[kernel.qemu-virt]` (fragment).
- **pi-zero-w fragment**: CONFIG_DRM=y, CONFIG_DRM_VC4=y, CONFIG_CMA=y,
  CONFIG_DMA_CMA=y, CONFIG_CMA_SIZE_MBYTES=64 (baked in — no cmdline
  surface needed), no fbdev emulation, no fbcon (appliance: kernel text
  never splashes on the display), no sound. **DTS patch**: enable the vc4
  display pipeline on bcm2835-rpi-zero-w (port the effect of the rpi tree's
  vc4-kms-v3d overlay for pi0: &vc4/&hvs/&pixelvalves/&hdmi status=okay —
  discover the exact node set from the overlay source in the pinned tree,
  arch/arm/boot/dts/overlays/). This is the same patch mechanism the
  Rockchip peripheral enablement uses; it must apply against the pinned
  raspberrypi/linux commit.
- **qemu-virt fragment**: CONFIG_DRM=y, CONFIG_DRM_VIRTIO_GPU=y (virtio
  core is already built in).
- Hardware truth in docs: pi-zero-w display is **build-proven only** (no
  hardware bring-up yet — same caveat as the whole board, bean gosd-et0q
  lineage); qemu-virt is run-proven.

## Locked decisions — runner support (same PR; the example is untestable without it)

- `internal/qemurun`: always attach `-device virtio-gpu-pci` (headless
  guests just get an unused /dev/dri; harmless).
- `gosd run` gains `--display`: switches qemu from `-display none` (or
  -nographic) to the host's default display so the map appears in a local
  qemu window. Default behavior unchanged.
- `scripts/qemu-run.sh`: attach the same device; opt-in window via
  `QEMU_DISPLAY=1` (CI stays headless).

## Verification (record results here)

### Results (2026-07-13)

- **Unit tests**: 12 behavioral tests in examples/sattrack (macOS, no
  display): equirectangular projection incl. letterbox + corner cases;
  antimeridian segments dropped; dash predicate is x-only with the locked
  16/28 boundaries; fade ramps only in the oldest 10 min; JP's label
  worked example + its mirror; steady tick dirties a bounded region
  (<25% asserted, actual far less) and never the static past-line middle;
  TLE fetch parse/404/missing-lines; consecutive frames share the
  absolute 10s sample grid.
- **Cross-compile**: linux/arm64 and linux/arm GOARM=6 both build
  (CGO_ENABLED=0); go-satellite, NeowayLabs/drm, x/image all verified on
  both arches before implementation.
- **qemu-virt: RUN-PROVEN.** Custom kernel built via gosd build-kernel
  (local colima; CONFIG_DRM=y + CONFIG_DRM_VIRTIO_GPU=y confirmed in the
  emitted kernel.config, no fbcon/logo). `gosd run --display`: TLE for
  68498 ("IO-1") fetched in-guest, mode 1280x800, monitor screendumps
  captured (kept in the agent scratchpad, deliberately not committed):
  full Blue Marble letterboxed map, solid past track, x-keyed dashed
  future track wrapping cleanly at the antimeridian, red circle, outlined
  IO-1 label on the correct side; a second dump 60s later shows the
  circle/label advanced, the track head extended, and existing dashes
  unmoved (no crawl).
- **pi-zero-w: BUILD-PROVEN.** gosd build-kernel completed: fragment
  merged, DTS patch applied against the pinned raspberrypi/linux commit,
  zImage + bcm2835-rpi-zero-w.dtb compiled, required-y assertions passed.
  kernel.config carries DRM/DRM_VC4/SOUND/SND/SND_SOC=y,
  CMA_SIZE_MBYTES=64, FBDEV_EMULATION/CEC unset, fbcon/LOGO absent;
  dtc readback of the emitted DTB shows vc4/hvs/pixelvalves/hdmi/txp/v3d
  all status="okay". No hardware run (blocked on gosd-s4t4).
- NORAD 68498 exists at the TLE API today ("IO-1", sun-synchronous
  ~97.8 deg) - the shipped default needed no substitute.

## Summary of Changes

- `examples/sattrack/`: the app (main/tle/track/render + display_linux
  with the DRM legacy modeset + hand-rolled DIRTYFB, display_other stub),
  embedded 2048x1024 Blue Marble JPEG (312KB, attributed), README, and 12
  behavioral unit tests that run displayless on macOS.
- `examples/sattrack/kernel/`: gosd-kernel.toml + pi-zero-w.fragment
  (DRM_VC4 + minimal sound core + CMA=64MB, no fbdev/fbcon/logo/CEC) +
  qemu-virt.fragment (DRM_VIRTIO_GPU) +
  patches/0001-enable-vc4-hdmi.patch (pins the vc4 pipeline okay on the
  upstream-style Zero W DTS).
- `internal/qemurun`: always attach virtio-gpu-pci (romfile=); new
  Options.Display swaps -nographic for -serial mon:stdio with qemu's
  default host display backend. `gosd run --display` flag;
  scripts/qemu-run.sh + internal/cmd/qemuboot honor QEMU_DISPLAY=1.
  Defaults unchanged (CI stays headless).
- `cmd/gosd/buildkernel.go`: applyOverlayAssertions - developer overlay
  =y lines win over a board's ForbiddenY and are asserted as RequiredY
  (finding 3 below).
- New deps: NeowayLabs/drm, joshuaferrara/go-satellite, x/image,
  x/crypto/x509roots/fallback - all pure Go, verified for arm64 and
  armv6.

### Findings against locked decisions (corrections, recorded not relitigated)

1. **DIRTYFB ioctl nr is 0xB1, not 0xB5.** The bean's "IOWR('d', 0xB5,
   sizeof)" is DRM_IOCTL_MODE_GETPLANERESOURCES, whose struct is the
   same size - the wrong nr returns success while no damage ever reaches
   the device (black screen under virtio-gpu). Diagnosed via qemu
   -trace virtio_gpu_cmd_* plus guest drm.debug; include/uapi/drm/drm.h
   confirms 0xB1.
2. **"No sound" is impossible with DRM_VC4** at the pinned
   raspberrypi/linux commit: drivers/gpu/drm/vc4/Kconfig has
   "depends on SND && SND_SOC" (HDMI audio is not severable), so the
   pi-zero-w fragment re-enables a minimal SOUND/SND/SND_SOC core (no
   codec/machine/USB drivers). Without it, olddefconfig silently drops
   DRM_VC4.
3. **qemu-virt's kernelspec ForbiddenY forbids CONFIG_DRM**, which made
   any DRM overlay fail its assertion - contradicting
   docs/custom-kernels.md's "a line in your fragment always wins".
   Fixed at the cmd layer (cmd/gosd/buildkernel.go,
   applyOverlayAssertions): overlay CONFIG_FOO=y lines are removed from
   ForbiddenY and added to RequiredY (so a dropped developer symbol now
   fails loudly by name). internal/kernelspec itself untouched.
4. **The upstream-style bcm2835-rpi-zero-w.dts already enables most of
   the vc4 pipeline** (nodes carry no status gate; &hdmi is okay at the
   board level), unlike the downstream bcm2708 DTS the vc4-kms-v3d
   overlay targets. The shipped DTS patch pins the discovered node set
   (vc4, hdmi, v3d, txp, hvs, 3x pixelvalve - hvs/pixelvalves by full
   path, they have no labels upstream) to status="okay" explicitly:
   faithful to the overlay's effect, defensive rather than load-bearing.
   The overlay's &fb/&i2c2/CMA fragments have no equivalent or are
   already satisfied (CMA via CONFIG_CMA_SIZE_MBYTES=64).
5. **GoSD images ship no CA bundle**, so the locked https:// TLE URL
   fails x509 verification on-device. The example blank-imports
   golang.org/x/crypto/x509roots/fallback (Mozilla roots, pure Go).
   Suggested follow-up: document the pattern in docs/runtime.md's
   networking section (new bean; not done here).

Also noted: qemu screendump proof used `gosd run --display` with
--qemu-arg=-monitor unix socket; the cocoa window path and the headless
path exercise the same virtio-gpu surface.

- Unit tests (behavioral, macOS, no display): projection incl. antimeridian
  split; dash-phase function (same x → same on/off regardless of segment);
  label side/alignment rule (JP's worked example as a test case); fade
  alpha; dirty-rect accumulation (a tick far from wrap touches a bounded
  pixel area, NOT the full frame).
- Cross-compile: example builds for arm64 AND arm/GOARM=6 (CI smoke build
  covers examples).
- **Real proof**: build the qemu-virt custom kernel via
  `gosd build-kernel --config examples/sattrack/kernel/gosd-kernel.toml`
  (local colima), then `gosd run --display --artifacts-dir <out>
  ./examples/sattrack` shows the map + moving satellite; capture a
  screenshot (qemu monitor `screendump`) and record. Build the pi-zero-w
  custom kernel to prove the fragment + DTS patch apply and compile; no
  hardware run (blocked on bring-up kit, gosd-s4t4).

## Todos

- [x] Runner: virtio-gpu device + `gosd run --display` + qemu-run.sh env gate
- [x] Blue Marble asset: fetch NASA original, downscale to 2048×1024 JPEG, embed, attribute
- [x] App: TLE fetch/refresh + SGP4 + clock-plausibility wait
- [x] App: DRM init (NeowayLabs/drm) + DirtyFB damage helper + no-display retry
- [x] App: renderer — map/letterbox, past line + tail fade, x-keyed dashed future line, circle, label placement rule
- [x] App: 1s partial-update loop with dirty rects == damage clips
- [x] Unit tests per Verification
- [x] Kernel recipe: gosd-kernel.toml + both fragments + pi-zero-w DTS patch
- [x] Proof: qemu-virt kernel build + gosd run --display screenshot; pi-zero-w kernel build
- [x] README (usage, kernel commands, NASA attribution, armv6 notes) + quality gates
