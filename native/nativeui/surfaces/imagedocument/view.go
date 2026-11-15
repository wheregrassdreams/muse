// Package imagedocument renders a read-only image as the active document.
package imagedocument

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"muse/locales"
	"muse/markdown/images"
	markdownui "muse/nativeui/components/markdown"
)

type ActionKind uint8

const (
	ZoomBy ActionKind = iota
	PanBy
	BeginDrag
	EndDrag
	ResetView
	OpenPreview
)

type Action struct {
	Kind               ActionKind
	Scale              float32
	DX, DY             float32
	PointerX, PointerY float32
}

type ViewModel struct {
	Path          string
	Bitmap        *ui.Bitmap
	Zoom          float32
	X, Y          float32
	Dragging      bool
	DragX, DragY  float32
	BacklinkCount int
}

type state struct {
	path         string
	bmp          *ui.Bitmap
	open         bool
	zoom         float32
	x, y         float32
	dragging     bool
	dragX, dragY float32
}

func (s *state) Open(path string, cache images.CacheStore, files images.LocalFiles) {
	if s == nil || path == "" {
		return
	}
	*s = state{path: path, bmp: markdownui.LoadImageWithLocalFiles(path, "", files, cache, nil), open: true, zoom: 1}
}

func (s *state) Close() {
	if s == nil {
		return
	}
	*s = state{}
}

func (s state) IsOpen() bool { return s.open }

func (s state) Path() string { return s.path }

func (s state) HasImage() bool { return s.bmp != nil }

func (s state) ViewModel(backlinkCount int) ViewModel {
	return ViewModel{
		Path: s.path, Bitmap: s.bmp, Zoom: s.zoom, X: s.x, Y: s.y,
		Dragging: s.dragging, DragX: s.dragX, DragY: s.dragY,
		BacklinkCount: backlinkCount,
	}
}

func (s *state) Apply(action Action) {
	if s == nil || !s.open {
		return
	}
	switch action.Kind {
	case ZoomBy:
		if action.Scale > 0 {
			s.zoom = clampZoom(s.zoom * action.Scale)
		}
	case PanBy:
		s.x += action.DX
		s.y += action.DY
	case BeginDrag:
		s.dragging, s.dragX, s.dragY = true, action.PointerX, action.PointerY
	case EndDrag:
		s.dragging = false
	case ResetView:
		s.zoom, s.x, s.y = 1, 0, 0
	}
}

type Options struct {
	Header       func()
	HeaderHeight float32
	OnBacklinks  func()
	OnFullscreen func()
	OnCopy       func()
}

