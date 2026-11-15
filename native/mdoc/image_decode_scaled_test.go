package mdoc

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestScaledPNGTargetRespectsPixelBudgetAndAspectRatio(t *testing.T) {
	for _, tc := range []struct {
		w, h int
	}{
		{3128, 2202},
		{2100, 2000},
		{4000, 1000},
	} {
		w, h := scaledPNGTarget(tc.w, tc.h)
		if int64(w)*int64(h) > maxInlineImagePixels {
			t.Fatalf("target %dx%d exceeds %d pixels", w, h, maxInlineImagePixels)
		}
		gotAspect := float64(w) / float64(h)
		wantAspect := float64(tc.w) / float64(tc.h)
		if d := gotAspect - wantAspect; d < -0.002 || d > 0.002 {
			t.Fatalf("target aspect %.5f differs from source %.5f", gotAspect, wantAspect)
		}
	}
	if w, h := scaledPNGTarget(1200, 800); w != 1200 || h != 800 {
		t.Fatalf("small PNG target = %dx%d, want 1200x800", w, h)
	}
}

func TestInlineImageTargetExtremeAspectRatioIsBoundedInConstantTime(t *testing.T) {
	for _, tc := range []struct {
		width, height, wantWidth, wantHeight int
	}{
		{16 << 20, 1, int(maxInlineImagePixels), 1},
		{1, 16 << 20, 1, int(maxInlineImagePixels)},
	} {
		w, h := inlineImageTarget(tc.width, tc.height)
		if w != tc.wantWidth || h != tc.wantHeight {
			t.Fatalf("inlineImageTarget(%d, %d) = %dx%d, want %dx%d", tc.width, tc.height, w, h, tc.wantWidth, tc.wantHeight)
		}
		if int64(w)*int64(h) > maxInlineImagePixels {
			t.Fatalf("inlineImageTarget(%d, %d) exceeds pixel budget: %dx%d", tc.width, tc.height, w, h)
		}
	}
}

func TestScaledPNGMemoryBudgetEstimates(t *testing.T) {
	if got := linearPNGAxisCacheEstimate(8, 4, false); got != 80 {
		t.Fatalf("horizontal coefficient estimate = %d bytes, want 80", got)
	}
	if got := linearPNGAxisCacheEstimate(8, 4, true); got != 96 {
		t.Fatalf("vertical coefficient estimate = %d bytes, want 96", got)
	}

	w, h := scaledPNGTarget(3128, 2202)
	if !scaledPNGMemoryBudgetOK(3128, 2202, w, h) {
		t.Fatalf("ordinary large note image %dx%d -> %dx%d was rejected", 3128, 2202, w, h)
	}
	for _, tc := range []struct{ width, height int }{
		{16 << 20, 1},
		{1, 16 << 20},
		{8 << 20, 1},
		{1, 8 << 20},
	} {
		dw, dh := scaledPNGTarget(tc.width, tc.height)
		if scaledPNGMemoryBudgetOK(tc.width, tc.height, dw, dh) {
			t.Fatalf("extreme image %dx%d -> %dx%d passed memory estimate", tc.width, tc.height, dw, dh)
		}
	}
}

func TestDecodeLargePNGAtDisplaySize(t *testing.T) {
	const width, height = 2100, 2000 // 4.2 MP: just above the scaled path threshold.
	src := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, src); err != nil {
		t.Fatal(err)
	}
	img, ok := decodeLargePNGAtDisplaySize(data.Bytes())
	if !ok || img == nil {
		t.Fatal("large non-interlaced PNG did not use the scaled decoder")
	}
	if got := int64(img.Bounds().Dx()) * int64(img.Bounds().Dy()); got > maxInlineImagePixels {
		t.Fatalf("scaled image has %d pixels, over budget %d", got, maxInlineImagePixels)
	}
}

func TestScaledPNGThresholdAndInterlaceGate(t *testing.T) {
	if scaledPNGThresholdPixels != 4<<20 {
		t.Fatalf("scaled PNG threshold = %d, want 4 MP", scaledPNGThresholdPixels)
	}
	data := make([]byte, 29)
	copy(data, []byte{137, 80, 78, 71, 13, 10, 26, 10})
	copy(data[12:], []byte("IHDR"))
	data[28] = 1 // interlace method
	if _, ok := decodeLargePNGAtDisplaySize(data); ok {
		t.Fatal("interlaced PNG was passed to scaled decoder")
	}
}
