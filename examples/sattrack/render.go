package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Dash geometry for the future line: a pixel/stamp at screen x is part of a
// dash iff (x mod 28) < dashOnPx. Keying the pattern on absolute screen x
// (not arc distance from the satellite) means dashes never crawl as the
// track advances, so already-painted dashes never need repainting.
const (
	dashOnPx     = 16
	dashPeriodPx = 28
)

// fadeWindow is the portion of the past hour, measured back from its oldest
// tip, whose alpha ramps 255->0 toward the tip. Everything younger renders
// at full opacity, which keeps the long middle of the past line
// byte-identical between ticks (only the head, this window, and the dropped
// tail repaint).
const (
	trackWindowSec = 3600
	fadeWindowSec  = 600
)

var trackRed = color.RGBA{R: 0xd8, G: 0x20, B: 0x20, A: 0xff}

// itemKind discriminates renderer display-list entries.
type itemKind uint8

const (
	kindSegment itemKind = iota
	kindCircle
	kindLabel
)

// item is one drawable in the renderer's display list. Items are plain
// comparable values: the per-tick partial repaint is a set difference
// between the previous and current display lists, so anything unchanged
// (same endpoints, same alpha, same style) costs nothing to keep on screen.
type item struct {
	kind    itemKind
	a, b    image.Point // segment endpoints; circle center in a; label rect in a/b
	alpha   uint8
	dashed  bool
	radius  int // circle radius; segment half-thickness
	nameGen int // label only: bumps when the satellite name re-renders
}

// bbox returns the item's screen-space bounding box, inflated by its stamp
// radius so erase/repaint covers every touched pixel.
func (it item) bbox() image.Rectangle {
	switch it.kind {
	case kindCircle:
		r := it.radius + 1
		return image.Rect(it.a.X-r, it.a.Y-r, it.a.X+r+1, it.a.Y+r+1)
	case kindLabel:
		return image.Rectangle{Min: it.a, Max: it.b}
	default:
		r := it.radius + 1
		box := image.Rectangle{Min: it.a, Max: it.a.Add(image.Pt(1, 1))}
		box = box.Union(image.Rectangle{Min: it.b, Max: it.b.Add(image.Pt(1, 1))})
		return box.Inset(-r)
	}
}

// renderer composes the scene into a full-screen RGBA backbuffer and
// reports, per frame, exactly which rectangles changed. It is pure image
// manipulation - no DRM, no clocks - so it runs (and is tested) anywhere.
type renderer struct {
	size      image.Point
	mapImg    *image.RGBA // pristine letterboxed map: the erase source
	back      *image.RGBA
	mapRect   image.Rectangle // where the 2:1 map sits within size
	thickness int
	radius    int
	labelGap  int

	face    font.Face
	label   *image.RGBA // rendered name, outline baked in
	nameGen int
	name    string

	prevItems map[item]struct{}
	prevN     [2]float64 // last usable screen-space travel normal
}

