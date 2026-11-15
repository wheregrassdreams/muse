package images

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"math"
	"strings"
	"time"
	"unicode"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	_ "image/jpeg"
	_ "image/png"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

var svgPrefixes = [...][]byte{[]byte("<svg"), []byte("<?xml"), []byte("<!--")}

// Decoded contains bounded, renderer-neutral pixels. Animated GIFs retain
// their indexed source frames and are composed incrementally by Asset.
type Decoded struct {
	Frames    []image.Image
	Delays    []time.Duration
	Animation *gif.GIF
}

// Decode validates compressed input and pixel budgets before allocating large
// raster planes. It publishes intrinsic dimensions before pixel decoding.
func Decode(data []byte, onSize func(image.Point)) (Decoded, error) {
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		budget, err := inspectGIFFrameBudget(data)
		if err != nil {
			return Decoded{}, fmt.Errorf("inspect GIF: %w", err)
		}
		if budget.firstPixels > maxSingleImageDecodePixels {
			return Decoded{}, fmt.Errorf("GIF first frame exceeds the %d-pixel decode limit", maxSingleImageDecodePixels)
		}
		if onSize != nil {
			onSize(image.Pt(budget.config.Width, budget.config.Height))
		}
		canvasPixels := int64(budget.config.Width) * int64(budget.config.Height)
		keepAnimation := budget.frames > 1 && budget.totalPixels <= maxAnimatedGIFFramePixels && canvasPixels <= maxInlineImagePixels
		if !keepAnimation {
			frame, err := gif.Decode(bytes.NewReader(data))
			if err != nil {
				return Decoded{}, fmt.Errorf("decode first GIF frame: %w", err)
			}
			canvas := image.Rect(0, 0, budget.config.Width, budget.config.Height)
			return Decoded{Frames: []image.Image{canonicalRGBA(limitInlineImagePixels(gifCanvasView(frame, canvas)))}}, nil
		}
		decoded, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return Decoded{}, fmt.Errorf("decode GIF: %w", err)
		}
		if len(decoded.Image) == 0 {
			return Decoded{}, fmt.Errorf("GIF has no image frames")
		}
		bounds := image.Rect(0, 0, decoded.Config.Width, decoded.Config.Height)
		if bounds.Empty() {
			return Decoded{}, fmt.Errorf("GIF has empty dimensions")
		}
		canvas := image.NewNRGBA(bounds)
		draw.Draw(canvas, decoded.Image[0].Bounds(), decoded.Image[0], decoded.Image[0].Bounds().Min, draw.Over)
		delays := make([]time.Duration, len(decoded.Image))
		for i := range delays {
			delay := 0
			if i < len(decoded.Delay) {
				delay = decoded.Delay[i]
			}
			delays[i] = max(time.Duration(delay)*10*time.Millisecond, 20*time.Millisecond)
		}
		return Decoded{Frames: []image.Image{cloneNRGBA(canvas)}, Delays: delays, Animation: decoded}, nil
	}
	if onSize != nil {
		if size, ok := IntrinsicSize(data); ok {
			onSize(size)
		}
	}
	if err := checkRasterDecodePixelBudget(data); err != nil {
		return Decoded{}, err
	}
	if img, ok := decodeLargePNGAtDisplaySize(data); ok {
		return Decoded{Frames: []image.Image{img}}, nil
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		if svg, svgErr := DecodeSVG(data); svgErr == nil {
			return Decoded{Frames: []image.Image{svg}}, nil
		}
		return Decoded{}, fmt.Errorf("decode image: %w", err)
	}
	if format == "jpeg" {
		img = orientJPEG(img, jpegOrientation(data))
	}
	return Decoded{Frames: []image.Image{canonicalRGBA(limitInlineImagePixels(img))}}, nil
}

// DecodeDocument decodes an image for the document/image-preview surface.
// Unlike inline Markdown assets it keeps the original raster dimensions, but
// still rejects sources above the single-image 16MP budget before decode.
func DecodeDocument(data []byte, onSize func(image.Point)) (Decoded, error) {
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		budget, err := inspectGIFFrameBudget(data)
		if err != nil {
			return Decoded{}, fmt.Errorf("inspect GIF: %w", err)
		}
		if budget.firstPixels > maxSingleImageDecodePixels {
			return Decoded{}, fmt.Errorf("GIF first frame exceeds the %d-pixel decode limit", maxSingleImageDecodePixels)
		}
		if onSize != nil {
			onSize(image.Pt(budget.config.Width, budget.config.Height))
		}
		frame, err := gif.Decode(bytes.NewReader(data))
		if err != nil {
			return Decoded{}, err
		}
		return Decoded{Frames: []image.Image{canonicalRGBA(gifCanvasView(frame, image.Rect(0, 0, budget.config.Width, budget.config.Height)))}}, nil
	}
	if onSize != nil {
		if p, ok := IntrinsicSize(data); ok {
			onSize(p)
		}
	}
	if err := checkRasterDecodePixelBudget(data); err != nil {
		return Decoded{}, err
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		img, err = DecodeSVG(data)
		if err != nil {
			return Decoded{}, fmt.Errorf("decode image: %w", err)
		}
	}
	if format == "jpeg" {
		img = orientJPEG(img, jpegOrientation(data))
	}
	return Decoded{Frames: []image.Image{canonicalRGBA(img)}}, nil
}

