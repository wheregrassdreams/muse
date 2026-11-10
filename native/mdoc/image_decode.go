package mdoc

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

// decodeDocumentImage shares the same decoding path for embedded images and
// the full-window viewer. SVGs need rasterization rather than bitmap decoding.
func decodeDocumentImage(data []byte) (*ui.Bitmap, error) {
	if bmp, err := ui.DecodeBitmap(data); err == nil {
		return bmp, nil
	}
	head := strings.ToLower(string(bytes.TrimSpace(data)))
	if !strings.HasPrefix(head, "<svg") && !strings.HasPrefix(head, "<?xml") && !strings.HasPrefix(head, "<!--") {
		return nil, fmt.Errorf("unsupported image format")
	}
	c, err := canvas.ParseSVG(bytes.NewReader(data))
	if c == nil || err != nil {
		return nil, fmt.Errorf("invalid SVG: %v", err)
	}
	w, h := c.Size()
	if w <= 0 || h <= 0 || math.IsNaN(w) || math.IsNaN(h) || math.IsInf(w, 0) || math.IsInf(h, 0) {
		return nil, fmt.Errorf("invalid SVG dimensions")
	}
	// Keep SVG raster allocations bounded while preserving intrinsic dimensions for ordinary images.
	density := math.Min(1, math.Min(4096/w, 4096/h))
	img := rasterizer.Draw(c, canvas.DPMM(density), canvas.DefaultColorSpace)
	if img == nil {
		return nil, fmt.Errorf("cannot render SVG")
	}
	return ui.NewBitmap(img), nil
}
