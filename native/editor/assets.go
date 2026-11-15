package editor

import (
	"strings"
	"unsafe"

	"muse/markdown"
)

// mathScale turns the pixels of a typeset formula into DIPs: mdoc
// rasterizes at twice the density of a DIP.
const mathScale = markdown.MathScale

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
	visual := markdown.RenderMathVisual(tex, float64(size), e.lk.dark, false, e.lk.font)
	if visual == nil || visual.Bitmap() == nil {
		return nil
	}
	bmp, baseline := visual.Bitmap(), visual.Baseline()
	w, h := bmp.Size()
	fw, fh := float32(w)*mathScale, float32(h)*mathScale
	// The formula's baseline sits on the text's.
	ascent := min(baseline*mathScale, fh)
	return &object{bmp: bmp, math: visual, w: fw + 2, h: ascent, descent: fh - ascent}
}

// mathBlockObject returns the typeset formula of a block of math.
func (e *Editor) mathBlockObject(tex string, width float32) *object {
	tex = strings.TrimSpace(tex)
	if tex == "" {
		return nil
	}
	visual := markdown.RenderMathVisual(tex, float64(e.lk.size*1.2), e.lk.dark, true, e.lk.font)
	if visual == nil || visual.Bitmap() == nil {
		return nil
	}
	bmp := visual.Bitmap()
	w, h := bmp.Size()
	fw, fh := float32(w)*mathScale, float32(h)*mathScale
	if width > 0 && fw > width {
		fh = fh * width / fw
		fw = width
	}
	return &object{bmp: bmp, math: visual, w: fw, h: fh, block: true}
}

// imageObject returns the image at dest as an inline object, scaled down
// to the width; a placeholder showing alt until it loads. alone makes it a
// line of its own: a paragraph of just an image.
func (e *Editor) imageObject(dest, alt string, width float32, alone bool) *object {
	resolved := ""
	if e.opts.ResolveImage != nil {
		resolved = e.opts.ResolveImage(dest, e.opts.BaseDir, e.opts.AssetRoot)
	} else {
		resolved = markdown.ResolveImageSource(dest, e.opts.BaseDir, e.opts.AssetRoot)
	}
	var asset *markdown.ImageAsset
	if strings.HasPrefix(resolved, "data:") {
		asset = e.dataImageAsset(resolved)
	} else {
		asset = markdown.LoadImageAssetWithSizeAndCacheStore(resolved, "", e.opts.ImageCacheStore, e.assetLoaded, e.assetLoaded)
	}
	bmp := asset.Bitmap()
	intrinsic, hasIntrinsic := asset.IntrinsicSize()
	// ImageAsset owns the asynchronous read and decode. Do not decode a second
	// copy here: doing that on the UI thread delays the first frame of every
	// note whose visible content contains a local image.
	obj := &object{asset: asset, bmp: bmp, imagePending: !asset.Loaded(), alt: alt, block: alone, imagePath: resolved}
	if bmp == nil {
		// A missing image is rendered as a paragraph-like fallback block. Keep
		// it full width even when its source occurred inline; the surrounding
		// text then wraps before and after the fallback instead of being squeezed
		// into a narrow image-sized chip.
		obj.imageDest = dest
		if width > 0 {
			obj.w = width
		} else {
			obj.w = 80
		}
		obj.h = e.lk.size * 2.4
		obj.descent = obj.h * 0.25
		if hasIntrinsic {
			obj.w, obj.h = imageSizeAtWidth(float32(intrinsic.X), float32(intrinsic.Y), width)
			// Decoded bitmap objects use a zero baseline descent. Keep the hinted
			// placeholder on the same metrics so completion does not change line
			// height after the aspect ratio has been reserved.
			obj.descent = 0
		}
		return obj
	}
	if hasIntrinsic {
		// Keep using source geometry after decode too. Large rasters may be
		// reduced during decode, and integer scaling can slightly alter their
		// ratio; layout should stay on the exact dimensions already reserved.
		obj.w, obj.h = imageSizeAtWidth(float32(intrinsic.X), float32(intrinsic.Y), width)
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

type dataImageAssetKey struct {
	data *byte
	len  int
}

// dataImageAsset keeps the asset attached to the editor while a large data
// URL is visible. Such sources are deliberately omitted from the process-wide
// byte-bounded cache after decode; without this document-local identity map,
// the assetGen invalidation after loading would parse the view again and start
// another full Base64 decode immediately.
func (e *Editor) dataImageAsset(src string) *markdown.ImageAsset {
	key := dataImageAssetKey{data: unsafe.StringData(src), len: len(src)}
	if asset := e.dataImageAssets[key]; asset != nil {
		return asset
	}
	asset := markdown.LoadImageAssetWithSize(src, "", e.assetLoaded, e.assetLoaded)
	if e.dataImageAssets == nil {
		e.dataImageAssets = make(map[dataImageAssetKey]*markdown.ImageAsset)
	}
	e.dataImageAssets[key] = asset
	return asset
}

// htmlImageObject returns the image an HTML block of just an image shows.
func (e *Editor) htmlImageObject(html string, width float32) *object {
	src, alt, _, iw, ih, ok := markdown.HTMLOnlyImage(html)
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
	obj.refreshAssetBitmap()
	if sz == (imgSize{}) {
		return obj
	}
	baseW, baseH := obj.w, obj.h
	if intrinsic, ok := obj.asset.IntrinsicSize(); ok {
		baseW, baseH = float32(intrinsic.X), float32(intrinsic.Y)
	} else if obj.bmp != nil {
		bw, bh := obj.bmp.Size()
		baseW, baseH = float32(bw), float32(bh)
	}
	obj.w, obj.h = sizedImageDimensions(baseW, baseH, sz, width)
	if obj.bmp == nil {
		// Explicit HTML geometry is already authoritative even when the source
		// header is not decodable yet.
		obj.descent = 0
	}
	return obj
}

func imageSizeAtWidth(w, h, width float32) (float32, float32) {
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	if width > 0 && w > width {
		h = h * width / w
		w = width
	}
	return w, h
}

func sizedImageDimensions(baseW, baseH float32, sz imgSize, width float32) (float32, float32) {
	if baseW <= 0 || baseH <= 0 {
		return 0, 0
	}
	fw, fh := baseW, baseH
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
	return fw, fh
}

func (o *object) refreshAssetBitmap() {
	if o != nil && o.asset != nil {
		if bmp := o.asset.Bitmap(); bmp != nil {
			o.bmp = bmp
		}
	}
}

// assetLoaded is called on a worker when image dimensions are known or image
// loading completes; the leaves showing it lay out again with stable geometry.
func (e *Editor) assetLoaded() {
	e.assetGen.Add(1)
	if e.opts.Invalidate != nil {
		e.opts.Invalidate()
	}
}

// sketchOptions is what a diagram needs to be drawn in this page: its
// colors, and a measure of its labels in this page's font.
func (e *Editor) sketchOptions() markdown.SketchOptions {
	return markdown.SketchOptions{
		Theme: &markdown.SketchTheme{
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
