package main

import (
	"fmt"
	"time"

	satellite "github.com/joshuaferrara/go-satellite"
)

// sampleStepSec is the ground-track sampling interval. Samples are aligned
// to absolute 10s boundaries so consecutive ticks share their sample times:
// per tick at most one sample enters or leaves each window edge, and every
// interior segment of the track is byte-identical to the previous frame's.
const sampleStepSec = 10

// trackPoint is a geodetic subpoint at an instant.
type trackPoint struct {
	t        time.Time
	lat, lon float64 // degrees
}

// frame is everything the renderer needs for one tick.
type frame struct {
	sat    trackPoint   // subpoint right now
	past   []trackPoint // oldest -> now, exact tips + 10s grid between
	future []trackPoint // now -> +trackWindow, exact tips + 10s grid between
}

// computeFrame propagates the satellite across the +-trackWindow (45min)
// around now.
// go-satellite panics on garbage TLE element combinations rather than
// returning errors, so the propagation of a whole frame is fenced with one
// recover and surfaced as an error (the caller refetches the TLE).
func computeFrame(sat satellite.Satellite, now time.Time) (f frame, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("propagating the TLE failed: %v", r)
		}
	}()

	f.sat = subpoint(sat, now)
	f.past = windowPoints(sat, now.Add(-trackWindowSec*time.Second), now)
	f.future = windowPoints(sat, now, now.Add(trackWindowSec*time.Second))
	return f, nil
}

// windowPoints samples [from, to]: the exact endpoints plus every absolute
// 10s grid time strictly between them.
func windowPoints(sat satellite.Satellite, from, to time.Time) []trackPoint {
	pts := []trackPoint{subpoint(sat, from)}
	grid := from.Truncate(sampleStepSec * time.Second)
	if !grid.After(from) {
		grid = grid.Add(sampleStepSec * time.Second)
	}
	for ; grid.Before(to); grid = grid.Add(sampleStepSec * time.Second) {
		pts = append(pts, subpoint(sat, grid))
	}
	return append(pts, subpoint(sat, to))
}

// subpoint propagates to t (UTC) and converts the ECI position to a
// geodetic latitude/longitude in degrees.
func subpoint(sat satellite.Satellite, t time.Time) trackPoint {
	t = t.UTC()
	pos, _ := satellite.Propagate(sat, t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	gmst := satellite.GSTimeFromDate(t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	_, _, ll := satellite.ECIToLLA(pos, gmst)
	deg := satellite.LatLongDeg(ll)
	return trackPoint{t: t, lat: deg.Latitude, lon: normalizeLon(deg.Longitude)}
}
