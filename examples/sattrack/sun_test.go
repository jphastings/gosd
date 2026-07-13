package main

import (
	"bytes"
	"image"
	"math"
	"testing"
	"time"
)

func declDeg(s sunPos) float64 { return math.Asin(s.sinDec) / degToRad }

func TestSunEphemerisMatchesKnownGeometry(t *testing.T) {
	equinox := sunAt(time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC))
	if d := declDeg(equinox); math.Abs(d) > 1 {
		t.Errorf("declination at the March equinox = %.2f deg, want ~0", d)
	}
	if lon := equinox.lon / degToRad; math.Abs(lon) > 3 {
		t.Errorf("subsolar longitude at 12:00 UTC = %.2f deg, want ~0 (+/- equation of time)", lon)
	}

	june := sunAt(time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC))
	if d := declDeg(june); math.Abs(d-23.44) > 0.3 {
		t.Errorf("declination at the June solstice = %.2f deg, want ~23.44", d)
	}
	dec := sunAt(time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC))
	if d := declDeg(dec); math.Abs(d+23.44) > 0.3 {
		t.Errorf("declination at the December solstice = %.2f deg, want ~-23.44", d)
	}

	morning := sunAt(time.Date(2026, 3, 20, 6, 0, 0, 0, time.UTC))
	if lon := morning.lon / degToRad; math.Abs(lon-90) > 3 {
		t.Errorf("subsolar longitude at 06:00 UTC = %.2f deg, want ~+90", lon)
	}
	evening := sunAt(time.Date(2026, 3, 20, 18, 0, 0, 0, time.UTC))
	if lon := evening.lon / degToRad; math.Abs(lon+90) > 3 {
		t.Errorf("subsolar longitude at 18:00 UTC = %.2f deg, want ~-90", lon)
	}
}

func TestDaylightAlphaRampEndpointsAndMonotonicity(t *testing.T) {
	if a := daylightAlpha(math.Sin(0)); a != 255 {
		t.Errorf("alpha at altitude 0 = %d, want 255", a)
	}
	if a := daylightAlpha(0.7); a != 255 {
		t.Errorf("alpha in full day = %d, want 255", a)
	}
	if a := daylightAlpha(math.Sin(-9.01 * degToRad)); a != 0 {
		t.Errorf("alpha below -9 deg = %d, want 0", a)
	}
	if a := daylightAlpha(math.Sin(-4.5 * degToRad)); a < 125 || a > 130 {
		t.Errorf("alpha mid-twilight = %d, want ~127 (linear in altitude)", a)
	}
	prev := uint8(0)
	for alt := -10.0; alt <= 1.0; alt += 0.25 {
		a := daylightAlpha(math.Sin(alt * degToRad))
		if a < prev {
			t.Fatalf("alpha not monotonic: %d after %d at altitude %.2f", a, prev, alt)
		}
		prev = a
	}
}

// The analytic strip solver must cover every row the brute-force oracle
// finds in the widened twilight band (under either sun), stay within a
// few rows of tight, and report no strip when the oracle finds none.
func TestStripRowsMatchBruteForceOracle(t *testing.T) {
	r := testRenderer(t, image.Pt(800, 400))
	mkSun := func(decDeg, lonDeg float64) sunPos {
		d := decDeg * degToRad
		return sunPos{sinDec: math.Sin(d), cosDec: math.Cos(d), lon: lonDeg * degToRad}
	}
	// The 3-degree declination exercises the |k| > R arcsin-domain branch
	// (columns near 90 degrees from the sun never reach the lower
	// threshold, so the strip swallows the whole dim side).
	for _, suns := range [][2]sunPos{
		{mkSun(23.4, 40), mkSun(23.4, 39.7)},
		{mkSun(-23.4, -170), mkSun(-23.4, -170.3)},
		{mkSun(0.2, 5), mkSun(0.2, 4.7)},
		{mkSun(3, 100), mkSun(2.99, 99.7)},
	} {
		oldSun, newSun := suns[0], suns[1]
		for x := r.mapRect.Min.X; x < r.mapRect.Max.X; x += 37 {
			lo, hi, ok := r.stripRows(x, oldSun, newSun)

			bLo, bHi := -1, -1
			for y := r.mapRect.Min.Y; y < r.mapRect.Max.Y; y++ {
				for _, s := range [2]sunPos{oldSun, newSun} {
					v := s.sinAlt(r.rowSin[y], r.rowCos[y], r.colLon[x])
					if v > sinStripLo && v < sinStripHi {
						if bLo == -1 {
							bLo = y
						}
						bHi = y
						break
					}
				}
			}

			if bLo == -1 {
				if ok {
					t.Errorf("col %d: solver found strip %d..%d, oracle found none", x, lo, hi)
				}
				continue
			}
			if !ok {
				t.Errorf("col %d: solver found no strip, oracle found %d..%d", x, bLo, bHi)
				continue
			}
			if lo > bLo || hi < bHi {
				t.Errorf("col %d: solver strip %d..%d misses oracle rows %d..%d", x, lo, hi, bLo, bHi)
			}
			if lo < bLo-3 || hi > bHi+3 {
				t.Errorf("col %d: solver strip %d..%d is slack vs oracle %d..%d", x, lo, hi, bLo, bHi)
			}
		}
	}
}

