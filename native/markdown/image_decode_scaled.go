package markdown

import (
	"bytes"
	"image"

	pngscaled "github.com/m8rge/go-scaled-png"
)

// scaledPNGThresholdPixels avoids adding resampling work to ordinary images.
// Inline images above this size are reduced during decoding to the same pixel
// budget used by the regular decode path.
const scaledPNGThresholdPixels int64 = 4 << 20

// Keep the estimated scratch and both coefficient tables for one candidate
// below 12 MiB. With two image decode slots, this leaves room for a previous
// pair of global cache entries while staying below the user's 50 MiB notice
// threshold for reachable candidate/cache allocations.
const scaledPNGMaxCandidateBytes int64 = 12 << 20

// decodeLargePNGAtDisplaySize uses a pure-Go decoder so this policy does not
// depend on the UI renderer or operating system. Interlaced PNGs are left to
// image/png because the scaled decoder returns them at their full dimensions.
// A false result asks the caller to use the normal decoder.
func decodeLargePNGAtDisplaySize(data []byte) (image.Image, bool) {
	if len(data) < 29 || !bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) ||
		!bytes.Equal(data[12:16], []byte("IHDR")) || data[28] != 0 {
		return nil, false
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "png" || config.Width <= 0 || config.Height <= 0 {
		return nil, false
	}
	pixels := int64(config.Width) * int64(config.Height)
	if pixels <= scaledPNGThresholdPixels || pixels > maxSingleImageDecodePixels {
		return nil, false
	}
	w, h := scaledPNGTarget(config.Width, config.Height)
	if !scaledPNGMemoryBudgetOK(config.Width, config.Height, w, h) {
		return nil, false
	}
	img, err := pngscaled.Decode(bytes.NewReader(data), w, h, pngscaled.Linear)
	if err != nil || img == nil || img.Bounds().Dx() > w || img.Bounds().Dy() > h {
		return nil, false
	}
	return img, true
}

// scaledPNGMemoryBudgetOK conservatively bounds the allocations specific to
// one candidate. It is a live allocation budget for the known row scratch and
// coefficient tables, not a guarantee about process RSS or unreclaimed Go heap
// pages. The width*4 row scratch bound covers paletted conversion scratch and
// intentionally overestimates other PNG color types.
func scaledPNGMemoryBudgetOK(srcW, srcH, dstW, dstH int) bool {
	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 {
		return false
	}
	var estimate int64
	if dstW < srcW {
		estimate += int64(srcW) * 4
	}
	estimate += linearPNGAxisCacheEstimate(srcW, dstW, false)
	estimate += linearPNGAxisCacheEstimate(srcH, dstH, true)
	return estimate <= scaledPNGMaxCandidateBytes
}

// linearPNGAxisCacheEstimate returns a conservative byte estimate for one
// cached coefficient table. The horizontal bound allows ceil(2*scale)+1 taps
// per output coordinate. The vertical bound follows the decoder's actual
// capacity formula: 2*ceil(scale)+3 taps per output coordinate.
func linearPNGAxisCacheEstimate(src, dst int, vertical bool) int64 {
	if src <= dst || src <= 0 || dst <= 0 {
		return 0
	}
	s, d := int64(src), int64(dst)
	var maxTaps int64
	if vertical {
		ceilScale := (s + d - 1) / d
		maxTaps = 2*ceilScale + 3
	} else {
		ceilTwiceScale := (2*s + d - 1) / d
		maxTaps = ceilTwiceScale + 1
	}
	// left/off/cnt use 4+4+2 bytes per output coordinate; weights are int16.
	return d*10 + d*maxTaps*2
}

func scaledPNGTarget(width, height int) (int, int) {
	return inlineImageTarget(width, height)
}
