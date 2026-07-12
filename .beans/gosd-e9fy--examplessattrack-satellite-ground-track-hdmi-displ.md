---
# gosd-e9fy
title: 'examples/sattrack: satellite ground-track HDMI display (pi-zero-w + qemu-virt custom-kernel recipe)'
status: in-progress
type: feature
priority: normal
created_at: 2026-07-12T06:10:08Z
updated_at: 2026-07-12T10:50:13Z
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
- [ ] Blue Marble asset: fetch NASA original, downscale to 2048×1024 JPEG, embed, attribute
- [ ] App: TLE fetch/refresh + SGP4 + clock-plausibility wait
- [ ] App: DRM init (NeowayLabs/drm) + DirtyFB damage helper + no-display retry
- [ ] App: renderer — map/letterbox, past line + tail fade, x-keyed dashed future line, circle, label placement rule
- [ ] App: 1s partial-update loop with dirty rects == damage clips
- [ ] Unit tests per Verification
- [x] Kernel recipe: gosd-kernel.toml + both fragments + pi-zero-w DTS patch
- [ ] Proof: qemu-virt kernel build + gosd run --display screenshot; pi-zero-w kernel build
- [ ] README (usage, kernel commands, NASA attribution, armv6 notes) + quality gates