// The gold partial-update guarantee: after a 60s terminator refresh, the
// lit map must equal a from-scratch full compose at the new time, and
// every changed pixel must lie inside a dirty rect the tick reported.
func TestTerminatorStripRefreshMatchesFullComposeAndStaysInsideRects(t *testing.T) {
	now := time.Unix(refEpoch, 0)
	r := testRenderer(t, image.Pt(800, 400))
	r.render(syntheticFrame(now, 0.02))

	before := make([]byte, len(r.litMap.Pix))
	copy(before, r.litMap.Pix)

	later := now.Add(61 * time.Second)
	dirty := r.render(syntheticFrame(later, 0.02))

	fresh := testRenderer(t, image.Pt(800, 400))
	fresh.composeLit(sunAt(later))
	if !bytes.Equal(r.litMap.Pix, fresh.litMap.Pix) {
		t.Error("lit map after a strip refresh differs from a full compose at the same time")
	}

	area := 0
	for _, rect := range dirty {
		area += rect.Dx() * rect.Dy()
	}
	full := r.back.Bounds().Dx() * r.back.Bounds().Dy()
	if area > full*35/100 {
		t.Errorf("refresh tick dirtied %d of %d px (%.0f%%); strips must stay bounded", area, full, 100*float64(area)/float64(full))
	}

	covered := func(p image.Point) bool {
		for _, rect := range dirty {
			if p.In(rect) {
				return true
			}
		}
		return false
	}
	for y := 0; y < r.size.Y; y++ {
		for x := 0; x < r.size.X; x++ {
			i := r.litMap.PixOffset(x, y)
			if !bytes.Equal(r.litMap.Pix[i:i+4], before[i:i+4]) && !covered(image.Pt(x, y)) {
				t.Fatalf("lit-map pixel (%d,%d) changed outside every reported dirty rect", x, y)
			}
		}
	}
}

// A track crossing the terminator must be re-stamped on top of the
// relit strip: after a refresh tick, the solid past line stays exactly
// track-red along its whole (unfaded) length.
func TestTrackIsRestampedInsideRefreshedStrips(t *testing.T) {
	r := testRenderer(t, image.Pt(800, 400))
	now := time.Unix(refEpoch, 0)

	vertical := func(tick time.Time) frame {
		const spanSec = 1400 // all younger than the fade window
		at := func(ts time.Time) trackPoint {
			age := tick.Sub(ts).Seconds()
			return trackPoint{t: ts, lat: -55 + (age/10)*1, lon: 20}
		}
		var past []trackPoint
		for age := spanSec; age >= 0; age -= sampleStepSec {
			past = append(past, at(tick.Add(-time.Duration(age)*time.Second)))
		}
		sat := past[len(past)-1]
		return frame{
			sat:  sat,
			past: past,
			future: []trackPoint{sat,
				{t: tick.Add(sampleStepSec * time.Second), lat: sat.lat - 1, lon: 20}},
		}
	}

	r.render(vertical(now))
	r.render(vertical(now.Add(61 * time.Second))) // includes the strip refresh

	x := r.project(0, 20).X
	for lat := 70.0; lat >= -40; lat-- {
		y := r.project(lat, 20).Y
		if got := r.back.RGBAAt(x, y); got != trackRed {
			t.Fatalf("past line at lat %.0f (%d,%d) = %v, want %v - strip relight must re-stamp the track", lat, x, y, got, trackRed)
		}
	}
}
