package images

import (
	"image"
	"math"

	xdraw "golang.org/x/image/draw"
)

// Inline images are shown inside the editor column, where more than two
// megapixels do not add useful display detail on current screens. Keeping a
// bounded pixel plane also caps each retained image asset before it enters
// the editor's byte-limited cache.
const maxInlineImagePixels = int64(2 << 20)

func limitInlineImagePixels(img image.Image) image.Image {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 || int64(w)*int64(h) <= maxInlineImagePixels {
		return img
	}

	dw, dh := inlineImageTarget(w, h)

	// The scaler has an optimized Src path for *image.RGBA. Use that type
	// here; the generic NRGBA destination path can leave pixels untouched for
	// image implementations that do not expose a concrete pixel buffer.
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, xdraw.Src, nil)
	return dst
}

// inlineImageTarget returns dimensions within the inline image pixel budget
// while preserving the source aspect ratio as closely as integer dimensions
// allow. The zero-dimension cases matter for extreme aspect ratios: repairing
// them by decrementing the long edge one pixel at a time can take millions of
// iterations for otherwise valid images.
func inlineImageTarget(width, height int) (int, int) {
	if width <= 0 || height <= 0 || int64(width)*int64(height) <= maxInlineImagePixels {
		return width, height
	}
	scale := math.Sqrt(float64(maxInlineImagePixels) / float64(width) / float64(height))
	dw := int(math.Floor(float64(width) * scale))
	dh := int(math.Floor(float64(height) * scale))
	if dw < 1 {
		return 1, min(height, int(maxInlineImagePixels))
	}
	if dh < 1 {
		return min(width, int(maxInlineImagePixels)), 1
	}
	if int64(dw)*int64(dh) > maxInlineImagePixels {
		if dw >= dh {
			dw--
		} else {
			dh--
		}
	}
	return dw, dh
}
