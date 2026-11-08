package mdedit

import (
	"strings"

	"muse/mdoc"
)

// mathScale turns the pixels of a typeset formula into DIPs: mdoc
// rasterizes at twice the density of a DIP.
const mathScale = mdoc.MathScale

// mathObject returns the typeset formula of tex at the size of the text
// around it, as an inline object, or nil when it cannot be typeset.
func (e *Editor) mathObject(tex string, size float32, display bool) *object {
	tex = strings.TrimSpace(tex)
	if tex == "" {
		return nil
	}
	if display {
		size *= 1.15
	}
	bmp, baseline := mdoc.RenderMathBaseline(tex, float64(size), e.lk.dark)
	if bmp == nil {
		return nil
	}
	w, h := bmp.Size()
	fw, fh := float32(w)*mathScale, float32(h)*mathScale
	// The formula's baseline sits on the text's.
	ascent := min(baseline*mathScale, fh)
	return &object{bmp: bmp, w: fw + 2, h: ascent, descent: fh - ascent}
}

// mathBlockObject returns the typeset formula of a block of math.
func (e *Editor) mathBlockObject(tex string, width float32) *object {
	tex = strings.TrimSpace(tex)
	if tex == "" {
		return nil
	}
	bmp := mdoc.RenderMath(tex, float64(e.lk.size*1.2), e.lk.dark)
	if bmp == nil {
		return nil
	}
	w, h := bmp.Size()
	fw, fh := float32(w)*mathScale, float32(h)*mathScale
	if width > 0 && fw > width {
		fh = fh * width / fw
		fw = width
	}
	return &object{bmp: bmp, w: fw, h: fh, block: true}
}

// imageObject returns the image at dest as an inline object, scaled down
// to the width; a placeholder showing alt until it loads. alone makes it a
// line of its own: a paragraph of just an image.
func (e *Editor) imageObject(dest, alt string, width float32, alone bool) *object {
	bmp := mdoc.LoadImage(dest, e.opts.BaseDir, e.assetLoaded)
	obj := &object{bmp: bmp, alt: alt, block: alone}
	if bmp == nil {
		obj.w = min(max(width, 80), 260)
		obj.h = e.lk.size * 2.4
		obj.descent = obj.h * 0.25
		return obj
	}
	w, h := bmp.Size()
	fw, fh := float32(w), float32(h)
	if width > 0 && fw > width {
		fh = fh * width / fw
		fw = width
	}
	obj.w, obj.h = fw, fh
	return obj
}

// htmlImageObject returns the image an HTML block of just an image shows.
func (e *Editor) htmlImageObject(html string, width float32) *object {
	src, alt, _, iw, ih, ok := mdoc.HTMLOnlyImage(html)
	if !ok {
		return nil
	}
	sz := imgSize{w: iw, h: ih}
	if i := strings.Index(strings.ToLower(html), "<img"); i >= 0 {
		if j := strings.IndexByte(html[i:], '>'); j >= 0 {
			if _, _, s, ok := imgTag(html[i : i+j+1]); ok {
				sz = s
			}
		}
	}
	return e.sizedImage(src, alt, sz, width, true)
}

// sizedImage returns the image at dest at the size an <img> asks for, by
// its width and height or a zoom, scaled down to the width.
func (e *Editor) sizedImage(dest, alt string, sz imgSize, width float32, alone bool) *object {
	obj := e.imageObject(dest, alt, width, alone)
	if obj.bmp == nil || sz == (imgSize{}) {
		return obj
	}
	bw, bh := obj.bmp.Size()
	fw, fh := float32(bw), float32(bh)
	if fw <= 0 || fh <= 0 {
		return obj
	}
	switch {
	case sz.w > 0 && sz.h > 0:
		fw, fh = sz.w, sz.h
	case sz.w > 0:
		fh = fh * sz.w / fw
		fw = sz.w
	case sz.h > 0:
		fw = fw * sz.h / fh
		fh = sz.h
	}
	if sz.zoom > 0 {
		fw, fh = fw*sz.zoom, fh*sz.zoom
	}
	if width > 0 && fw > width {
		fh = fh * width / fw
		fw = width
	}
	obj.w, obj.h = fw, fh
	return obj
}

// assetLoaded is called on another goroutine when an image has loaded:
// the leaves showing it lay out again with its size.
func (e *Editor) assetLoaded() {
	e.assetGen.Add(1)
	if e.opts.Invalidate != nil {
		e.opts.Invalidate()
	}
}

// sketchOptions is what a diagram needs to be drawn in this page: its
// colors, and a measure of its labels in this page's font.
func (e *Editor) sketchOptions() mdoc.SketchOptions {
	return mdoc.SketchOptions{
		Theme: &mdoc.SketchTheme{
			Bg:         e.lk.panel,
			Fg:         e.lk.text,
			Muted:      e.lk.muted,
			Line:       e.lk.border,
			Surface:    e.lk.panel,
			SurfaceAlt: e.lk.codeBg,
			Border:     e.lk.border,
			Accent:     e.lk.accent,
			Radius:     4,
		},
	}
}
