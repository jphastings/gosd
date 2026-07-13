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

// Dash geometry for the future line: a stamp at cumulative along-track arc
// length s is part of a dash iff (s mod 28) < dashOnPx. Arc length is
// accumulated over the drawn polyline from an anchor fixed at each full
// redraw, so dashes are a uniform 16px-on/12px-off along the track however
// steep it runs on screen - and because samples sit on absolute 10s epochs,
// an existing sample's arc length never changes between full redraws, so
// painted dashes never crawl and never need repainting.
const (
	dashOnPx     = 16
	dashPeriodPx = 28
)

// fadeWindow is the portion of the past half hour, measured back from its
// oldest tip, whose alpha ramps 255->0 toward the tip. Everything younger
// renders at full opacity, which keeps the long middle of the past line
// byte-identical between ticks (only the head, this window, and the dropped
// tail repaint).
//
// fadeWindowSec is deliberately a standalone knob: JP asked for "the oldest
// ~5 minutes" of the 30-minute window; flip it to 600 for a 10-minute ramp.
const (
	trackWindowSec = 1800
	fadeWindowSec  = 300
)

// circleStroke is the width of the black ring drawn around the outside of
// the red satellite circle.
const circleStroke = 2

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
// (same endpoints, same alpha, same style, same dash phase) costs nothing
// to keep on screen.
type item struct {
	kind    itemKind
	a, b    image.Point // segment endpoints; circle center in a; label rect in a/b
	alpha   uint8
	dashed  bool
	phase   float64 // dashed segments: cumulative arc length at endpoint a
	radius  int     // circle radius; segment half-thickness
	nameGen int     // label only: bumps when the satellite name re-renders
}