// IntrinsicSize reads dimensions without allocating decoded pixels. SVG sizes
// report the bounded raster dimensions used by DecodeSVG.
func IntrinsicSize(data []byte) (image.Point, bool) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		if config.Width <= 0 || config.Height <= 0 || int64(config.Width) > maxSingleImageDecodePixels/int64(config.Height) {
			return image.Point{}, false
		}
		w, h := config.Width, config.Height
		if w, h = orientedSize(w, h, jpegOrientation(data)); w <= 0 || h <= 0 {
			return image.Point{}, false
		}
		return image.Pt(w, h), true
	}
	if !svgPrefix(data) {
		return image.Point{}, false
	}
	c, err := canvas.ParseSVG(bytes.NewReader(data))
	if c == nil || err != nil {
		return image.Point{}, false
	}
	w, h := c.Size()
	size, _, ok := svgRasterSize(w, h)
	return size, ok
}

// DecodeSVG rasterizes a vector image into bounded renderer-neutral pixels.
func DecodeSVG(data []byte) (image.Image, error) {
	if err := checkRasterDecodePixelBudget(data); err != nil {
		return nil, err
	}
	if !svgPrefix(data) {
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
	img := rasterizer.Draw(c, canvas.DPMM(density), canvas.DefaultColorSpace)
	if img == nil {
		return nil, fmt.Errorf("cannot render SVG")
	}
	return img, nil
}

func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return 0
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xff {
			return 0
		}
		marker, size := data[i+1], int(binary.BigEndian.Uint16(data[i+2:]))
		if marker == 0xda || size < 2 || i+2+size > len(data) {
			return 0
		}
		seg := data[i+4 : i+2+size]
		if marker == 0xe1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 0
}

func tiffOrientation(data []byte) int {
	if len(data) < 8 {
		return 0
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0
	}
	ifd := int(order.Uint32(data[4:]))
	if ifd < 8 || ifd+2 > len(data) {
		return 0
	}
	n := int(order.Uint16(data[ifd:]))
	for e := ifd + 2; e+12 <= len(data) && n > 0; e, n = e+12, n-1 {
		if order.Uint16(data[e:]) == 0x0112 && order.Uint16(data[e+2:]) == 3 {
			o := int(order.Uint16(data[e+8:]))
			if o >= 1 && o <= 8 {
				return o
			}
			return 0
		}
	}
	return 0
}

func orientedSize(w, h, orientation int) (int, int) {
	if orientation >= 5 && orientation <= 8 {
		return h, w
	}
	return w, h
}

func orientJPEG(src image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := orientedSize(w, h, orientation)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch orientation {
			case 2:
				sx, sy = w-1-x, y
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			dst.Set(x, y, src.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return dst
}

// Validate checks an encoded image without allocating decoded pixel buffers.
func ValidateImageData(data []byte) error {
	if svgPrefix(data) {
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

func checkRasterDecodePixelBudget(data []byte) error {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	if config.Width <= 0 || config.Height <= 0 {
		return fmt.Errorf("image has empty dimensions")
	}
	if int64(config.Width) > maxSingleImageDecodePixels/int64(config.Height) {
		return fmt.Errorf("image dimensions exceed the %d-pixel decode limit", maxSingleImageDecodePixels)
	}
	return nil
}

func svgPrefix(data []byte) bool {
	head := bytes.TrimLeftFunc(data, unicode.IsSpace)
	for _, prefix := range svgPrefixes {
		if len(head) >= len(prefix) && bytes.EqualFold(head[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

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

func cloneNRGBA(src *image.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

// ValidateDataURL verifies the bounded base64 form and checks its declared
// image type against the first decoded bytes before allocating the payload.
func ValidateDataURL(src string) ([]byte, error) { return dataURLBytes(src) }

func dataURLBytes(src string) ([]byte, error) {
	comma := strings.IndexByte(src, ',')
	if comma < 0 || !strings.Contains(src[:comma], ";base64") {
		return nil, fmt.Errorf("image data URL is not base64")
	}
	encoded := strings.TrimSpace(src[comma+1:])
	if dataURLHeaderMismatch(src[:comma], encoded) {
		return nil, fmt.Errorf("image data URL does not match its declared format")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func dataURLHeaderMismatch(header, encoded string) bool {
	if colon := strings.IndexByte(header, ':'); colon >= 0 {
		header = header[colon+1:]
	}
	semi := strings.IndexByte(header, ';')
	const encodedPrefixBytes = 16
	if semi < 0 || len(encoded) < encodedPrefixBytes {
		return false
	}
	mimeType := header[:semi]
	var want []byte
	switch {
	case strings.EqualFold(mimeType, "image/png"):
		want = []byte("\x89PNG\r\n\x1a\n")
	case strings.EqualFold(mimeType, "image/jpeg"), strings.EqualFold(mimeType, "image/jpg"):
		want = []byte{0xff, 0xd8, 0xff}
	case strings.EqualFold(mimeType, "image/gif"):
		return !strings.HasPrefix(encoded, "R0lGOD")
	case strings.EqualFold(mimeType, "image/bmp"):
		want = []byte{'B', 'M'}
	case strings.EqualFold(mimeType, "image/webp"):
		var prefix [12]byte
		n, _ := base64.StdEncoding.Decode(prefix[:], []byte(encoded[:encodedPrefixBytes]))
		return n < 12 || string(prefix[:4]) != "RIFF" || string(prefix[8:12]) != "WEBP"
	default:
		return false
	}
	var prefix [12]byte
	n, _ := base64.StdEncoding.Decode(prefix[:], []byte(encoded[:encodedPrefixBytes]))
	return n < len(want) || !bytes.Equal(prefix[:len(want)], want)
}
