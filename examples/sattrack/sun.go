package main

import (
	"math"
	"time"
)

// Daylight ramp thresholds: full day at solar altitude >= dayFullAltDeg,
// full night at <= nightFullAltDeg, linear in altitude between. -9 degrees
// sits between civil (-6) and nautical (-12) twilight; both constants are
// taste knobs for JP.
const (
	dayFullAltDeg   = 0.0
	nightFullAltDeg = -9.0
)

// stripMarginDeg widens the twilight band when computing refresh strips.
// The subsolar point drifts ~0.25 degrees of longitude per minute, so one
// degree comfortably covers a 60s refresh interval plus rounding.
const stripMarginDeg = 1.0

const degToRad = math.Pi / 180

// sunPos is the subsolar point, in the precomputed form the per-pixel
// lighting equation wants.
type sunPos struct {
	sinDec, cosDec float64
	lon            float64 // subsolar longitude, radians
}

// sunAt computes the subsolar point with NOAA's low-accuracy solar
// formulas (fractional year -> equation of time + declination), good to
// a few hundredths of a degree - plenty for a 2px-per-degree map.
func sunAt(t time.Time) sunPos {
	t = t.UTC()
	hours := float64(t.Hour()) + float64(t.Minute())/60 + float64(t.Second())/3600
	g := 2 * math.Pi / 365 * (float64(t.YearDay()) - 1 + (hours-12)/24)

	eqTimeMin := 229.18 * (0.000075 +
		0.001868*math.Cos(g) - 0.032077*math.Sin(g) -
		0.014615*math.Cos(2*g) - 0.040849*math.Sin(2*g))
	decl := 0.006918 -
		0.399912*math.Cos(g) + 0.070257*math.Sin(g) -
		0.006758*math.Cos(2*g) + 0.000907*math.Sin(2*g) -
		0.002697*math.Cos(3*g) + 0.00148*math.Sin(3*g)

	// The sun is overhead where local true solar time is 12:00; Greenwich
	// true solar time runs hours*60+eqTime minutes, and longitude shifts
	// it by 4 minutes per degree.
	lonDeg := -(hours*60 + eqTimeMin - 720) / 4
	return sunPos{sinDec: math.Sin(decl), cosDec: math.Cos(decl), lon: normalizeLon(lonDeg) * degToRad}
}

// sinAlt is the sine of the solar altitude at a point, given the point's
// latitude trig and longitude (radians): one multiply-add once the
// per-column cos(lon - sun.lon) is cached.
func (s sunPos) sinAlt(sinLat, cosLat, lonRad float64) float64 {
	return sinLat*s.sinDec + cosLat*s.cosDec*math.Cos(lonRad-s.lon)
}

var (
	sinNightFull    = math.Sin(nightFullAltDeg * degToRad)
	sinDayFull      = math.Sin(dayFullAltDeg * degToRad)
	sinStripLo      = math.Sin((nightFullAltDeg - stripMarginDeg) * degToRad)
	sinStripHi      = math.Sin((dayFullAltDeg + stripMarginDeg) * degToRad)
	nightFullAltRad = nightFullAltDeg * degToRad
	dayFullAltRad   = dayFullAltDeg * degToRad
)

// daylightAlpha maps the sine of the solar altitude to the day-texture
// opacity: 255 in full day, 0 in full night, linear in altitude through
// the twilight band (the asin only runs for the band's few percent of
// pixels).
func daylightAlpha(sinAlt float64) uint8 {
	if sinAlt >= sinDayFull {
		return 255
	}
	if sinAlt <= sinNightFull {
		return 0
	}
	alt := math.Asin(sinAlt)
	return uint8(math.Round(255 * (alt - nightFullAltRad) / (dayFullAltRad - nightFullAltRad)))
}

// latRoots collects the latitudes (radians, within [-pi/2, pi/2]) where
// this sun's sin(altitude) equals k along the meridian at lonRad.
// sin(alt)(lat) = a*sin(lat) + b*cos(lat) = R*sin(lat+phi), so each
// threshold has up to two arcsin branches; |k| > R (polar day or night
// deep enough that the threshold is never reached) yields none.
func (s sunPos) latRoots(lonRad, k float64, out []float64) []float64 {
	a := s.sinDec
	b := s.cosDec * math.Cos(lonRad-s.lon)
	r := math.Hypot(a, b)
	if r < 1e-12 || math.Abs(k) > r {
		return out
	}
	base := math.Asin(k / r)
	phi := math.Atan2(b, a)
	for _, root := range [2]float64{base - phi, math.Pi - base - phi} {
		// Wrap into [-pi, pi), then keep real latitudes.
		root = math.Mod(root+3*math.Pi, 2*math.Pi) - math.Pi
		if root >= -math.Pi/2 && root <= math.Pi/2 {
			out = append(out, root)
		}
	}
	return out
}
