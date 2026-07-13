---
# gosd-sa3g
title: 'sattrack: day/night terminator — Black Marble night base, daylight-masked Blue Marble, strip-updated twilight'
status: in-progress
type: feature
priority: normal
created_at: 2026-07-13T00:52:00Z
updated_at: 2026-07-13T04:26:35Z
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

## Todos

- [ ] Black Marble asset (downscale, embed, attribute)
- [ ] Solar ephemeris + daylight alpha
- [ ] Lit-map composite (startup full pass; row/column caches)
- [ ] 60s terminator strip refresh (analytic row ranges, in-place patch, overlay re-stamp, dirty rects)
- [ ] Tests per Verification
- [ ] Cached-kernel qemu proof + screendumps recorded here
- [ ] README update (new asset attribution, day/night description) + quality gates
