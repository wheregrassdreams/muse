package images

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"strings"
	"testing"
)

func TestDecodePNGNormalizesToOriginRGBAAndReportsSize(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 32, 18))); err != nil {
		t.Fatal(err)
	}
	var hinted image.Point
	decoded, err := Decode(encoded.Bytes(), func(p image.Point) { hinted = p })
	if err != nil {
		t.Fatal(err)
	}
	if hinted != image.Pt(32, 18) || len(decoded.Frames) != 1 {
		t.Fatalf("size/frame=(%v,%d)", hinted, len(decoded.Frames))
	}
	rgba, ok := decoded.Frames[0].(*image.RGBA)
	if !ok || rgba.Bounds() != image.Rect(0, 0, 32, 18) {
		t.Fatalf("decoded image=%T %v; want origin RGBA", decoded.Frames[0], decoded.Frames[0].Bounds())
	}
}

func TestDecodeSVGAndDataURLValidation(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="80"><rect width="120" height="80" fill="red"/></svg>`)
	if p, ok := IntrinsicSize(svg); !ok || p != image.Pt(120, 80) {
		t.Fatalf("SVG intrinsic=(%v,%v)", p, ok)
	}
	decoded, err := Decode(svg, nil)
	if err != nil || len(decoded.Frames) != 1 {
		t.Fatalf("SVG decode frames=%d err=%v", len(decoded.Frames), err)
	}
	pngData := pngFixture(t, 1, 1)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData)
	got, err := ValidateDataURL(uri)
	if err != nil || !bytes.Equal(got, pngData) {
		t.Fatalf("data URL decode bytes=%d err=%v", len(got), err)
	}
	if err := ValidateImageData(pngData); err != nil {
		t.Fatalf("valid PNG header rejected: %v", err)
	}
	if err := ValidateImageData([]byte("not an image")); err == nil {
		t.Fatal("invalid image header accepted")
	}
	bad := "data:image/png;base64," + strings.Repeat("A", 1<<20)
	if data, err := ValidateDataURL(bad); err == nil || data != nil {
		t.Fatal("mismatched PNG data URL was decoded")
	}
}

func TestRasterDecodeBudgetRejectsOversizedHeader(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	binary.BigEndian.PutUint32(data[16:20], uint32(maxSingleImageDecodePixels+1))
	binary.BigEndian.PutUint32(data[20:24], 1)
	crc := crc32.ChecksumIEEE(data[12:29])
	binary.BigEndian.PutUint32(data[29:33], crc)
	if err := checkRasterDecodePixelBudget(data); err == nil {
		t.Fatal("over-budget PNG dimensions were accepted")
	}
	if _, err := Decode(data, nil); err == nil {
		t.Fatal("over-budget PNG was decoded")
	}
}

func TestLargeAnimatedGIFFallsBackToBoundedFirstFrame(t *testing.T) {
	const w, h = 2048, 1536
	palette := color.Palette{color.RGBA{A: 255}, color.RGBA{R: 255, A: 255}}
	frames := make([]*image.Paletted, 3)
	for i := range frames {
		frames[i] = image.NewPaletted(image.Rect(0, 0, w, h), palette)
		for p := range frames[i].Pix {
			frames[i].Pix[p] = uint8(i % 2)
		}
	}
	var out bytes.Buffer
	if err := gif.EncodeAll(&out, &gif.GIF{Image: frames, Delay: []int{2, 2, 2}, Config: image.Config{ColorModel: palette, Width: w, Height: h}}); err != nil {
		t.Fatal(err)
	}
	budget, err := inspectGIFFrameBudget(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if budget.totalPixels <= maxAnimatedGIFFramePixels {
		t.Fatalf("GIF total pixels=%d, budget=%d", budget.totalPixels, maxAnimatedGIFFramePixels)
	}
	decoded, err := Decode(out.Bytes(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Animation != nil || len(decoded.Delays) != 0 || len(decoded.Frames) != 1 {
		t.Fatalf("large animation result=%+v", decoded)
	}
	b := decoded.Frames[0].Bounds()
	if p := int64(b.Dx()) * int64(b.Dy()); p > maxInlineImagePixels {
		t.Fatalf("fallback has %d pixels, limit %d", p, maxInlineImagePixels)
	}
}

func TestOversizedGIFFirstFrameRejectedBeforeDecode(t *testing.T) {
	data := []byte{'G', 'I', 'F', '8', '9', 'a', 1, 0, 1, 0, 0, 0, 0, 0x2c, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0, 2, 0, 0x3b}
	if _, err := Decode(data, nil); err == nil {
		t.Fatal("oversized first GIF frame was accepted")
	}
}
