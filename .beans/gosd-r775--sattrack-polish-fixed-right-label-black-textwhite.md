---
# gosd-r775
title: 'sattrack polish: fixed-right label, black-text/white-stroke, arc-length dashes, ±30min, circle stroke'
status: in-progress
type: task
priority: normal
created_at: 2026-07-13T00:34:55Z
updated_at: 2026-07-13T00:37:51Z
---

JP's post-merge review feedback on [[gosd-e9fy]] (PR #75), 2026-07-13. App-only: the kernel recipe under examples/sattrack/kernel/ MUST NOT change (JP: check in before any kernel change); verification reuses the cached qemu-virt kernel.

## Locked decisions (supersede the corresponding [[gosd-e9fy]] rules)

1. **Label placement**: always horizontally to the RIGHT of the circle (left-aligned, vertically centered on the circle center). The perpendicular/trailing-side rule is retired.
2. **Label style**: BLACK glyphs with a thin WHITE stroke around the outside (inverse of the shipped white-on-black), so the name is legible on ocean, land, ice, and the red line alike.
3. **Dashes — uniform length without whole-line redraws**: parameterize by cumulative arc length along the drawn polyline, anchored at an absolute-time reference fixed at each full redraw (startup / TLE refresh). Samples land on fixed 10s epoch boundaries, so between full redraws every sample's position and cumulative arc length s are immutable; pixel painted iff (s mod 28) < 16 (16px dash, 12px gap, measured along-track — uniform for steep and shallow segments alike). Window slide appends/consumes segments but never alters s of painted ones → no dash crawl, no repaint of the existing line. Antimeridian: the split's wrap jump contributes 0 to s (arc length accumulates over drawn segments only).
4. **Window**: past 30 min / future 30 min (was ±60). Fade scales proportionally: the oldest ~5 minutes ramp alpha 255→0 (flag in the PR for JP — he didn't specify the fade share; a constant to flip if he prefers 10 min).
5. **Circle**: thin black stroke (~2px) around the outside of the red circle.

## Verification

- Unit tests updated/added: fixed-right label anchor + alignment; dash predicate — (a) same absolute samples ⇒ identical on/off after the window slides, (b) synthetic steep vs shallow polylines produce along-track dash runs of 16px ±1px, (c) antimeridian split doesn't inject phantom arc length. Fade window constant. Bounded dirty-rect test still holds.
- Real proof with the CACHED qemu-virt kernel (no rebuild): gosd run --display --artifacts-dir, two screendumps ~60s apart showing right-side black/white-stroked label, stroked circle, uniform dashes on both steep and shallow track portions, and no dash movement between frames. Record what they show here.
- Quality gates + both cross-compiles (arm64, arm/GOARM=6).

## Todos

- [ ] Label: fixed-right placement + black/white-stroke rendering
- [ ] Dashes: absolute-time-anchored arc-length parameterization
- [ ] Window ±30min + proportional fade
- [ ] Circle black stroke
- [ ] Tests updated per Verification
- [ ] Cached-kernel qemu proof + screendumps recorded here
- [ ] Quality gates
