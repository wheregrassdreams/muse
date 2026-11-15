// Package imagepreview owns the global image preview model and its native UI.
package imagepreview

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"muse/locales"
	"muse/markdown/images"
	markdownui "muse/nativeui/components/markdown"
)

const (
	minZoom = float32(0.1)
	maxZoom = float32(8)
)

type ActionKind uint8

const (
	Close ActionKind = iota
	Copy
	ZoomBy
	PanBy
	BeginDrag
	EndDrag
	ResetView
	SetBaseZoom
)

// Action describes a user intent emitted by Render. State transitions stay in
// the panel model and app-level effects are handled by the composition shell.
type Action struct {
	Kind               ActionKind
	Source             string
	Scale              float32
	DX, DY             float32
	PointerX, PointerY float32
}

type ViewModel struct {
	Open         bool
	Source       string
	Label        string
	Bitmap       *ui.Bitmap
	Loading      bool
	Zoom         float32
	BaseZoom     float32
	X, Y         float32
	Dragging     bool
	DragX, DragY float32
}

type state struct {
	source       string
	label        string
	asset        *markdownui.ImageAsset
	bmp          *ui.Bitmap
	loading      bool
	zoom         float32
	baseZoom     float32
	x, y         float32
	dragging     bool
	dragX, dragY float32
	open         bool
}

func (s *state) Open(source, label string, cache images.CacheStore, files images.LocalFiles, onMain func(func()), invalidate func()) {
	if s == nil || source == "" {
		return
	}
	*s = state{source: source, label: label, open: true, loading: true, zoom: 1}
	s.asset = markdownui.LoadImageAssetWithLocalFiles(source, "", files, cache, nil, func() {
		complete := func() {
			if !s.open || s.source != source {
				return
			}
			s.Advance(time.Now())
			s.loading = false
			if invalidate != nil {
				invalidate()
			}
		}
		if onMain == nil {
			complete()
		} else {
			onMain(complete)
		}
	})
	s.Advance(time.Now())
	if s.bmp != nil {
		s.loading = false
	} else if s.asset == nil || s.asset.Failed() {
		s.loading = false
	}
}

func (s *state) Close() {
	if s == nil {
		return
	}
	s.open = false
	s.asset = nil
	s.bmp = nil
	s.loading = false
	s.dragging = false
}

func (s *state) Advance(now time.Time) time.Duration {
	if s == nil || s.asset == nil {
		return 0
	}
	next, _ := s.asset.Advance(now)
	s.bmp = s.asset.Bitmap()
	return next
}

func (s state) ViewModel() ViewModel {
	return ViewModel{
		Open: s.open, Source: s.source, Label: s.label, Bitmap: s.bmp,
		Loading: s.loading, Zoom: s.zoom, BaseZoom: s.baseZoom,
		X: s.x, Y: s.y, Dragging: s.dragging, DragX: s.dragX, DragY: s.dragY,
	}
}

func (s state) IsOpen() bool { return s.open }

func (s state) Source() string { return s.source }

func (s state) HasAsset() bool { return s.asset != nil }

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
	case SetBaseZoom:
		s.baseZoom = action.Scale
	}
}