// bbox returns the item's screen-space bounding box, inflated by its stamp
// radius so erase/repaint covers every touched pixel.
func (it item) bbox() image.Rectangle {
	switch it.kind {
	case kindCircle:
		r := it.radius + circleStroke + 1
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
	arc       map[int64]float64 // future grid sample epoch -> cumulative arc length
}

func newRenderer(size image.Point, mapSrc image.Image) (*renderer, error) {
	r := &renderer{
		size:      size,
		back:      image.NewRGBA(image.Rectangle{Max: size}),
		mapImg:    image.NewRGBA(image.Rectangle{Max: size}),
		thickness: max(3, size.Y/240),
		radius:    max(6, size.Y/90),
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

// dashAt reports whether a stamp at cumulative along-track arc length s
// falls on a dash (rather than a gap) of the future line.
func dashAt(s float64) bool {
	m := math.Mod(s, dashPeriodPx)
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

// labelOutlineAlpha is the opacity of the label's 1px white outline.
const labelOutlineAlpha = 128

// setName re-renders the label image: the satellite name at ~height/34 px,
// black glyphs with a 1px white outline at 50% opacity so the name reads
// on ocean, land, ice, and the red track alike. The outline is built as a
// single mask - the glyph coverage dilated by 1px minus the glyphs - and
// composited exactly once; stamping offset 50%-white copies instead would
// stack past 50% wherever they overlap.
func (r *renderer) setName(name string) {
	if name == r.name && r.label != nil {
		return
	}
	r.name = name
	r.nameGen++

	metrics := r.face.Metrics()
	d := font.Drawer{Face: r.face}
	w := d.MeasureString(name).Ceil() + 4
	h := (metrics.Ascent + metrics.Descent).Ceil() + 4

	glyphs := image.NewAlpha(image.Rect(0, 0, w, h))
	d.Dst = glyphs
	d.Src = image.NewUniform(color.Alpha{A: 0xff})
	d.Dot = fixed.P(2, 2+metrics.Ascent.Ceil())
	d.DrawString(name)

	img := image.NewRGBA(glyphs.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g := uint32(glyphs.AlphaAt(x, y).A)
			o := uint32(dilatedAlpha(glyphs, x, y)-uint8(g)) * labelOutlineAlpha / 255
			// White outline under black glyphs (premultiplied alpha).
			c := uint8(o * (255 - g) / 255)
			a := uint8(g + o*(255-g)/255)
			img.SetRGBA(x, y, color.RGBA{R: c, G: c, B: c, A: a})
		}
	}
	r.label = img
}

// dilatedAlpha is the 8-neighbor maximum of the glyph coverage at (x, y):
// a 1px dilation of the glyph mask.
func dilatedAlpha(m *image.Alpha, x, y int) uint8 {
	var best uint8
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if a := m.AlphaAt(x+dx, y+dy).A; a > best {
				best = a
			}
		}
	}
	return best
}

// labelRect places the label horizontally to the right of the circle:
// left-aligned just past the stroked ring, vertically centered on the
// circle center. Fixed placement means the label rect depends only on the
// circle position, never on the direction of travel.
func labelRect(center image.Point, size image.Point, radius, gap int) image.Rectangle {
	x0 := center.X + radius + circleStroke + gap
	y0 := center.Y - size.Y/2
	return image.Rect(x0, y0, x0+size.X, y0+size.Y)
}

// buildItems converts a computed frame into the display list. Segments
// whose endpoints straddle the antimeridian (>180 degrees of longitude
// apart) are dropped: the track wraps from one screen edge to the other
// instead of smearing a line across the whole map.
func (r *renderer) buildItems(f frame, full bool) map[item]struct{} {
	items := make(map[item]struct{}, len(f.past)+len(f.future)+2)

	half := r.thickness / 2
	now := f.sat.t
	for i := 1; i < len(f.past); i++ {
		p, q := f.past[i-1], f.past[i]
		if wraps(p, q) {
			continue
		}
		a := fadeAlpha(now.Sub(p.t).Seconds())
		if a == 0 {
			continue
		}
		items[item{
			kind: kindSegment, a: r.project(p.lat, p.lon), b: r.project(q.lat, q.lon),
			alpha: a, radius: half,
		}] = struct{}{}
	}

	ss := r.futurePhases(f, full)
	for i := 1; i < len(f.future); i++ {
		p, q := f.future[i-1], f.future[i]
		if wraps(p, q) {
			continue
		}
		items[item{
			kind: kindSegment, a: r.project(p.lat, p.lon), b: r.project(q.lat, q.lon),
			alpha: 0xff, dashed: true, phase: ss[i-1], radius: half,
		}] = struct{}{}
	}

	center := r.project(f.sat.lat, f.sat.lon)
	items[item{kind: kindCircle, a: center, alpha: 0xff, radius: r.radius}] = struct{}{}

	if r.label != nil {
		rect := labelRect(center, r.label.Bounds().Size(), r.radius, r.labelGap)
		items[item{kind: kindLabel, a: rect.Min, b: rect.Max, alpha: 0xff, nameGen: r.nameGen}] = struct{}{}
	}
	return items
}

// wraps reports whether the segment p->q straddles the antimeridian.
func wraps(p, q trackPoint) bool {
	return math.Abs(normalizeLon(q.lon)-normalizeLon(p.lon)) > 180
}

// futurePhases returns each future point's cumulative along-track arc
// length in the dash parameterization. Grid samples (fixed absolute 10s
// epochs) get their arc length from a persistent table: anchored once per
// full redraw, extended as the sliding window appends samples, and never
// recomputed for samples already painted - that immutability is what keeps
// on-screen dashes from crawling or needing repaints. The two exact tips
// (now, now+window), which move every tick and repaint anyway, extrapolate
// from their neighboring grid sample.
func (r *renderer) futurePhases(f frame, full bool) []float64 {
	if full || r.arc == nil {
		r.arc = make(map[int64]float64)
	}
	pts := f.future
	ss := make([]float64, len(pts))
	if len(pts) < 3 {
		return ss
	}

	prev := -1 // index of the previous grid sample, its s already in ss
	for i := 1; i <= len(pts)-2; i++ {
		key := pts[i].t.Unix()
		if s, ok := r.arc[key]; ok {
			ss[i] = s
		} else if prev == -1 {
			// Anchor: the oldest grid sample at full-redraw time (or
			// after a discontinuous jump, which only re-phases dashes
			// that were about to be fully repainted anyway).
			r.arc[key] = 0
		} else {
			s := ss[prev] + r.segmentArc(pts[prev], pts[i])
			r.arc[key] = s
			ss[i] = s
		}
		prev = i
	}
	for k := range r.arc {
		if k < pts[0].t.Unix()-2*sampleStepSec {
			delete(r.arc, k)
		}
	}

	ss[0] = ss[1] - r.segmentArc(pts[0], pts[1])
	last := len(pts) - 1
	ss[last] = ss[last-1] + r.segmentArc(pts[last-1], pts[last])
	return ss
}

// segmentArc is the on-screen length a segment contributes to the dash
// parameterization: its euclidean pixel length, or 0 across an
// antimeridian split - arc length accumulates over drawn segments only,
// so the wrap jump injects no phantom phase.
func (r *renderer) segmentArc(p, q trackPoint) float64 {
	if wraps(p, q) {
		return 0
	}
	a := r.project(p.lat, p.lon)
	b := r.project(q.lat, q.lon)
	return math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y))
}

