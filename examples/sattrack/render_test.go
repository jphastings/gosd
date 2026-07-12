package main

import (
	"image"
	"image/color"
	"testing"
	"time"
)

func testRenderer(t *testing.T, size image.Point) *renderer {
	t.Helper()
	src := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for i := range src.Pix {
		src.Pix[i] = 0x40
	}
	r, err := newRenderer(size, src)
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
		sat:       trackPoint{t: now, lat: 0, lon: 179},
		lookahead: trackPoint{t: now.Add(10 * time.Second), lat: 0, lon: -179.5},
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
	for it := range r.buildItems(f) {
		if it.kind == kindSegment {
			segs++
		}
	}
	// 2 candidate past segments minus the wrapping one, plus 1 future.
	if segs != 2 {
		t.Errorf("got %d segments, want 2 (the antimeridian-straddling segment must be dropped)", segs)
	}
}

func TestDashPhaseIsAFunctionOfScreenXOnly(t *testing.T) {
	for x := -100; x < 100; x++ {
		want := ((x%dashPeriodPx)+dashPeriodPx)%dashPeriodPx < dashOnPx
		if got := dashOn(x); got != want {
			t.Fatalf("dashOn(%d) = %v, want %v", x, got, want)
		}
	}
	// Spot-check the locked 16-on/12-off pattern boundaries.
	if !dashOn(0) || !dashOn(15) || dashOn(16) || dashOn(27) || !dashOn(28) {
		t.Error("dash pattern is not the locked 16-on/12-off keyed to x")
	}
}

func TestFadeAlphaRampsOnlyInTheOldestTenMinutes(t *testing.T) {
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
	if !(fadeAlpha(3500) < fadeAlpha(3200)) {
		t.Error("fade is not monotonic toward the oldest tip")
	}
}

// JP's locked worked example: a path going down-right, having come from
// top-left, puts the label above-right of the circle, left-aligned.
func TestLabelPlacementWorkedExample(t *testing.T) {
	center := image.Pt(100, 100)
	rect := labelPlacement(center, [2]float64{1, 1}, image.Pt(60, 12), 8, 4)

	if rect.Min.X < center.X {
		t.Errorf("label rect %v is not left-aligned to the right of the circle center %v", rect, center)
	}
	if rect.Max.Y > center.Y {
		t.Errorf("label rect %v is not above the circle center %v", rect, center)
	}
}

func TestLabelPlacementMirrorsOnOppositeTravel(t *testing.T) {
	center := image.Pt(100, 100)
	// Up-left travel: n = (vy,-vx) = (-1,1)/sqrt2 -> below-left, right-aligned.
	rect := labelPlacement(center, [2]float64{-1, -1}, image.Pt(60, 12), 8, 4)

	if rect.Max.X > center.X {
		t.Errorf("label rect %v is not right-aligned to the left of the circle center %v", rect, center)
	}
	if rect.Min.Y < center.Y {
		t.Errorf("label rect %v is not below the circle center %v", rect, center)
	}
}

// syntheticFrame builds a frame for a fake satellite gliding east along a
// fixed latitude at rate degrees of longitude per second, sampled the same
// way computeFrame samples the real one.
func syntheticFrame(now time.Time, rate float64) frame {
	at := func(ts time.Time) trackPoint {
		dt := ts.Sub(now).Seconds()
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
		sat:       at(now),
		lookahead: at(now.Add(sampleStepSec * time.Second)),
		past:      window(now.Add(-trackWindowSec*time.Second), now),
		future:    window(now, now.Add(trackWindowSec*time.Second)),
	}
}

func TestSteadyTickRepaintsABoundedRegionNotTheFullFrame(t *testing.T) {
	r := testRenderer(t, image.Pt(800, 400))
	r.setName("TESTSAT 1")
	now := time.Unix(1_800_000_003, 0) // deliberately off the 10s grid

	first := r.render(syntheticFrame(now, 0.008))
	if len(first) != 1 || first[0] != r.back.Bounds() {
		t.Fatalf("first render dirty = %v, want the full frame", first)
	}

	dirty := r.render(syntheticFrame(now.Add(time.Second), 0.008))
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
	probe := r.project(10, normalizeLon(-60+0.008*(-1800)))
	for _, rect := range dirty {
		if probe.In(rect) {
			t.Errorf("dirty rect %v covers the static middle of the past line at %v", rect, probe)
		}
	}
}

func TestRenderDrawsCircleOverLinesAndLabelOnTop(t *testing.T) {
	r := testRenderer(t, image.Pt(400, 200))
	r.setName("X")
	now := time.Unix(1_800_000_000, 0)
	r.render(syntheticFrame(now, 0.008))

	center := r.project(10, -60)
	got := r.back.RGBAAt(center.X, center.Y)
	want := color.RGBA{R: trackRed.R, G: trackRed.G, B: trackRed.B, A: 0xff}
	if got != want {
		t.Errorf("pixel at the satellite position = %v, want the opaque track red %v", got, want)
	}
}