// Render draws a full-window preview and emits typed actions for gestures and
// toolbar commands. The caller schedules animation frames via State.Advance.
func Render(c *ui.Context, vm ViewModel, dispatch func(Action)) {
	if !vm.Open {
		return
	}
	emit := func(action Action) {
		if dispatch != nil {
			dispatch(action)
		}
	}
	dragging, dragX, dragY := vm.Dragging, vm.DragX, vm.DragY
	ui.Overlay(c, func() {
		root := ui.Box(c).Key("image-preview").Absolute().Top(0).Left(0).Right(0).Bottom(0).
			Background(ui.RGBA(0, 0, 0, 0.88)).Focusable().Focus().
			ContextMenu(func(m *ui.Menu) {
				if m.Item(locales.CommonCopyAction()).Chosen() {
					emit(Action{Kind: Copy, Source: vm.Source})
				}
			}).HandleInput(func(ev ui.InputEvent) bool {
			switch ev.Kind {
			case ui.InputKeyDown:
				if ev.Key == ui.KeyEscape {
					emit(Action{Kind: Close})
				}
				return true
			case ui.InputKeyUp, ui.InputText, ui.InputCompose, ui.InputCommand:
				return true
			case ui.InputScroll:
				if ev.Precise {
					emit(Action{Kind: PanBy, DX: -ev.DX, DY: -ev.DY})
				} else if ev.DY != 0 {
					emit(Action{Kind: ZoomBy, Scale: float32(math.Pow(1.12, float64(-ev.DY/60)))})
				}
				return true
			case ui.InputZoom:
				if ev.Scale > 0 {
					emit(Action{Kind: ZoomBy, Scale: ev.Scale})
				}
				return true
			case ui.InputPointerDown:
				if ev.Button == 0 && ev.Y > 60 {
					emit(Action{Kind: BeginDrag, PointerX: ev.X, PointerY: ev.Y})
					dragging, dragX, dragY = true, ev.X, ev.Y
					return true
				}
				return false
			case ui.InputPointerMove:
				if dragging && ev.Button == 0 {
					emit(Action{Kind: PanBy, DX: ev.X - dragX, DY: ev.Y - dragY})
					emit(Action{Kind: BeginDrag, PointerX: ev.X, PointerY: ev.Y})
					dragX, dragY = ev.X, ev.Y
					return true
				}
				return false
			case ui.InputPointerUp:
				if dragging {
					emit(Action{Kind: EndDrag})
					dragging = false
					return true
				}
				return false
			default:
				return false
			}
		})
		root.Children(func() {
			canvas := ui.Box(c).Key("image-preview-canvas").Absolute().Top(0).Left(0).Right(0).Bottom(0)
			vw, vh := canvas.Bounds().W, canvas.Bounds().H
			if vw <= 0 || vh <= 0 {
				vw, vh = 1200, 800
			}
			if vm.Bitmap != nil {
				bw, bh := ui.ImageSize(vm.Bitmap)
				if bw > 0 && bh > 0 {
					base := imageInitialScale(bw, bh, vw-80, vh-100)
					w, h := bw*base*vm.Zoom, bh*base*vm.Zoom
					ui.Image(c, vm.Bitmap).Key("image-preview-image").Size(w, h).Absolute().
						Left((vw-w)/2 + vm.X).Top((vh-h)/2 + vm.Y)
					if vm.BaseZoom != base {
						emit(Action{Kind: SetBaseZoom, Scale: base})
					}
				}
			} else {
				status := locales.ImageLoading()
				if !vm.Loading {
					status = markdownui.MissingImageMessage(vm.Label)
				}
				ui.Text(c, status).Key("image-preview-status").Absolute().Left(0).Right(0).Top(vh/2 - 12).
					TextColor(ui.RGB(255, 255, 255)).TextAlign(ui.Center)
			}
			ui.Row(c).Key("image-preview-toolbar").Absolute().Top(16).Left(16).Right(16).
				AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, vm.Source).Key("image-preview-path").SingleLine().Ellipsis("…").
					TextColor(ui.RGB(255, 255, 255)).Grow(1)
				button(c, "−", locales.ImageZoomOut(), "image-preview-zoom-out", func() { emit(Action{Kind: ZoomBy, Scale: 1 / 1.25}) })
				ui.Text(c, fmt.Sprintf("%.0f%%", vm.BaseZoom*vm.Zoom*100)).Key("image-preview-zoom").
					TextColor(ui.RGB(255, 255, 255)).Width(48).TextAlign(ui.Center)
				button(c, "+", locales.ImageZoomIn(), "image-preview-zoom-in", func() { emit(Action{Kind: ZoomBy, Scale: 1.25}) })
				button(c, locales.ImageFitWindow(), locales.ImageFitWindow(), "image-preview-fit", func() { emit(Action{Kind: ResetView}) })
				button(c, "×", locales.ImageClosePreview(), "image-preview-close", func() { emit(Action{Kind: Close}) })
			})
		})
	})
}

func imageInitialScale(bw, bh, vw, vh float32) float32 {
	if bw <= 0 || bh <= 0 || vw <= 0 || vh <= 0 {
		return 1
	}
	return min(float32(1), min(vw/bw, vh/bh))
}

func button(c *ui.Context, text, label, key string, click func()) {
	b := ui.Button(c, text).Key(key).Size(34, 32).Label(label)
	if key == "image-preview-fit" {
		b.Size(88, 32)
	}
	if b.Clicked() {
		click()
	}
}

func clampZoom(zoom float32) float32 { return min(maxZoom, max(minZoom, zoom)) }

func ClampZoom(zoom float32) float32 { return clampZoom(zoom) }

func InitialScale(bw, bh, vw, vh float32) float32 { return imageInitialScale(bw, bh, vw, vh) }
