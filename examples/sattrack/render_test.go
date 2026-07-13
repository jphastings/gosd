package main

import (
	"image"
	"image/color"
	"math"
	"testing"
	"time"
)

func testRenderer(t *testing.T, size image.Point) *renderer {
	t.Helper()
	day := image.NewRGBA(image.Rect(0, 0, 64, 32))
	night := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for i := range day.Pix {
		day.Pix[i] = 0x80
		night.Pix[i] = 0x10
	}
	r, err := newRenderer(size, day, night)
	if err != nil {
		t.Fatalf("newRenderer: %v", err)
	}
	return r
}

func TestProjectMapsEquirectangularCorners(t *testing.T) {
	r := testRenderer(t, image.Pt(400, 200)) // exactly 2:1, no bars

	if got := r.project(90, -180); got != image.Pt(0, 0) {
		t.Errorf("project(90,-180) = %v, want (0,0)", got)
	}
	if got := r.project(0, 0); got != image.Pt(200, 100) {
		t.Errorf("project(0,0) = %v, want (200,100)", got)
	}
	if got := r.project(-90, 179.99); (got.X < 399) || got.Y != 200 {
		t.Errorf("project(-90,179.99) = %v, want near (400,200)", got)
	}
}

func TestProjectLetterboxesNonTwoToOneModes(t *testing.T) {
	r := testRenderer(t, image.Pt(640, 480)) // 4:3 mode -> 640x320 map, bars top+bottom

	want := image.Rect(0, 80, 640, 400)
	if r.mapRect != want {
		t.Fatalf("mapRect = %v, want %v", r.mapRect, want)
	}
	if got := r.project(90, -180); got != want.Min {
		t.Errorf("project(90,-180) = %v, want %v", got, want.Min)
	}
}

func TestAntimeridianSegmentsAreNeverDrawn(t *testing.T) {
	r := testRenderer(t, image.Pt(400, 200))
	now := time.Unix(1_800_000_000, 0)

	f := frame{
		sat: trackPoint{t: now, lat: 0, lon: 179},
		past: []trackPoint{
			{t: now.Add(-20 * time.Second), lat: 0, lon: 178},
			{t: now.Add(-10 * time.Second), lat: 0, lon: 179},
			{t: now, lat: 0, lon: -179}, // wraps: must not draw 179 -> -179
		},
		future: []trackPoint{
			{t: now, lat: 0, lon: -179},
			{t: now.Add(10 * time.Second), lat: 0, lon: -178},
		},
	}

	segs := 0
	for it := range r.buildItems(f, true) {
		if it.kind == kindSegment {
			segs++
		}
	}
	// 2 candidate past segments minus the wrapping one, plus 1 future.
	if segs != 2 {
		t.Errorf("got %d segments, want 2 (the antimeridian-straddling segment must be dropped)", segs)
	}
}

// refEpoch anchors synthetic tracks to absolute time, exactly like real
// SGP4 output: a sample at a given absolute instant has the same subpoint
// whichever tick's frame it appears in.
const refEpoch = 1_800_000_000

// lineFrame builds a frame whose future track is a straight line from
// (lat0, lon0) at refEpoch advancing by (dLat, dLon) degrees per 10s
// sample, with the satellite (and a degenerate past line) parked in a far
// corner so scans of the future line see only the dashes.
func lineFrame(now time.Time, lat0, lon0, dLat, dLon float64) frame {
	at := func(ts time.Time) trackPoint {
		n := ts.Sub(time.Unix(refEpoch, 0)).Seconds() / sampleStepSec
		return trackPoint{t: ts, lat: lat0 + dLat*n, lon: normalizeLon(lon0 + dLon*n)}
	}
	pts := []trackPoint{at(now)}
	grid := now.Truncate(sampleStepSec * time.Second).Add(sampleStepSec * time.Second)
	end := now.Add(trackWindowSec * time.Second)
	for ; grid.Before(end); grid = grid.Add(sampleStepSec * time.Second) {
		pts = append(pts, at(grid))
	}
	pts = append(pts, at(end))

	corner := trackPoint{t: now, lat: -85, lon: -175}
	return frame{
		sat:    corner,
		past:   []trackPoint{{t: now.Add(-sampleStepSec * time.Second), lat: -85, lon: -175}, corner},
		future: pts,
	}
}

