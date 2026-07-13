---
# gosd-sa3g
title: 'sattrack: day/night terminator — Black Marble night base, daylight-masked Blue Marble, strip-updated twilight'
status: completed
type: feature
priority: normal
created_at: 2026-07-13T00:52:00Z
updated_at: 2026-07-13T04:49:07Z
---

JP's request 2026-07-13, building on [[gosd-e9fy]] + [[gosd-r775]]: render night-side Earth with NASA's Black Marble (city lights) and show the Blue Marble day texture only where it's currently daytime, with a realistic fading twilight band at the terminator. App-only: no kernel changes (JP's standing constraint); verification reuses the cached qemu-virt kernel.

## Locked decisions

1. **Asset**: NASA Black Marble equirectangular night-lights image, downscaled to 2048×1024 JPEG (~1-1.5MB) with the same pipeline as the Blue Marble, embedded via go:embed, public-domain + attributed (README + code comment).
2. **Compositing model**: the pristine map buffer becomes a *lit map*: per pixel, lerp(night, day, A) where A is the daylight alpha from solar altitude — A=1 at altitude ≥ 0°, A=0 at ≤ −9°, linear ramp between (both thresholds named constants, flagged for JP's taste at review). Night texture is the base ('behind'); day shows through where A>0, twilight is the semi-transparent blend band. All overlay erase operations read from the lit map exactly as they read from the static map today.
3. **Sun position**: standard low-precision solar ephemeris (declination + equation of time), pure Go, ~0.01° accuracy — no dependency.
4. **Per-pixel efficiency**: sin(altitude) = sin(lat)sin(dec) + cos(lat)cos(dec)cos(lon − lon_sun). Cache per-row sin/cos(lat) and per-column lon; a lighting pass is one multiply-add + compare per pixel. Full-frame compose only at startup (and TLE-refresh full redraws, which already repaint everything).
5. **The terminator moves (~1px per ~80s at map scale) — partial updates preserved**: a terminator refresh every 60s (constant) recomputes A ONLY inside the union of the old and new twilight strips (thresholds widened by the sun's 60s motion as margin). Strip row-ranges are found analytically per column (solve sin(alt)=threshold for lat — two arcsin branches), never by full-frame scanning; polar day/night (no crossing in a column) falls out naturally. The lit-map buffer is patched in place, any track segments / circle / label intersecting the strips are re-stamped on top, and the strips join that tick's dirty rects. Full-frame recomposites happen ONLY at startup/TLE refresh, same as today.
6. armv6 budget: startup full compose is a one-off (~tens of ms at 1280×800); per-minute strip updates are bounded thin curves.
7. **Label glyphs: track red with a 1px solid black outline** (JP live amendments 2026-07-13, in two steps: glyphs black→track-red referencing the same trackRed constant the lines/circle use, then the outer stroke white-50%→solid black "like the circle"). Opacity stays a named constant (labelOutlineAlpha) so flipping back to a 50% stroke is a one-constant change if full opacity proves too heavy. Supersedes gosd-r775's black-glyphs/white-stroke label.
8. **Track window live-tuned by JP to ±45min with a 10-minute fade** (trackWindowSec=2700, fadeWindowSec=600, edited directly by JP 2026-07-13 during this task; supersedes gosd-r775's ±30min/5min).

## Verification

- Unit tests: solar ephemeris sanity (equinox/solstice declination, subsolar longitude vs UTC noon); alpha ramp endpoints/monotonicity; per-column strip row-range solver incl. polar no-crossing cases; strip-update leaves pixels outside the strips byte-identical (partial-update guarantee); track re-stamp inside a refreshed strip.
- Real proof with the CACHED qemu-virt kernel: screendumps showing night side with city lights, day side, and a soft terminator band; a second dump ≥2 min later showing the terminator advanced while dashes/track outside the strips did not repaint. Record here.
- Quality gates + both cross-compiles.

### Results (2026-07-13)

- **Cached-kernel proof**: Image from kernel-build cache entry 60d570e19...
  via flat --artifacts-dir; no Docker, no kernel rebuild, nothing under
  examples/sattrack/kernel/ touched. `gosd run --display` booted, TLE
  for 68498 ("IO-1") fetched, mode 1280x800.
- **Screendumps** (sattrack-daynight-1/2.png, ~2min10s apart, kept in the
  agent scratchpad, not committed): night side over the Americas with
  Black Marble city lights clearly visible (US coasts, Brazil), Blue
  Marble day side over Europe/Africa/Asia/Australia, soft twilight
  gradients at both terminators, Antarctic polar night dark on the
  night-side half - geometry consistent with the July sun (dec ~+22).
  Label is track-red glyphs with the 1px solid black outline, right of
  the black-ringed circle. Pixel-level comparison across the gap (which
  spans two 60s terminator refreshes): all 645 red dash pixels in the
  deep-night Antarctic sweep byte-identical (no repaint outside strips),
  while 5527 non-track pixels changed inside the morning terminator band
  (the strip relight advanced it).
- **Unit tests**: ephemeris vs equinox/solstice declination and
  06/12/18 UTC subsolar longitudes; alpha ramp endpoints, ~127 midpoint,
  monotonicity; strip solver vs a brute-force oracle across four sun
  geometries including the arcsin-domain (|k|>R) polar branch; gold test
  - after a 61s tick the lit map equals a from-scratch full compose at
  the new time AND every changed pixel lies inside a reported dirty rect
  (bounded <35% of frame, actual far less); a vertical track crossing
  the terminator stays exactly trackRed after a strip refresh (re-stamp
  proof).
- **Gates**: go test/vet, gofmt, golangci-lint darwin+GOOS=linux clean;
  arm64 and arm/GOARM=6 cross-builds pass.
- Shipped solver variant (per the implementation-notes latitude): pure
  analytic arcsin brackets + midpoint interval classification, no
  full-frame or per-row scanning; the per-column result is the bounding
  interval of in-band intervals (anything between two twilight bands is
  saturated identically under both suns, so relighting it rewrites
  identical bytes - the gold test pins this).

## Summary of Changes

- examples/sattrack/blackmarble.jpg: NASA Black Marble (Earth at Night
  2016, visibleearth.nasa.gov/images/144898) downscaled 3600x1800 ->
  2048x1024 (137KB), embedded and attributed like the Blue Marble.
- examples/sattrack/sun.go (new): NOAA low-accuracy solar ephemeris
  (declination + equation of time -> subsolar point), the daylight alpha
  ramp (0 deg / -9 deg named constants), and the per-threshold arcsin
  latitude-root solver.
- examples/sattrack/render.go: the erase source is now a lit map -
  lerp(night, day, alpha) with both scaled textures resident; full
  compose only at startup/invalidate (cached per-row trig, per-column
  longitudes, asin only inside the twilight band); a 60s terminator
  refresh patches only the strip rows each column needs (analytic
  brackets + midpoint classification, 64-column chunk rects hugging the
  curve) and feeds the strips into the existing z-ordered dirty-rect
  repaint, so track/circle/label re-stamp automatically. Label glyphs
  are now trackRed with a 1px solid black outline (JP's live
  amendments); window constants live-tuned by JP to 2700/600.
- examples/sattrack/main.go + README.md: second embed + decode, new
  description and attribution, 45min copy.
- Tests: sun_test.go (ephemeris, ramp, solver-vs-oracle, gold
  strip-refresh test, terminator re-stamp) plus label/fade test updates.
- No changes outside examples/sattrack/ app code; kernel recipe and
  runner untouched.

## Todos

- [x] Black Marble asset (downscale, embed, attribute)
- [x] Solar ephemeris + daylight alpha
- [x] Lit-map composite (startup full pass; row/column caches)
- [x] 60s terminator strip refresh (analytic row ranges, in-place patch, overlay re-stamp, dirty rects)
- [x] Tests per Verification
- [x] Cached-kernel qemu proof + screendumps recorded here
- [x] README update (new asset attribution, day/night description) + quality gates
