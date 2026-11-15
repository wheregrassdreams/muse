package mdoc

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

var svgDocumentPrefixes = [...][]byte{[]byte("<svg"), []byte("<?xml"), []byte("<!--")}

// decodeDocumentImage shares the same decoding path for embedded images and
// the full-window viewer. SVGs need rasterization rather than bitmap decoding.
func decodeDocumentImage(data []byte) (*ui.Bitmap, error) {
	if err := checkRasterDecodePixelBudget(data); err != nil {
		return nil, err
	}
	if bmp, err := ui.DecodeBitmap(data); err == nil {
		return bmp, nil
	}
	if !svgDocumentPrefix(data) {
		return nil, fmt.Errorf("unsupported image format")
	}
	c, err := canvas.ParseSVG(bytes.NewReader(data))
	if c == nil || err != nil {
		return nil, fmt.Errorf("invalid SVG: %v", err)
	}
	w, h := c.Size()
	_, density, ok := svgRasterSize(w, h)
	if !ok {
		return nil, fmt.Errorf("invalid SVG dimensions")
	}
	// Keep SVG raster allocations bounded while preserving intrinsic dimensions for ordinary images.
	img := rasterizer.Draw(c, canvas.DPMM(density), canvas.DefaultColorSpace)
	if img == nil {
		return nil, fmt.Errorf("cannot render SVG")
	}
	return ui.NewBitmap(img), nil
}

// Keep the source decode bounded before handing data to a decoder that
// allocates a full-size pixel plane. The rendered inline bitmap is reduced to
// 2MP later, but applying that limit after Decode is too late for pathological
// source dimensions. Match the existing single-frame GIF limit so ordinary
// high-resolution photos remain supported while a single image cannot demand
// multiple gigabytes of transient Go heap.
func checkRasterDecodePixelBudget(data []byte) error {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		// SVG and unsupported data are handled by their existing decode/error
		// paths below; DecodeConfig deliberately has no SVG decoder.
		return nil
	}
	if config.Width <= 0 || config.Height <= 0 {
		return fmt.Errorf("image has empty dimensions")
	}
	width, height := int64(config.Width), int64(config.Height)
	if width > maxSingleImageDecodePixels/height {
		return fmt.Errorf("image dimensions exceed the %d-pixel decode limit", maxSingleImageDecodePixels)
	}
	return nil
}

// imageIntrinsicSize reads dimensions without allocating decoded pixels. It
// is shared by every backend because it uses Go's image decoders and the
// existing SVG parser, not a renderer-specific bitmap API.
func imageIntrinsicSize(data []byte) (image.Point, bool) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		if config.Width <= 0 || config.Height <= 0 || int64(config.Width) > maxSingleImageDecodePixels/int64(config.Height) {
			return image.Point{}, false
		}
		return image.Pt(config.Width, config.Height), true
	}
	if !svgDocumentPrefix(data) {
		return image.Point{}, false
	}
	c, err := canvas.ParseSVG(bytes.NewReader(data))
	if c == nil || err != nil {
		return image.Point{}, false
	}
	w, h := c.Size()
	size, _, ok := svgRasterSize(w, h)
	if !ok {
		return image.Point{}, false
	}
	return size, true
}

// svgRasterSize mirrors rasterizer.Draw's pixel rounding and decode size cap.
// Returning its output dimensions makes a pending SVG's layout match the
// bitmap dimensions that decodeDocumentImage will produce.
func svgRasterSize(w, h float64) (image.Point, float64, bool) {
	if w <= 0 || h <= 0 || math.IsNaN(w) || math.IsNaN(h) || math.IsInf(w, 0) || math.IsInf(h, 0) {
		return image.Point{}, 0, false
	}
	density := math.Min(1, math.Min(4096/w, 4096/h))
	size := image.Pt(int(w*density+0.5), int(h*density+0.5))
	if size.X <= 0 || size.Y <= 0 {
		return image.Point{}, 0, false
	}
	return size, density, true
}

// validateImageData checks a cached or downloaded image without rasterizing it.
// The caller will decode it once when the ImageAsset is created; decoding here
// as well would transiently allocate a second full pixel buffer per image.
func validateImageData(data []byte) error {
	if svgDocumentPrefix(data) {
		c, err := canvas.ParseSVG(bytes.NewReader(data))
		if c == nil || err != nil {
			return fmt.Errorf("invalid SVG: %v", err)
		}
		w, h := c.Size()
		if w <= 0 || h <= 0 || math.IsNaN(w) || math.IsNaN(h) || math.IsInf(w, 0) || math.IsInf(h, 0) {
			return fmt.Errorf("invalid SVG dimensions")
		}
		return nil
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("invalid image data: %w", err)
	}
	return nil
}

// svgDocumentPrefix checks only the leading markup. Converting and lowercasing
// the full data buffer here copied large embedded images just to inspect a
// four or five byte prefix.
func svgDocumentPrefix(data []byte) bool {
	head := bytes.TrimLeftFunc(data, unicode.IsSpace)
	for _, prefix := range svgDocumentPrefixes {
		if hasFoldedPrefix(head, prefix) {
			return true
		}
	}
	return false
}

func hasFoldedPrefix(data, prefix []byte) bool {
	return len(data) >= len(prefix) && bytes.EqualFold(data[:len(prefix)], prefix)
}
