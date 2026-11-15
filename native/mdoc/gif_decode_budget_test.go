package mdoc

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

func TestLargeAnimatedGIFFallsBackToBoundedFirstFrame(t *testing.T) {
	const width, height = 2048, 1536
	palette := color.Palette{color.RGBA{A: 255}, color.RGBA{R: 255, A: 255}}
	frames := make([]*image.Paletted, 3)
	for i := range frames {
		frames[i] = image.NewPaletted(image.Rect(0, 0, width, height), palette)
		for j := range frames[i].Pix {
			frames[i].Pix[j] = 1
		}
	}
	var encoded bytes.Buffer
	if err := gif.EncodeAll(&encoded, &gif.GIF{
		Image:  frames,
		Delay:  []int{2, 2, 2},
		Config: image.Config{ColorModel: palette, Width: width, Height: height},
	}); err != nil {
		t.Fatal(err)
	}

	budget, err := inspectGIFFrameBudget(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if budget.frames != len(frames) || budget.totalPixels <= maxAnimatedGIFFramePixels {
		t.Fatalf("GIF budget = %+v, want %d frames above %d pixels", budget, len(frames), maxAnimatedGIFFramePixels)
	}
	decoded, delays, direct, animation, err := decodeImageAsset(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if direct != nil || animation != nil || len(delays) != 0 || len(decoded) != 1 {
		t.Fatalf("oversized animation decode = frames:%d delays:%d direct:%v animation:%v", len(decoded), len(delays), direct != nil, animation != nil)
	}
	bounds := decoded[0].Bounds()
	if pixels := int64(bounds.Dx()) * int64(bounds.Dy()); pixels > maxInlineImagePixels {
		t.Fatalf("first-frame preview has %d pixels, limit is %d", pixels, maxInlineImagePixels)
	}
	r, _, _, a := decoded[0].At(bounds.Dx()/2, bounds.Dy()/2).RGBA()
	if r == 0 || a == 0 {
		t.Fatalf("first-frame preview lost its image pixels: rgba=(%d, _, _, %d)", r, a)
	}
}

func TestOversizedGIFFirstFrameIsRejectedBeforeDecode(t *testing.T) {
	data := []byte{
		'G', 'I', 'F', '8', '9', 'a',
		1, 0, 1, 0, 0, 0, 0,
		0x2c, 0, 0, 0, 0,
		0xff, 0xff, 0xff, 0xff, 0,
		2, 0, 0x3b,
	}
	binary.LittleEndian.PutUint16(data[6:8], 1)
	binary.LittleEndian.PutUint16(data[8:10], 1)

	budget, err := inspectGIFFrameBudget(data)
	if err != nil {
		t.Fatal(err)
	}
	if budget.firstPixels <= maxSingleImageDecodePixels {
		t.Fatalf("first GIF frame has %d pixels, want above %d", budget.firstPixels, maxSingleImageDecodePixels)
	}
	if _, _, _, _, err := decodeImageAsset(data); err == nil {
		t.Fatal("oversized GIF first frame decoded instead of being rejected")
	}
}
