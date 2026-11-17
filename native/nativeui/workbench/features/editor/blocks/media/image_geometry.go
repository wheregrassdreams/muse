package media

import "github.com/egoist/mygo/ui"

// ImageSize describes explicit HTML/CSS image geometry in logical pixels.
type ImageSize struct {
	Width  float32
	Height float32
	Zoom   float32
}

// ImageSizeAtWidth reserves intrinsic geometry after applying the available
// width. It is shared by image layout and the placeholder shown while loading.
func ImageSizeAtWidth(w, h, width float32) (float32, float32) {
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	if width > 0 && w > width {
		h = h * width / w
		w = width
	}
	return w, h
}

// SizedImageDimensions applies an explicit HTML size hint and then clamps the
// result to the available content width.
func SizedImageDimensions(baseW, baseH float32, size ImageSize, width float32) (float32, float32) {
	if baseW <= 0 || baseH <= 0 {
		return 0, 0
	}
	fw, fh := baseW, baseH
	switch {
	case size.Width > 0 && size.Height > 0:
		fw, fh = size.Width, size.Height
	case size.Width > 0:
		fh = fh * size.Width / fw
		fw = size.Width
	case size.Height > 0:
		fw = fw * size.Height / fh
		fh = size.Height
	}
	if size.Zoom > 0 {
		fw, fh = fw*size.Zoom, fh*size.Zoom
	}
	if width > 0 && fw > width {
		fh = fh * width / fw
		fw = width
	}
	return fw, fh
}

// ToolRects returns the stable hit regions for image controls and resize grip.
func ToolRects(box ui.Rect, size float32) (preview, source, align, grip ui.Rect) {
	button := max(size*1.4, 22)
	preview = ui.Rect{X: box.X + box.W - button*3 - 6, Y: box.Y + 4, W: button, H: button}
	align = ui.Rect{X: box.X + box.W - button*2 - 4, Y: box.Y + 4, W: button, H: button}
	source = ui.Rect{X: box.X + box.W - button - 2, Y: box.Y + 4, W: button, H: button}
	grip = ui.Rect{X: box.X + box.W - button, Y: box.Y + box.H - button, W: button, H: button}
	return
}

// ImageToolAction is the semantic result of hit-testing an image affordance.
type ImageToolAction uint8

const (
	ImageToolNone ImageToolAction = iota
	ImageToolPreview
	ImageToolSource
	ImageToolAlign
	ImageToolResize
)

// ImageToolAt resolves a pointer against an image's control regions. The host
// decides which editor operation or application callback handles the result.
func ImageToolAt(box ui.Rect, size, x, y float32, block bool) ImageToolAction {
	preview, source, align, grip := ToolRects(box, size)
	switch {
	case preview.Contains(x, y):
		return ImageToolPreview
	case source.Contains(x, y):
		return ImageToolSource
	case block && align.Contains(x, y):
		return ImageToolAlign
	case grip.Contains(x, y):
		return ImageToolResize
	default:
		return ImageToolNone
	}
}

// ImageToolButton is a backend-neutral presentation instruction for one image
// tool. The editor renderer supplies actual colors, borders, and text shaping.
type ImageToolButton struct {
	Bounds ui.Rect
	Label  string
}

// ImageToolButtons returns the visible media controls in paint order.
func ImageToolButtons(box ui.Rect, size float32, block, centered bool) []ImageToolButton {
	preview, source, align, _ := ToolRects(box, size)
	buttons := []ImageToolButton{{Bounds: preview, Label: "↗"}}
	if block {
		label := "↤"
		if centered {
			label = "↔"
		}
		buttons = append(buttons, ImageToolButton{Bounds: align, Label: label})
	}
	return append(buttons, ImageToolButton{Bounds: source, Label: "</>"})
}