// render composes frame f and returns the changed rectangles (clipped to
// the screen, overlapping ones merged). The first call - and any call
// after invalidate - paints and returns the full frame; steady-state calls
// erase-and-repaint only the display-list difference.
func (r *renderer) render(f frame) []image.Rectangle {
	full := r.prevItems == nil
	items := r.buildItems(f, full)

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
	// Fixed z-order per rect - track lines, then the stroked circle, then
	// the label - so the circle's black ring paints over any line passing
	// through it regardless of map iteration order.
	for _, rect := range dirty {
		for _, kind := range [3]itemKind{kindSegment, kindCircle, kindLabel} {
			for it := range items {
				if it.kind == kind && it.bbox().Overlaps(rect) {
					r.draw(it, rect)
				}
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
		r.stampDisc(it.a, it.radius+circleStroke, color.RGBA{A: 0xff}, it.alpha, clip)
		r.stampDisc(it.a, it.radius, trackRed, it.alpha, clip)
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
// half-thickness; the dashed style skips stamps whose cumulative arc
// length (the segment's start phase plus the along-segment offset) falls
// in a gap. Filled discs along the walk give clean joints and honest
// thickness at these sizes without a polygon rasterizer.
func (r *renderer) stampSegment(it item, clip image.Rectangle) {
	dx, dy := float64(it.b.X-it.a.X), float64(it.b.Y-it.a.Y)
	length := math.Hypot(dx, dy)
	steps := int(math.Ceil(length))
	if steps == 0 {
		steps = 1
	}
	for s := 0; s <= steps; s++ {
		t := float64(s) / float64(steps)
		if it.dashed && !dashAt(it.phase+t*length) {
			continue
		}
		x := int(math.Round(float64(it.a.X) + dx*t))
		y := int(math.Round(float64(it.a.Y) + dy*t))
		r.stampDisc(image.Pt(x, y), it.radius, trackRed, it.alpha, clip)
	}
}

// stampDisc writes a filled disc of the given color at the given opacity,
// clipped. Plain source-over per pixel; alpha scales the color.
func (r *renderer) stampDisc(c image.Point, radius int, col color.RGBA, alpha uint8, clip image.Rectangle) {
	box := image.Rect(c.X-radius, c.Y-radius, c.X+radius+1, c.Y+radius+1).Intersect(clip)
	rr := radius * radius
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			ddx, ddy := x-c.X, y-c.Y
			if ddx*ddx+ddy*ddy > rr {
				continue
			}
			blend(r.back, x, y, col, alpha)
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