func Render(c *ui.Context, vm ViewModel, opts Options, dispatch func(Action)) {
	if vm.Path == "" {
		return
	}
	emit := func(action Action) {
		if dispatch != nil {
			dispatch(action)
		}
	}
	dragging, dragX, dragY := vm.Dragging, vm.DragX, vm.DragY
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		if opts.Header != nil {
			opts.Header()
		}
		ui.Box(c).Height(opts.HeaderHeight).Shrink(0).Key("image-document-header-spacer")
		if vm.BacklinkCount > 0 {
			ui.Row(c).Height(36).Shrink(0).Padding(0, 12).AlignItems(ui.Center).Children(func() {
				b := ui.ButtonBase(c).Height(30).Padding(0, 7).Radius(5).Label(locales.PropertyViewBacklinks())
				if b.Hovered() {
					b.Background(t.SurfaceHover)
				}
				b.OnClick(opts.OnBacklinks)
				b.Children(func() {
					ui.Text(c, locales.PropertyBacklinkCount(vm.BacklinkCount)).FontSize(13).TextColor(t.TextMuted)
				})
			})
		}
		canvas := ui.Box(c).Grow(1).FillWidth().MinHeight(0).Key("image-document-canvas").Clip().HandleInput(func(ev ui.InputEvent) bool {
			switch ev.Kind {
			case ui.InputScroll:
				if ev.Precise {
					emit(Action{Kind: PanBy, DX: -ev.DX, DY: -ev.DY})
					return true
				}
				if ev.DY != 0 {
					emit(Action{Kind: ZoomBy, Scale: float32(math.Pow(1.12, float64(-ev.DY/60)))})
					return true
				}
			case ui.InputZoom:
				if ev.Scale > 0 {
					emit(Action{Kind: ZoomBy, Scale: ev.Scale})
					return true
				}
			case ui.InputPointerDown:
				if ev.Button == 0 {
					emit(Action{Kind: BeginDrag, PointerX: ev.X, PointerY: ev.Y})
					dragging, dragX, dragY = true, ev.X, ev.Y
					return true
				}
			case ui.InputPointerMove:
				if dragging && ev.Button == 0 {
					emit(Action{Kind: PanBy, DX: ev.X - dragX, DY: ev.Y - dragY})
					emit(Action{Kind: BeginDrag, PointerX: ev.X, PointerY: ev.Y})
					dragX, dragY = ev.X, ev.Y
					return true
				}
			case ui.InputPointerUp:
				if dragging {
					emit(Action{Kind: EndDrag})
					dragging = false
					return true
				}
			}
			return false
		}).ContextMenu(func(m *ui.Menu) {
			if m.Item(locales.CommonCopyAction()).Chosen() && opts.OnCopy != nil {
				opts.OnCopy()
			}
		})
		vw, vh := canvas.Bounds().W, canvas.Bounds().H
		canvas.Children(func() {
			if vm.Bitmap == nil {
				ui.Text(c, locales.ImageDecodeFailed()).Key("image-document-error").TextColor(t.TextMuted)
				return
			}
			bw, bh := ui.ImageSize(vm.Bitmap)
			if bw <= 0 || bh <= 0 {
				return
			}
			if vw <= 0 || vh <= 0 {
				vw, vh = 1200, 800
			}
			fit := imageInitialScale(bw, bh, vw-64, vh-64) * vm.Zoom
			ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Image(c, vm.Bitmap).Key("image-document-image").Size(bw*fit, bh*fit).Translate(vm.X, vm.Y)
			})
		})
		if vm.Bitmap != nil {
			bw, bh := ui.ImageSize(vm.Bitmap)
			percent := imageInitialScale(bw, bh, vw-64, vh-64) * vm.Zoom
			toolbarTop := opts.HeaderHeight + 12
			if vm.BacklinkCount > 0 {
				toolbarTop += 36
			}
			ui.Row(c).Key("image-document-toolbar").Absolute().Top(toolbarTop).Right(16).Gap(6).AlignItems(ui.Center).Children(func() {
				button(c, "−", locales.ImageZoomOut(), "image-document-zoom-out", func() { emit(Action{Kind: ZoomBy, Scale: 1 / 1.25}) })
				ui.Text(c, fmt.Sprintf("%.0f%%", percent*100)).Key("image-document-zoom").Width(48).TextAlign(ui.Center).TextColor(t.Text)
				button(c, "+", locales.ImageZoomIn(), "image-document-zoom-in", func() { emit(Action{Kind: ZoomBy, Scale: 1.25}) })
				button(c, locales.ImageFitWindow(), locales.ImageFitWindow(), "image-document-fit", func() { emit(Action{Kind: ResetView}) })
				button(c, "⛶", locales.ImagePreviewTitle(), "image-document-fullscreen", func() {
					if opts.OnFullscreen != nil {
						opts.OnFullscreen()
					}
				})
			})
		}
	})
}

func clampZoom(zoom float32) float32 { return min(float32(8), max(float32(0.1), zoom)) }

func imageInitialScale(bw, bh, vw, vh float32) float32 {
	if bw <= 0 || bh <= 0 || vw <= 0 || vh <= 0 {
		return 1
	}
	return min(float32(1), min(vw/bw, vh/bh))
}

func button(c *ui.Context, text, label, key string, click func()) {
	b := ui.Button(c, text).Key(key).Size(34, 32).Label(label)
	if key == "image-document-fit" {
		b.Size(88, 32)
	}
	if b.Clicked() {
		click()
	}
}
