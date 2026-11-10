package nativeui

import (
	"fmt"
	"math"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"muse/mdoc"
)

// imageDocumentState is a read-only document shown in the editor canvas. It
// deliberately leaves the Markdown editor and its recovery state untouched.
type imageDocumentState struct {
	path string
	bmp  *ui.Bitmap
	open bool
}

func (a *App) openImageDocument(path string, record bool) {
	if path == "" {
		return
	}
	bmp := mdoc.LoadImage(path, "", nil)
	a.imageDocument = imageDocumentState{path: path, bmp: bmp, open: true}
	a.propertiesOpen = false
	a.propertyTitleEditing = false
	a.propertyEditKey, a.propertyEditLocation = "", ""
	a.selectedPath = path
	a.propertiesOpen = false
	if record {
		a.pushNav(path)
	}
	a.updateTitle()
	a.invalidate()
	if bmp == nil {
		a.notice(fmt.Sprintf("无法解码图片：%s", filepath.Base(path)))
	}
}

func (a *App) imageDocumentView(c *ui.Context) {
	doc := &a.imageDocument
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.editorHeader(c)
		canvas := ui.Box(c).Grow(1).FillWidth().MinHeight(0).Key("image-document-canvas").Clip()
		vw, vh := canvas.Bounds().W, canvas.Bounds().H
		canvas.Children(func() {
			if doc.bmp == nil {
				ui.Text(c, "无法解码此图片格式").Key("image-document-error").TextColor(t.TextMuted)
				return
			}
			bw, bh := ui.ImageSize(doc.bmp)
			if bw <= 0 || bh <= 0 {
				return
			}
			if vw <= 0 || vh <= 0 {
				vw, vh = 1200, 800
			}
			fit := min((vw-64)/bw, (vh-64)/bh)
			if fit <= 0 || math.IsNaN(float64(fit)) {
				fit = 1
			}
			ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Image(c, doc.bmp).Key("image-document-image").Size(bw*fit, bh*fit)
			})
		})
	})
}