// runLengths scans a horizontal or vertical pixel line for runs of the
// track red, returning the lengths of complete on-runs and the gaps
// between them (partial runs at either end are discarded).
func runLengths(img *image.RGBA, from, to image.Point) (on, off []int) {
	isRed := func(p image.Point) bool {
		c := img.RGBAAt(p.X, p.Y)
		return c.R > 100 && int(c.R) > int(c.G)+int(c.B)
	}
	step := image.Pt(sign(to.X-from.X), sign(to.Y-from.Y))
	type run struct {
		red bool
		n   int
	}
	var runs []run
	for p := from; p != to; p = p.Add(step) {
		red := isRed(p)
		if len(runs) == 0 || runs[len(runs)-1].red != red {
			runs = append(runs, run{red: red})
		}
		runs[len(runs)-1].n++
	}
	if len(runs) < 3 {
		return nil, nil
	}
	for _, ru := range runs[1 : len(runs)-1] { // drop partial runs at the scan edges
		if ru.red {
			on = append(on, ru.n)
		} else {
			off = append(off, ru.n)
		}
	}
	return on, off
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// The dash pattern is parameterized by along-track arc length, so a steep
// (here: vertical) line and a shallow (horizontal) line must both show the
// locked 16px-on/12px-off rhythm measured along the track. The retired
// x-keyed pattern fails this vertically (constant x = one endless run).
func TestDashRunsAreUniformForSteepAndShallowTracks(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	half := 1 // thickness 3 at 400px height -> stamps widen runs by ~2*half
	onLo, onHi := dashOnPx-2, dashOnPx+2*half+2
	offLo, offHi := dashPeriodPx-dashOnPx-2*half-2, dashPeriodPx-dashOnPx+2

	check := func(t *testing.T, on, off []int) {
		t.Helper()
		if len(on) < 6 || len(off) < 6 {
			t.Fatalf("too few dash runs to judge: %d on, %d off", len(on), len(off))
		}
		for _, n := range on {
			if n < onLo || n > onHi {
				t.Errorf("on-run of %dpx, want %d..%d (all: %v)", n, onLo, onHi, on)
				break
			}
		}
		for _, n := range off {
			if n < offLo || n > offHi {
				t.Errorf("gap of %dpx, want %d..%d (all: %v)", n, offLo, offHi, off)
				break
			}
		}
	}

	t.Run("shallow", func(t *testing.T) {
		r := testRenderer(t, image.Pt(800, 400))
		r.render(lineFrame(now, 0, -80, 0, 1)) // horizontal along the equator
		y := r.project(0, 0).Y
		on, off := runLengths(r.back, image.Pt(r.project(0, -70).X, y), image.Pt(r.project(0, 70).X, y))
		check(t, on, off)
	})
	t.Run("steep", func(t *testing.T) {
		r := testRenderer(t, image.Pt(800, 400))
		r.render(lineFrame(now, 80, 20, -1, 0)) // vertical down lon 20
		x := r.project(0, 20).X
		on, off := runLengths(r.back, image.Pt(x, r.project(70, 20).Y), image.Pt(x, r.project(-70, 20).Y))
		check(t, on, off)
	})
}

// Sliding the window by a tick must not change the phase (or anything
// else) of dashed segments already on screen: their samples sit on fixed
// absolute epochs and their arc lengths live in a table that is only ever
// appended to between full redraws.
func TestDashPhasesAreStableAcrossWindowSlides(t *testing.T) {
	r := testRenderer(t, image.Pt(800, 400))
	now := time.Unix(1_800_000_003, 0) // off-grid on purpose

	prev := r.buildItems(lineFrame(now, 0, -80, 0, 1), true)
	next := r.buildItems(lineFrame(now.Add(time.Second), 0, -80, 0, 1), false)

	var reused, fresh int
	for it := range next {
		if !it.dashed {
			continue
		}
		if _, ok := prev[it]; ok {
			reused++
		} else {
			fresh++
		}
	}
	if reused < 100 {
		t.Errorf("only %d dashed segments survived the slide unchanged; nearly all should", reused)
	}
	// Only the two moving tips (and at most one newly appended interior
	// segment) may differ.
	if fresh > 3 {
		t.Errorf("%d dashed segments changed across a 1s slide, want <= 3 (the moving tips)", fresh)
	}
}

// The antimeridian wrap contributes zero arc length: the dash phase
// continues seamlessly from the segment before the split to the segment
// after it.
func TestAntimeridianInjectsNoPhantomArcLength(t *testing.T) {
	r := testRenderer(t, image.Pt(800, 400))
	now := time.Unix(1_800_000_000, 0)
	f := lineFrame(now, 0, 170, 0, 0.5) // crosses 180 heading east

	ss := r.futurePhases(f, true)
	wrapIdx := -1
	for i := 1; i < len(f.future); i++ {
		if wraps(f.future[i-1], f.future[i]) {
			wrapIdx = i
			break
		}
	}
	if wrapIdx < 2 || wrapIdx > len(f.future)-2 {
		t.Fatalf("expected an interior antimeridian split, got index %d", wrapIdx)
	}
	if ss[wrapIdx] != ss[wrapIdx-1] {
		t.Errorf("arc length jumped %f -> %f across the wrap, want no change", ss[wrapIdx-1], ss[wrapIdx])
	}
	step := ss[wrapIdx-1] - ss[wrapIdx-2]
	if after := ss[wrapIdx+1] - ss[wrapIdx]; math.Abs(after-step) > 1 {
		t.Errorf("arc-length step after the wrap = %f, want ~%f (the pre-wrap step)", after, step)
	}
}

// The 30-minute window keeps full opacity until its oldest five minutes,
// which ramp linearly to zero at the tip.
func TestFadeAlphaRampsOnlyInTheOldestFiveMinutes(t *testing.T) {
	if trackWindowSec != 1800 || fadeWindowSec != 300 {
		t.Fatalf("window constants = %d/%d, want 1800/300 (locked: past 30min, ~5min fade)", trackWindowSec, fadeWindowSec)
	}
	if a := fadeAlpha(0); a != 255 {
		t.Errorf("fadeAlpha(0) = %d, want 255", a)
	}
	if a := fadeAlpha(trackWindowSec - fadeWindowSec); a != 255 {
		t.Errorf("fadeAlpha at the fade-window boundary = %d, want 255", a)
	}
	if a := fadeAlpha(trackWindowSec); a != 0 {
		t.Errorf("fadeAlpha at the oldest tip = %d, want 0", a)
	}
	mid := fadeAlpha(trackWindowSec - fadeWindowSec/2)
	if mid < 120 || mid > 135 {
		t.Errorf("fadeAlpha mid-window = %d, want ~127", mid)
	}
	if !(fadeAlpha(trackWindowSec-50) < fadeAlpha(trackWindowSec-200)) {
		t.Error("fade is not monotonic toward the oldest tip")
	}
}

// The label always sits horizontally to the right of the circle:
// left-aligned just past the stroked ring, vertically centered on the
// circle center - regardless of the direction of travel.
func TestLabelSitsFixedRightOfCircle(t *testing.T) {
	r := testRenderer(t, image.Pt(400, 200))
	r.setName("IO-1")
	now := time.Unix(1_800_000_000, 0)

	for _, f := range []frame{
		syntheticFrame(now, 0.02),        // travelling east
		lineFrame(now, 60, -20, -0.5, 0), // travelling due south
	} {
		var found bool
		center := r.project(f.sat.lat, f.sat.lon)
		for it := range r.buildItems(f, true) {
			if it.kind != kindLabel {
				continue
			}
			found = true
			if wantX := center.X + r.radius + circleStroke + r.labelGap; it.a.X != wantX {
				t.Errorf("label left edge at x=%d, want %d (fixed right of the circle)", it.a.X, wantX)
			}
			if mid := (it.a.Y + it.b.Y) / 2; abs(mid-center.Y) > 1 {
				t.Errorf("label vertical middle at y=%d, want ~%d (centered on the circle)", mid, center.Y)
			}
		}
		if !found {
			t.Fatal("no label item in the display list")
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// The label image is black glyphs with a 1px white outline at 50% opacity
// (not the retired white-on-black, and never accumulating past 50%).
func TestLabelIsBlackGlyphsWithHalfOpaqueWhiteOutline(t *testing.T) {
	r := testRenderer(t, image.Pt(400, 200))
	r.setName("IO-1")

	var black, outline, tooOpaqueWhite int
	b := r.label.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := r.label.RGBAAt(x, y) // premultiplied: whiteness has RGB == A
			whitish := c.R == c.G && c.G == c.B && c.R > 0 && c.R >= c.A-10
			switch {
			case c.A > 150 && c.R < 50 && c.G < 50 && c.B < 50:
				black++
			case whitish && c.A > 60 && c.A <= labelOutlineAlpha+10:
				outline++
			case whitish && c.A > labelOutlineAlpha+10:
				tooOpaqueWhite++
			}
		}
	}
	if black == 0 || outline == 0 {
		t.Errorf("label has %d dark and %d half-opaque outline pixels; want both (black glyphs, 50%% white stroke)", black, outline)
	}
	if tooOpaqueWhite > 0 {
		t.Errorf("%d outline pixels exceed 50%% opacity; offset-stamp accumulation is not allowed", tooOpaqueWhite)
	}
}

// syntheticFrame builds a frame for a fake satellite gliding east along a
// fixed latitude at rate degrees of longitude per second (crossing lon -60
// at refEpoch), sampled the same way computeFrame samples the real one.
func syntheticFrame(now time.Time, rate float64) frame {
	at := func(ts time.Time) trackPoint {
		dt := ts.Sub(time.Unix(refEpoch, 0)).Seconds()
		return trackPoint{t: ts, lat: 10, lon: normalizeLon(-60 + rate*dt)}
	}
	window := func(from, to time.Time) []trackPoint {
		pts := []trackPoint{at(from)}
		grid := from.Truncate(sampleStepSec * time.Second)
		if !grid.After(from) {
			grid = grid.Add(sampleStepSec * time.Second)
		}
		for ; grid.Before(to); grid = grid.Add(sampleStepSec * time.Second) {
			pts = append(pts, at(grid))
		}
		return append(pts, at(to))
	}
	return frame{
		sat:    at(now),
		past:   window(now.Add(-trackWindowSec*time.Second), now),
		future: window(now, now.Add(trackWindowSec*time.Second)),
	}
}

func TestSteadyTickRepaintsABoundedRegionNotTheFullFrame(t *testing.T) {
	r := testRenderer(t, image.Pt(800, 400))
	r.setName("TESTSAT 1")
	now := time.Unix(1_800_000_003, 0) // deliberately off the 10s grid

	first := r.render(syntheticFrame(now, 0.02))
	if len(first) != 1 || first[0] != r.back.Bounds() {
		t.Fatalf("first render dirty = %v, want the full frame", first)
	}

	dirty := r.render(syntheticFrame(now.Add(time.Second), 0.02))
	area := 0
	for _, rect := range dirty {
		area += rect.Dx() * rect.Dy()
	}
	full := r.back.Bounds().Dx() * r.back.Bounds().Dy()
	if area == 0 {
		t.Fatal("second render dirtied nothing; the satellite moved, so something must repaint")
	}
	if area > full/4 {
		t.Errorf("second render dirtied %d px of %d (%.0f%%); a steady tick must repaint a bounded region, not the frame", area, full, 100*float64(area)/float64(full))
	}

	// The static middle of the past line must not repaint: probe a sample
	// well inside the window - older than the moving head, younger than
	// the fade window.
	probe := r.project(10, normalizeLon(-60+0.02*(-float64(trackWindowSec)/2)))
	for _, rect := range dirty {
		if probe.In(rect) {
			t.Errorf("dirty rect %v covers the static middle of the past line at %v", rect, probe)
		}
	}
}

func TestCircleIsRedWithBlackStrokeOverTheTrack(t *testing.T) {
	r := testRenderer(t, image.Pt(400, 200))
	r.setName("X")
	now := time.Unix(1_800_000_000, 0)
	r.render(syntheticFrame(now, 0.02))

	center := r.project(10, -60)
	got := r.back.RGBAAt(center.X, center.Y)
	want := color.RGBA{R: trackRed.R, G: trackRed.G, B: trackRed.B, A: 0xff}
	if got != want {
		t.Errorf("pixel at the satellite position = %v, want the opaque track red %v", got, want)
	}

	// The ring sits just outside the red disc; probe above the center,
	// clear of the roughly horizontal track line and the right-side label.
	ring := r.back.RGBAAt(center.X, center.Y-(r.radius+1))
	if (ring != color.RGBA{A: 0xff}) {
		t.Errorf("pixel on the circle's stroke = %v, want opaque black", ring)
	}
}