func newRenderer(size image.Point, mapSrc image.Image) (*renderer, error) {
	r := &renderer{
		size:      size,
		back:      image.NewRGBA(image.Rectangle{Max: size}),
		mapImg:    image.NewRGBA(image.Rectangle{Max: size}),
		thickness: max(3, size.Y/240),
		radius:    max(6, size.Y/90),
		prevN:     [2]float64{0, -1},
	}
	r.labelGap = max(4, r.radius/2)
	r.mapRect = letterbox2to1(size)

	// Black bars around the letterboxed map; CatmullRom for the one-off
	// high-quality scale.
	draw.Draw(r.mapImg, r.mapImg.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(r.mapImg, r.mapRect, mapSrc, mapSrc.Bounds(), xdraw.Src, nil)

	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	r.face, err = opentype.NewFace(f, &opentype.FaceOptions{
		Size: math.Max(10, float64(size.Y)/34), DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// letterbox2to1 returns the largest centered 2:1 rectangle within size.
func letterbox2to1(size image.Point) image.Rectangle {
	w, h := size.X, size.Y
	if w >= 2*h {
		w = 2 * h
	} else {
		h = w / 2
	}
	x0 := (size.X - w) / 2
	y0 := (size.Y - h) / 2
	return image.Rect(x0, y0, x0+w, y0+h)
}

// project maps a geodetic subpoint (degrees) onto the letterboxed
// equirectangular map: lon -180..180 left->right, lat 90..-90 top->bottom.
func (r *renderer) project(lat, lon float64) image.Point {
	lon = normalizeLon(lon)
	x := float64(r.mapRect.Min.X) + (lon+180)/360*float64(r.mapRect.Dx())
	y := float64(r.mapRect.Min.Y) + (90-lat)/180*float64(r.mapRect.Dy())
	return image.Pt(int(math.Round(x)), int(math.Round(y)))
}

// normalizeLon wraps a longitude into [-180, 180).
func normalizeLon(lon float64) float64 {
	lon = math.Mod(lon+180, 360)
	if lon < 0 {
		lon += 360
	}
	return lon - 180
}

// dashOn reports whether a stamp centered on screen column x falls on a
// dash (rather than a gap) of the future line. Position-stable by
// construction: the answer depends on x alone.
func dashOn(x int) bool {
	m := x % dashPeriodPx
	if m < 0 {
		m += dashPeriodPx
	}
	return m < dashOnPx
}

// fadeAlpha returns the past line's opacity for a sample ageSec seconds
// old: opaque until the fade window, then a linear ramp to 0 at the
// window's (and track's) oldest tip.
func fadeAlpha(ageSec float64) uint8 {
	if ageSec <= trackWindowSec-fadeWindowSec {
		return 0xff
	}
	if ageSec >= trackWindowSec {
		return 0
	}
	return uint8(255 * (trackWindowSec - ageSec) / fadeWindowSec)
}

// setName re-renders the label image: the satellite name at ~height/34 px,
// white with a 1px black outline (drawn 4x offset in black, then once in
// white) so it stays legible over both ocean and landmass.
func (r *renderer) setName(name string) {
	if name == r.name && r.label != nil {
		return
	}
	r.name = name
	r.nameGen++

	d := font.Drawer{Face: r.face}
	adv := d.MeasureString(name)
	metrics := r.face.Metrics()
	w := adv.Ceil() + 4
	h := (metrics.Ascent + metrics.Descent).Ceil() + 4
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	d.Dst = img

	baseline := fixed.P(2, 2+metrics.Ascent.Ceil())
	d.Src = image.NewUniform(color.Black)
	for _, off := range [4]image.Point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		d.Dot = baseline.Add(fixed.P(off.X, off.Y))
		d.DrawString(name)
	}
	d.Src = image.NewUniform(color.White)
	d.Dot = baseline
	d.DrawString(name)
	r.label = img
}

// labelPlacement computes the label rectangle for a satellite at center
// travelling with screen-space velocity v (y down). The label sits
// perpendicular to the direction of travel on the trailing (solid-line)
// side: offset direction n = (vy, -vx) normalized, anchor = center +
// n*(radius+gap). Horizontal alignment follows the offset - n.x > 0 means
// left-aligned (text extends rightward, away from the circle), n.x < 0
// right-aligned - and the label slides vertically with n.y so it clears
// the circle when the offset is mostly vertical.
func labelPlacement(center image.Point, v [2]float64, size image.Point, radius, gap int) image.Rectangle {
	n := normal(v)
	dist := float64(radius + gap)
	ax := float64(center.X) + n[0]*dist
	ay := float64(center.Y) + n[1]*dist

	w, h := size.X, size.Y
	var x0 float64
	switch {
	case n[0] > 1e-9:
		x0 = ax
	case n[0] < -1e-9:
		x0 = ax - float64(w)
	default:
		x0 = ax - float64(w)/2
	}
	y0 := ay - float64(h)/2 + n[1]*float64(h)/2

	return image.Rect(int(math.Round(x0)), int(math.Round(y0)),
		int(math.Round(x0))+w, int(math.Round(y0))+h)
}

// normal returns the unit normal (vy, -vx) of a screen-space velocity;
// a degenerate velocity yields "straight up" so the label lands somewhere
// sane rather than atop the circle.
func normal(v [2]float64) [2]float64 {
	n := [2]float64{v[1], -v[0]}
	l := math.Hypot(n[0], n[1])
	if l < 1e-9 {
		return [2]float64{0, -1}
	}
	return [2]float64{n[0] / l, n[1] / l}
}

// buildItems converts a computed frame into the display list. Segments
// whose endpoints straddle the antimeridian (>180 degrees of longitude
// apart) are dropped: the track wraps from one screen edge to the other
// instead of smearing a line across the whole map.
func (r *renderer) buildItems(f frame) map[item]struct{} {
	items := make(map[item]struct{}, len(f.past)+len(f.future)+2)

	half := r.thickness / 2
	addPolyline := func(pts []trackPoint, dashed bool, alphaFor func(trackPoint) uint8) {
		for i := 1; i < len(pts); i++ {
			p, q := pts[i-1], pts[i]
			if math.Abs(normalizeLon(q.lon)-normalizeLon(p.lon)) > 180 {
				continue
			}
			a := alphaFor(p)
			if a == 0 {
				continue
			}
			items[item{
				kind: kindSegment, a: r.project(p.lat, p.lon), b: r.project(q.lat, q.lon),
				alpha: a, dashed: dashed, radius: half,
			}] = struct{}{}
		}
	}

	now := f.sat.t
	addPolyline(f.past, false, func(p trackPoint) uint8 {
		return fadeAlpha(now.Sub(p.t).Seconds())
	})
	addPolyline(f.future, true, func(trackPoint) uint8 { return 0xff })

	center := r.project(f.sat.lat, f.sat.lon)
	items[item{kind: kindCircle, a: center, alpha: 0xff, radius: r.radius}] = struct{}{}

	if r.label != nil {
		v, ok := r.screenVelocity(f)
		if ok {
			r.prevN = normal(v)
		}
		rect := labelPlacementFromNormal(center, r.prevN, r.label.Bounds().Size(), r.radius, r.labelGap)
		items[item{kind: kindLabel, a: rect.Min, b: rect.Max, alpha: 0xff, nameGen: r.nameGen}] = struct{}{}
	}
	return items
}

// screenVelocity derives the on-screen direction of travel from the
// current subpoint and the frame's 10-second lookahead. It reports !ok
// when the pair straddles the antimeridian wrap - the caller keeps the
// previous direction for that tick.
func (r *renderer) screenVelocity(f frame) ([2]float64, bool) {
	if math.Abs(normalizeLon(f.lookahead.lon)-normalizeLon(f.sat.lon)) > 180 {
		return [2]float64{}, false
	}
	a := r.project(f.sat.lat, f.sat.lon)
	b := r.project(f.lookahead.lat, f.lookahead.lon)
	v := [2]float64{float64(b.X - a.X), float64(b.Y - a.Y)}
	if math.Hypot(v[0], v[1]) < 1e-9 {
		return [2]float64{}, false
	}
	return v, true
}

// labelPlacementFromNormal is labelPlacement with the normal already
// resolved (so a wrap tick can reuse the previous direction).
func labelPlacementFromNormal(center image.Point, n [2]float64, size image.Point, radius, gap int) image.Rectangle {
	// labelPlacement recomputes the normal from a velocity; feed it one
	// whose normal is n: v = (-ny, nx) since normal(v) = (vy, -vx).
	return labelPlacement(center, [2]float64{-n[1], n[0]}, size, radius, gap)
}

// render composes frame f and returns the changed rectangles (clipped to
// the screen, overlapping ones merged). The first call - and any call
// after invalidate - paints and returns the full frame; steady-state calls
// erase-and-repaint only the display-list difference.
func (r *renderer) render(f frame) []image.Rectangle {
	items := r.buildItems(f)

	full := r.prevItems == nil
	var dirty []image.Rectangle
	if full {
		dirty = []image.Rectangle{r.back.Bounds()}
	} else {
		for it := range r.prevItems {
			if _, still := items[it]; !still {
				dirty = append(dirty, it.bbox())
			}
		}
		for it := range items {
			if _, had := r.prevItems[it]; !had {
				dirty = append(dirty, it.bbox())
			}
		}
	}
	dirty = clipAndMerge(dirty, r.back.Bounds())

	for _, rect := range dirty {
		draw.Draw(r.back, rect, r.mapImg, rect.Min, draw.Src)
	}
	for _, rect := range dirty {
		for it := range items {
			if it.bbox().Overlaps(rect) {
				r.draw(it, rect)
			}
		}
	}

	r.prevItems = items
	return dirty
}

// invalidate forces the next render to repaint the whole frame (startup,
// TLE refresh, mode discovery).
func (r *renderer) invalidate() {
	r.prevItems = nil
}

// draw paints one item onto the backbuffer, writing only inside clip:
// repaints erase just the dirty rectangle, so an item must never blend
// pixels outside it (they'd double-blend over the previous frame's paint).
func (r *renderer) draw(it item, clip image.Rectangle) {
	switch it.kind {
	case kindCircle:
		r.stampDisc(it.a, it.radius, it.alpha, clip)
	case kindLabel:
		target := image.Rectangle{Min: it.a, Max: it.b}.Intersect(clip)
		if !target.Empty() {
			draw.Draw(r.back, target, r.label, target.Min.Sub(it.a), draw.Over)
		}
	case kindSegment:
		r.stampSegment(it, clip)
	}
}

// stampSegment walks a..b in ~1px steps stamping discs of the line's
// half-thickness; the dashed style simply skips stamps whose x fails
// dashOn. Filled discs along the walk give clean joints and honest
// thickness at these sizes without a polygon rasterizer.
func (r *renderer) stampSegment(it item, clip image.Rectangle) {
	dx, dy := float64(it.b.X-it.a.X), float64(it.b.Y-it.a.Y)
	steps := int(math.Ceil(math.Hypot(dx, dy)))
	if steps == 0 {
		steps = 1
	}
	for s := 0; s <= steps; s++ {
		t := float64(s) / float64(steps)
		x := int(math.Round(float64(it.a.X) + dx*t))
		y := int(math.Round(float64(it.a.Y) + dy*t))
		if it.dashed && !dashOn(x) {
			continue
		}
		r.stampDisc(image.Pt(x, y), it.radius, it.alpha, clip)
	}
}

// stampDisc writes a filled disc in the track color at the given opacity,
// clipped. Plain source-over per pixel; Alpha scales the color.
func (r *renderer) stampDisc(c image.Point, radius int, alpha uint8, clip image.Rectangle) {
	box := image.Rect(c.X-radius, c.Y-radius, c.X+radius+1, c.Y+radius+1).Intersect(clip)
	rr := radius * radius
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			ddx, ddy := x-c.X, y-c.Y
			if ddx*ddx+ddy*ddy > rr {
				continue
			}
			blend(r.back, x, y, trackRed, alpha)
		}
	}
}

// blend source-overs col at opacity alpha onto dst's pixel (x, y).
func blend(dst *image.RGBA, x, y int, col color.RGBA, alpha uint8) {
	i := dst.PixOffset(x, y)
	a := uint32(alpha)
	inv := 255 - a
	p := dst.Pix[i : i+4 : i+4]
	p[0] = uint8((uint32(col.R)*a + uint32(p[0])*inv) / 255)
	p[1] = uint8((uint32(col.G)*a + uint32(p[1])*inv) / 255)
	p[2] = uint8((uint32(col.B)*a + uint32(p[2])*inv) / 255)
	p[3] = 0xff
}

// clipAndMerge clips rects to bounds, drops empties, and unions any
// overlapping pairs until none remain, so the flush sends a minimal set
// of non-overlapping-ish damage clips.
func clipAndMerge(rects []image.Rectangle, bounds image.Rectangle) []image.Rectangle {
	var out []image.Rectangle
	for _, rect := range rects {
		rect = rect.Intersect(bounds)
		if rect.Empty() {
			continue
		}
		out = append(out, rect)
	}
	for {
		merged := false
		for i := 0; i < len(out) && !merged; i++ {
			for j := i + 1; j < len(out); j++ {
				if out[i].Overlaps(out[j]) {
					out[i] = out[i].Union(out[j])
					out = append(out[:j], out[j+1:]...)
					merged = true
					break
				}
			}
		}
		if !merged {
			return out
		}
	}
}
