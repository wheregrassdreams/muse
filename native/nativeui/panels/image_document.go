package nativeui

import (
	"fmt"
	"math"
	"muse/locales"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"muse/mdoc"
)

// imageDocumentState is a read-only document shown in the editor canvas. It
// deliberately leaves the Markdown editor and its recovery state untouched.
type imageDocumentState struct {
	path         string
	bmp          *ui.Bitmap
	open         bool
	zoom         float32
	x, y         float32
	dragging     bool
	dragX, dragY float32
}

func (a *App) openImageDocument(path string, record bool) {
	if path == "" {
		return
	}
	bmp := mdoc.LoadImage(path, "", nil)
	a.imageDocument = imageDocumentState{path: path, bmp: bmp, open: true, zoom: 1}
	a.propertiesOpen = false
	a.propertyEditKey, a.propertyEditLocation = "", ""
	a.selectedPath = path
	a.propertiesOpen = false
	if record {
		a.pushNav(path)
	}
	a.updateTitle()
	a.invalidate()
	if bmp == nil {
		a.notice(locales.ImageDecodeError(filepath.Base(path)))
	}
}

func (a *App) imageDocumentView(c *ui.Context) {
	doc := &a.imageDocument
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.editorHeader(c)
		ui.Box(c).Height(a.headerH(c)).Shrink(0).Key("image-document-header-spacer")
		if sources := a.documentBacklinkSources(); len(sources) > 0 {
			ui.Row(c).Height(36).Shrink(0).Padding(0, 12).AlignItems(ui.Center).Children(func() {
				b := ui.ButtonBase(c).Height(30).Padding(0, 7).Radius(5).Label(locales.PropertyViewBacklinks())
				if b.Hovered() {
					b.Background(t.SurfaceHover)
				}
				b.OnClick(a.openLinks)
				b.Children(func() {
					ui.Text(c, locales.PropertyBacklinkCount(len(sources))).FontSize(13).TextColor(propertyQuietColor(t))
				})
			})
		}
		canvas := ui.Box(c).Grow(1).FillWidth().MinHeight(0).Key("image-document-canvas").Clip().HandleInput(func(ev ui.InputEvent) bool {
			switch ev.Kind {
			case ui.InputScroll:
				if ev.Precise {
					doc.x -= ev.DX
					doc.y -= ev.DY
					a.invalidate()
					return true
				}
				if ev.DY != 0 {
					doc.zoom = clampImageZoom(doc.zoom * float32(math.Pow(1.12, float64(-ev.DY/60))))
					a.invalidate()
					return true
				}
			case ui.InputZoom:
				if ev.Scale > 0 {
					doc.zoom = clampImageZoom(doc.zoom * ev.Scale)
					a.invalidate()
					return true
				}
			case ui.InputPointerDown:
				if ev.Button == 0 {
					doc.dragging, doc.dragX, doc.dragY = true, ev.X, ev.Y
					return true
				}
			case ui.InputPointerMove:
				if doc.dragging && ev.Button == 0 {
					doc.x += ev.X - doc.dragX
					doc.y += ev.Y - doc.dragY
					doc.dragX, doc.dragY = ev.X, ev.Y
					a.invalidate()
					return true
				}
			case ui.InputPointerUp:
				wasDragging := doc.dragging
				doc.dragging = false
				return wasDragging
			}
			return false
		}).ContextMenu(func(m *ui.Menu) {
			if m.Item(locales.CommonCopyAction()).Chosen() {
				a.ed.CopyImageObject(doc.path)
			}
		})
		vw, vh := canvas.Bounds().W, canvas.Bounds().H
		canvas.Children(func() {
			if doc.bmp == nil {
				ui.Text(c, locales.ImageDecodeFailed()).Key("image-document-error").TextColor(t.TextMuted)
				return
			}
			bw, bh := ui.ImageSize(doc.bmp)
			if bw <= 0 || bh <= 0 {
				return
			}
			if vw <= 0 || vh <= 0 {
				vw, vh = 1200, 800
			}
			fit := imageInitialScale(bw, bh, vw-64, vh-64)
			fit *= doc.zoom
			ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Image(c, doc.bmp).Key("image-document-image").Size(bw*fit, bh*fit).Translate(doc.x, doc.y)
			})
		})
		if doc.bmp != nil {
			bw, bh := ui.ImageSize(doc.bmp)
			percent := imageInitialScale(bw, bh, vw-64, vh-64) * doc.zoom
			toolbarTop := a.headerH(c) + 12
			if len(a.documentBacklinkSources()) > 0 {
				toolbarTop += 36
			}
			ui.Row(c).Key("image-document-toolbar").Absolute().Top(toolbarTop).Right(16).Gap(6).AlignItems(ui.Center).Children(func() {
				imagePreviewButton(c, "−", locales.ImageZoomOut(), "image-document-zoom-out", func() { doc.zoom = clampImageZoom(doc.zoom / 1.25); a.invalidate() })
				ui.Text(c, fmt.Sprintf("%.0f%%", percent*100)).Key("image-document-zoom").Width(48).TextAlign(ui.Center).TextColor(t.Text)
				imagePreviewButton(c, "+", locales.ImageZoomIn(), "image-document-zoom-in", func() { doc.zoom = clampImageZoom(doc.zoom * 1.25); a.invalidate() })
				imagePreviewButton(c, locales.ImageFitWindow(), locales.ImageFitWindow(), "image-document-fit", func() { doc.zoom = 1; doc.x = 0; doc.y = 0; a.invalidate() })
				imagePreviewButton(c, "⛶", locales.ImagePreviewTitle(), "image-document-fullscreen", func() { a.openImagePreview(doc.path) })
			})
		}
	})
}
