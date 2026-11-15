package nativeui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"muse/locales"
	"muse/mdoc"
)

const (
	imagePreviewMinZoom = float32(0.1)
	imagePreviewMaxZoom = float32(8)
)

type imagePreviewState struct {
	src          string
	label        string
	asset        *mdoc.ImageAsset
	bmp          *ui.Bitmap
	loading      bool
	zoom         float32
	baseZoom     float32
	x, y         float32
	dragging     bool
	dragX, dragY float32
	open         bool
}

func (a *App) openImagePreview(src string) {
	if src == "" {
		return
	}
	a.imagePreview = imagePreviewState{src: src, label: a.imagePreviewLabel(src), open: true, loading: true, zoom: 1}
	a.imagePreview.asset = mdoc.LoadImageAsset(src, "", func() {
		a.onMain(func() {
			if !a.imagePreview.open || a.imagePreview.src != src {
				return
			}
			if a.imagePreview.asset != nil {
				a.imagePreview.asset.Advance(time.Now())
				a.imagePreview.bmp = a.imagePreview.asset.Bitmap()
			}
			a.imagePreview.loading = false
			a.invalidate()
		})
	})
	a.imagePreview.asset.Advance(time.Now())
	a.imagePreview.bmp = a.imagePreview.asset.Bitmap()
	if a.imagePreview.bmp != nil {
		a.imagePreview.loading = false
	} else if !isRemoteImageSource(src) && !strings.HasPrefix(src, "data:") {
		if _, err := os.Stat(src); err != nil {
			a.imagePreview.loading = false
		}
	}
	if a.imagePreview.asset == nil {
		a.imagePreview.loading = false
	}
	a.invalidate()
}

func (a *App) imagePreviewLabel(src string) string {
	if isRemoteImageSource(src) || strings.HasPrefix(src, "data:") {
		return src
	}
	base := a.docDir()
	if base == "" {
		return filepath.ToSlash(src)
	}
	if abs, err := filepath.Abs(src); err == nil {
		src = abs
	}
	if rel, err := filepath.Rel(base, src); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(src)
}

func isRemoteImageSource(src string) bool {
	return len(src) >= 7 && (src[:7] == "http://" || len(src) >= 8 && src[:8] == "https://")
}

func (a *App) closeImagePreview() {
	a.imagePreview.open = false
	a.imagePreview.asset = nil
	a.imagePreview.bmp = nil
	a.imagePreview.loading = false
	a.restoreEditorFocus = true
	a.invalidate()
}

func (a *App) imagePreviewDialog(c *ui.Context) {
	p := &a.imagePreview
	if !p.open {
		return
	}
	if p.asset != nil {
		if next, _ := p.asset.Advance(c.Now()); next > 0 {
			c.After(next)
		}
		p.bmp = p.asset.Bitmap()
	}
	ui.Overlay(c, func() {
		// Keep the overlay to one full-window root. Overlay children otherwise
		// participate in flow layout, which can leave the lower window exposed.
		root := ui.Box(c).Key("image-preview").Absolute().Top(0).Left(0).Right(0).Bottom(0).
			Background(ui.RGBA(0, 0, 0, 0.88)).Focusable().Focus().
			ContextMenu(func(m *ui.Menu) {
				if m.Item(locales.CommonCopyAction()).Chosen() {
					a.ed.CopyImageObject(p.src)
				}
			}).
			HandleInput(func(ev ui.InputEvent) bool {
				switch ev.Kind {
				case ui.InputKeyDown:
					if ev.Key == ui.KeyEscape {
						a.closeImagePreview()
					}
					return true
				case ui.InputKeyUp, ui.InputText, ui.InputCompose, ui.InputCommand:
					return true
				case ui.InputScroll:
					if ev.Precise {
						p.x -= ev.DX
						p.y -= ev.DY
						a.invalidate()
						return true
					}
					if ev.DY != 0 {
						p.zoom = clampImageZoom(p.zoom * float32(math.Pow(1.12, float64(-ev.DY/60))))
						a.invalidate()
					}
					return true
				case ui.InputZoom:
					if ev.Scale > 0 {
						p.zoom = clampImageZoom(p.zoom * ev.Scale)
						a.invalidate()
					}
					return true
				case ui.InputPointerDown:
					if ev.Button == 0 && ev.Y > 60 {
						p.dragging, p.dragX, p.dragY = true, ev.X, ev.Y
						return true
					}
					return false
				case ui.InputPointerMove:
					if p.dragging && ev.Button == 0 {
						p.x += ev.X - p.dragX
						p.y += ev.Y - p.dragY
						p.dragX, p.dragY = ev.X, ev.Y
						a.invalidate()
						return true
					}
					return false
				case ui.InputPointerUp:
					wasDragging := p.dragging
					p.dragging = false
					return wasDragging
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
			if p.bmp != nil {
				bw, bh := ui.ImageSize(p.bmp)
				if bw > 0 && bh > 0 {
					p.baseZoom = imageInitialScale(bw, bh, vw-80, vh-100)
					w, h := bw*p.baseZoom*p.zoom, bh*p.baseZoom*p.zoom
					ui.Image(c, p.bmp).Key("image-preview-image").Size(w, h).Absolute().
						Left((vw-w)/2 + p.x).Top((vh-h)/2 + p.y)
				}
			} else {
				status := locales.ImageLoading()
				if !p.loading {
					status = mdoc.MissingImageMessage(p.label)
				}
				ui.Text(c, status).Key("image-preview-status").Absolute().Left(0).Right(0).Top(vh/2 - 12).
					TextColor(ui.RGB(255, 255, 255)).TextAlign(ui.Center)
			}
			ui.Row(c).Key("image-preview-toolbar").Absolute().Top(16).Left(16).Right(16).
				AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, p.src).Key("image-preview-path").SingleLine().Ellipsis("…").
					TextColor(ui.RGB(255, 255, 255)).Grow(1)
				imagePreviewButton(c, "−", locales.ImageZoomOut(), "image-preview-zoom-out", func() { p.zoom = clampImageZoom(p.zoom / 1.25) })
				ui.Text(c, fmt.Sprintf("%.0f%%", p.baseZoom*p.zoom*100)).Key("image-preview-zoom").
					TextColor(ui.RGB(255, 255, 255)).Width(48).TextAlign(ui.Center)
				imagePreviewButton(c, "+", locales.ImageZoomIn(), "image-preview-zoom-in", func() { p.zoom = clampImageZoom(p.zoom * 1.25) })
				imagePreviewButton(c, locales.ImageFitWindow(), locales.ImageFitWindow(), "image-preview-fit", func() { p.zoom, p.x, p.y = 1, 0, 0 })
				imagePreviewButton(c, "×", locales.ImageClosePreview(), "image-preview-close", a.closeImagePreview)
			})
		})
	})
}

// imageInitialScale keeps small images at their native size and only scales
// down images that would exceed the available viewport.
func imageInitialScale(bw, bh, vw, vh float32) float32 {
	if bw <= 0 || bh <= 0 || vw <= 0 || vh <= 0 {
		return 1
	}
	return min(float32(1), min(vw/bw, vh/bh))
}

func imagePreviewButton(c *ui.Context, text, label, key string, click func()) {
	b := ui.Button(c, text).Key(key).Size(34, 32).Label(label)
	if key == "image-preview-fit" {
		b.Size(88, 32)
	}
	if b.Clicked() {
		click()
	}
}

func clampImageZoom(zoom float32) float32 {
	return min(imagePreviewMaxZoom, max(imagePreviewMinZoom, zoom))
}
