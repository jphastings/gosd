---
# gosd-r775
title: 'sattrack polish: fixed-right label, black-text/white-stroke, arc-length dashes, ±30min, circle stroke'
status: completed
type: task
priority: normal
created_at: 2026-07-13T00:34:55Z
updated_at: 2026-07-13T03:39:26Z
---

JP's post-merge review feedback on [[gosd-e9fy]] (PR #75), 2026-07-13. App-only: the kernel recipe under examples/sattrack/kernel/ MUST NOT change (JP: check in before any kernel change); verification reuses the cached qemu-virt kernel.

## Locked decisions (supersede the corresponding [[gosd-e9fy]] rules)

1. **Label placement**: always horizontally to the RIGHT of the circle (left-aligned, vertically centered on the circle center). The perpendicular/trailing-side rule is retired.
2. **Label style**: BLACK glyphs with a 1px WHITE outline at 50% opacity (amended live by JP 2026-07-13; was "thin white stroke" at full opacity — too heavy against the glyph weight). Built as a single outline mask (glyph coverage dilated 1px, 8-neighbor max, minus the glyph mask) composited exactly once at alpha 128 with the black glyphs on top, so overlaps can't stack past 50%. Legible on ocean, land, ice, and the red line alike.
3. **Dashes — uniform length without whole-line redraws**: parameterize by cumulative arc length along the drawn polyline, anchored at an absolute-time reference fixed at each full redraw (startup / TLE refresh). Samples land on fixed 10s epoch boundaries, so between full redraws every sample's position and cumulative arc length s are immutable; pixel painted iff (s mod 28) < 16 (16px dash, 12px gap, measured along-track — uniform for steep and shallow segments alike). Window slide appends/consumes segments but never alters s of painted ones → no dash crawl, no repaint of the existing line. Antimeridian: the split's wrap jump contributes 0 to s (arc length accumulates over drawn segments only).
4. **Window**: past 30 min / future 30 min (was ±60). Fade scales proportionally: the oldest ~5 minutes ramp alpha 255→0 (flag in the PR for JP — he didn't specify the fade share; a constant to flip if he prefers 10 min).
5. **Circle**: thin black stroke (~2px) around the outside of the red circle.

## Verification

- Unit tests updated/added: fixed-right label anchor + alignment; dash predicate — (a) same absolute samples ⇒ identical on/off after the window slides, (b) synthetic steep vs shallow polylines produce along-track dash runs of 16px ±1px, (c) antimeridian split doesn't inject phantom arc length. Fade window constant. Bounded dirty-rect test still holds.
- Real proof with the CACHED qemu-virt kernel (no rebuild): gosd run --display --artifacts-dir, two screendumps ~60s apart showing right-side black/white-stroked label, stroked circle, uniform dashes on both steep and shallow track portions, and no dash movement between frames. Record what they show here.

### Results (2026-07-13)

- **Cached-kernel proof**: Image copied from kernel-build cache entry
  60d570e19... into a flat --artifacts-dir; no Docker started, no kernel
  rebuilt, nothing under examples/sattrack/kernel/ touched. `gosd run
  --display` booted, TLE for 68498 ("IO-1") fetched, mode 1280x800.
- **Screendumps** (sattrack-polish-1/2.png, ~60s apart, kept in the agent
  scratchpad, not committed): +-30min track (visibly shorter than the
  +-1h screenshots on PR #75); "IO-1" in black glyphs with the amended
  1px 50%-white outline, fixed to the right of the circle and vertically
  centered; red circle with a 2px black stroke that cleanly overpaints
  the track line entering it; dash rhythm uniform along the steep
  descent near the circle and the shallow sweep across Antarctica alike.
  Between the two dumps the circle/label advanced south and the head
  extended - and a pixel-level comparison of the dashed region (all 919
  red pixels at x<500) found ZERO changed pixels: no dash crawl, no
  incidental repaints.
- **Unit tests**: along-track run lengths within 16+-widening on both a
  horizontal and a vertical synthetic line (the retired x-keyed pattern
  fails the vertical case by construction); dashed-segment display-list
  items are identical across a 1s window slide (<=3 moving-tip items may
  differ); the antimeridian wrap adds zero arc length and the phase step
  resumes unchanged after the split; fixed-right label geometry for
  eastbound and southbound travel; label mask has black glyphs plus a
  half-opaque outline and no outline pixel above 50% (guards the
  offset-stamp accumulation pitfall); fade constants 1800/300 with the
  ramp confined to the oldest five minutes; the bounded dirty-rect test
  still holds.
- **Gates**: go test/vet, gofmt, golangci-lint on darwin and GOOS=linux
  all clean; arm64 and arm/GOARM=6 cross-builds pass.
- Note: synthetic test frames now anchor subpoints to absolute time
  (refEpoch) exactly like SGP4 output; the original helpers anchored to
  the sliding 'now', which understated per-tick churn.
- Quality gates + both cross-compiles (arm64, arm/GOARM=6).

## Summary of Changes

- examples/sattrack/render.go: label fixed right of the circle
  (labelRect replaces the velocity-normal placement machinery); label
  mask rebuilt as black glyphs + 1px dilated-outline white at alpha 128
  composited once; dashes parameterized by cumulative along-track arc
  length via a persistent per-sample table (futurePhases/segmentArc)
  anchored at each full redraw, wrap segments contributing zero;
  window/fade constants 1800/300 (fadeWindowSec is the flip-to-600
  knob); 2px black circle stroke with a fixed segments-circle-label
  z-order per dirty rect.
- examples/sattrack/track.go: frame.lookahead removed (only the retired
  label rule used it); window comments now say 30min.
- examples/sattrack/main.go + README.md: copy updated for the new
  window, dash, label, and circle styling.
- examples/sattrack/render_test.go: x-keyed dash and perpendicular-label
  tests replaced per the bean; synthetic frames now anchored to absolute
  time (refEpoch) like real SGP4 output.
- No changes outside examples/sattrack/ app code; kernel recipe and
  runner untouched.

## Todos

- [x] Label: fixed-right placement + black/white-stroke rendering
- [x] Dashes: absolute-time-anchored arc-length parameterization
- [x] Window ±30min + proportional fade
- [x] Circle black stroke
- [x] Tests updated per Verification
- [x] Cached-kernel qemu proof + screendumps recorded here
- [x] Quality gates
