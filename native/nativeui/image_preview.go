package nativeui

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"muse/mdoc"
)

const (
	imagePreviewMinZoom = float32(0.1)
	imagePreviewMaxZoom = float32(8)
)

type imagePreviewState struct {
	src          string
	bmp          *ui.Bitmap
	loading      bool
	zoom         float32
	x, y         float32
	dragging     bool
	dragX, dragY float32
	open         bool
}

func (a *App) openImagePreview(src string) {
	if src == "" {
		return
	}
	a.imagePreview = imagePreviewState{src: src, open: true, loading: true, zoom: 1}
	a.imagePreview.bmp = mdoc.LoadImage(src, "", func() {
		a.onMain(func() {
			if !a.imagePreview.open || a.imagePreview.src != src {
				return
			}
			a.imagePreview.bmp = mdoc.LoadImage(src, "", nil)
			a.imagePreview.loading = false
			a.invalidate()
		})
	})
	if a.imagePreview.bmp != nil {
		a.imagePreview.loading = false
	} else if !isRemoteImageSource(src) {
		a.imagePreview.loading = false
		a.notice(fmt.Sprintf("无法解码图片：%s", src))
	}
	a.invalidate()
}

func isRemoteImageSource(src string) bool {
	return len(src) >= 7 && (src[:7] == "http://" || len(src) >= 8 && src[:8] == "https://")
}

func (a *App) closeImagePreview() {
	a.imagePreview.open = false
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
	ui.Overlay(c, func() {
		// Keep the overlay to one full-window root. Overlay children otherwise
		// participate in flow layout, which can leave the lower window exposed.
		root := ui.Box(c).Key("image-preview").Absolute().Top(0).Left(0).Right(0).Bottom(0).
			Background(ui.RGBA(0, 0, 0, 0.88)).Focusable().Focus().
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
					fit := min((vw-80)/bw, (vh-100)/bh)
					if fit <= 0 {
						fit = 1
					}
					w, h := bw*fit*p.zoom, bh*fit*p.zoom
					ui.Image(c, p.bmp).Key("image-preview-image").Size(w, h).Absolute().
						Left((vw-w)/2 + p.x).Top((vh-h)/2 + p.y)
				}
			} else {
				status := "正在加载图片…"
				if !p.loading {
					status = "无法解码此图片格式"
				}
				ui.Text(c, status).Key("image-preview-status").Absolute().Left(0).Right(0).Top(vh/2 - 12).
					TextColor(ui.RGB(255, 255, 255)).TextAlign(ui.Center)
			}
			ui.Row(c).Key("image-preview-toolbar").Absolute().Top(16).Left(16).Right(16).
				AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, p.src).Key("image-preview-path").SingleLine().Ellipsis("…").
					TextColor(ui.RGB(255, 255, 255)).Grow(1)
				imagePreviewButton(c, "−", "缩小图片", "image-preview-zoom-out", func() { p.zoom = clampImageZoom(p.zoom / 1.25) })
				ui.Text(c, fmt.Sprintf("%.0f%%", p.zoom*100)).Key("image-preview-zoom").
					TextColor(ui.RGB(255, 255, 255)).Width(48).TextAlign(ui.Center)
				imagePreviewButton(c, "+", "放大图片", "image-preview-zoom-in", func() { p.zoom = clampImageZoom(p.zoom * 1.25) })
				imagePreviewButton(c, "适合窗口", "适合窗口", "image-preview-fit", func() { p.zoom, p.x, p.y = 1, 0, 0 })
				imagePreviewButton(c, "×", "关闭图片预览", "image-preview-close", a.closeImagePreview)
			})
		})
	})
}

func imagePreviewButton(c *ui.Context, text, label, key string, click func()) {
	b := ui.Button(c, text).Key(key).Size(34, 32).Label(label)
	if text == "适合窗口" {
		b.Size(88, 32)
	}
	if b.Clicked() {
		click()
	}
}

func clampImageZoom(zoom float32) float32 {
	return min(imagePreviewMaxZoom, max(imagePreviewMinZoom, zoom))
}
