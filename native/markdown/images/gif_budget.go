package images

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/gif"
)

// DecodeAll retains one indexed pixel per source pixel for every GIF frame.
// Bound that retained animation data before invoking the standard decoder.
const (
	maxAnimatedGIFFramePixels  = int64(8 << 20)
	maxSingleImageDecodePixels = int64(16 << 20)
)

type gifFrameBudget struct {
	config         image.Config
	frames         int
	totalPixels    int64
	firstFrameRect image.Rectangle
	firstPixels    int64
}

func inspectGIFFrameBudget(data []byte) (gifFrameBudget, error) {
	var budget gifFrameBudget
	if len(data) < 13 || string(data[:6]) != "GIF87a" && string(data[:6]) != "GIF89a" {
		return budget, fmt.Errorf("invalid GIF header")
	}
	config, err := gif.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return budget, err
	}
	if config.Width <= 0 || config.Height <= 0 {
		return budget, fmt.Errorf("GIF has empty dimensions")
	}
	budget.config = config

	packed := data[10]
	offset := 13
	if packed&0x80 != 0 {
		offset += 3 * (1 << ((packed & 0x07) + 1))
	}
	if offset > len(data) {
		return gifFrameBudget{}, fmt.Errorf("truncated GIF global color table")
	}

	for offset < len(data) {
		switch data[offset] {
		case 0x3b: // trailer
			if budget.frames == 0 {
				return gifFrameBudget{}, fmt.Errorf("GIF has no image frames")
			}
			return budget, nil
		case 0x21: // extension and its data sub-blocks
			if offset+2 > len(data) {
				return gifFrameBudget{}, fmt.Errorf("truncated GIF extension")
			}
			offset += 2
			offset, err = skipGIFSubBlocks(data, offset)
			if err != nil {
				return gifFrameBudget{}, err
			}
		case 0x2c: // image descriptor, local table, LZW data
			if offset+10 > len(data) {
				return gifFrameBudget{}, fmt.Errorf("truncated GIF image descriptor")
			}
			left := int(binary.LittleEndian.Uint16(data[offset+1 : offset+3]))
			top := int(binary.LittleEndian.Uint16(data[offset+3 : offset+5]))
			width := int(binary.LittleEndian.Uint16(data[offset+5 : offset+7]))
			height := int(binary.LittleEndian.Uint16(data[offset+7 : offset+9]))
			flags := data[offset+9]
			if width <= 0 || height <= 0 {
				return gifFrameBudget{}, fmt.Errorf("GIF has empty frame dimensions")
			}
			pixels := int64(width) * int64(height)
			if budget.frames == 0 {
				budget.firstFrameRect = image.Rect(left, top, left+width, top+height)
				budget.firstPixels = pixels
			}
			budget.frames++
			if budget.totalPixels > int64(^uint64(0)>>1)-pixels {
				budget.totalPixels = int64(^uint64(0) >> 1)
			} else {
				budget.totalPixels += pixels
			}
			offset += 10
			if flags&0x80 != 0 {
				offset += 3 * (1 << ((flags & 0x07) + 1))
			}
			if offset >= len(data) {
				return gifFrameBudget{}, fmt.Errorf("truncated GIF image data")
			}
			offset++ // LZW minimum code size
			offset, err = skipGIFSubBlocks(data, offset)
			if err != nil {
				return gifFrameBudget{}, err
			}
		default:
			return gifFrameBudget{}, fmt.Errorf("unknown GIF block marker 0x%02x", data[offset])
		}
	}
	return gifFrameBudget{}, fmt.Errorf("GIF is missing its trailer")
}

func skipGIFSubBlocks(data []byte, offset int) (int, error) {
	for {
		if offset >= len(data) {
			return 0, fmt.Errorf("truncated GIF data sub-block")
		}
		size := int(data[offset])
		offset++
		if size == 0 {
			return offset, nil
		}
		if size > len(data)-offset {
			return 0, fmt.Errorf("truncated GIF data block")
		}
		offset += size
	}
}

func gifCanvasView(frame image.Image, canvas image.Rectangle) image.Image {
	return gifCanvasImage{frame: frame, canvas: canvas}
}

type gifCanvasImage struct {
	frame  image.Image
	canvas image.Rectangle
}

func (img gifCanvasImage) ColorModel() color.Model { return color.NRGBAModel }
func (img gifCanvasImage) Bounds() image.Rectangle { return img.canvas }
func (img gifCanvasImage) At(x, y int) color.Color {
	if image.Pt(x, y).In(img.frame.Bounds()) {
		return img.frame.At(x, y)
	}
	return color.NRGBA{}
}
