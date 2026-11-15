package markdown

import (
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"image"
	"image/color"
	"strings"
	"testing"
)

type boundedTestImage struct{ bounds image.Rectangle }

func (img boundedTestImage) ColorModel() color.Model { return color.NRGBAModel }
func (img boundedTestImage) Bounds() image.Rectangle { return img.bounds }
func (img boundedTestImage) At(x, y int) color.Color {
	return color.NRGBA{R: uint8(x), G: uint8(y), B: 160, A: 255}
}

func TestLimitInlineImagePixelsPreservesSmallImageAndBoundsLargeImage(t *testing.T) {
	small := boundedTestImage{bounds: image.Rect(0, 0, 1024, 1024)}
	if got := limitInlineImagePixels(small); got != small {
		t.Fatal("image under the pixel budget was copied")
	}

	large := boundedTestImage{bounds: image.Rect(0, 0, 8192, 4096)}
	got := limitInlineImagePixels(large)
	bounds := got.Bounds()
	pixels := int64(bounds.Dx()) * int64(bounds.Dy())
	if pixels > maxInlineImagePixels {
		t.Fatalf("scaled image has %d pixels, over the %d pixel budget", pixels, maxInlineImagePixels)
	}
	if ratio := float64(bounds.Dx()) / float64(bounds.Dy()); ratio < 1.99 || ratio > 2.01 {
		t.Fatalf("scaled aspect ratio = %.3f, want 2.0", ratio)
	}
	_, _, b, a := got.At(bounds.Dx()/2, bounds.Dy()/2).RGBA()
	if b == 0 || a == 0 {
		t.Fatalf("scaled image pixel = b:%d a:%d bounds=%v source=%v", b, a, bounds, large.At(large.bounds.Dx()/2, large.bounds.Dy()/2))
	}
}

func TestRasterDecodePixelBudgetRejectsOversizedPNGBeforeDecode(t *testing.T) {
	data := pngConfigHeader(maxSingleImageDecodePixels+1, 1)
	if err := checkRasterDecodePixelBudget(data); err == nil {
		t.Fatal("oversized PNG dimensions passed the decode budget")
	}
	if _, _, _, _, err := decodeImageAsset(data); err == nil || !strings.Contains(err.Error(), "decode limit") {
		t.Fatalf("image asset decoder error = %v, want pre-decode budget error", err)
	}
	if _, err := decodeDocumentImage(data); err == nil || !strings.Contains(err.Error(), "decode limit") {
		t.Fatalf("document image decoder error = %v, want pre-decode budget error", err)
	}
}

func pngConfigHeader(width, height int64) []byte {
	header := make([]byte, 8, 33)
	decoded, _ := hex.DecodeString("89504e470d0a1a0a")
	copy(header, decoded)
	chunk := make([]byte, 4+4+13+4)
	binary.BigEndian.PutUint32(chunk[:4], 13)
	copy(chunk[4:8], "IHDR")
	binary.BigEndian.PutUint32(chunk[8:12], uint32(width))
	binary.BigEndian.PutUint32(chunk[12:16], uint32(height))
	chunk[16] = 8 // bit depth
	chunk[17] = 6 // truecolor with alpha
	chunk[18] = 0 // compression method
	chunk[19] = 0 // filter method
	chunk[20] = 0 // no interlace
	binary.BigEndian.PutUint32(chunk[21:25], crc32.ChecksumIEEE(chunk[4:21]))
	return append(header, chunk...)
}

func TestRasterDecodePixelBudgetAllowsNormalPhotoDimensions(t *testing.T) {
	data := pngConfigHeader(4000, 3000)
	if err := checkRasterDecodePixelBudget(data); err != nil {
		t.Fatalf("12MP image rejected: %v", err)
	}
}
