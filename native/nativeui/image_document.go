package nativeui

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"muse/locales"
	"muse/nativeui/panels/imagedocument"
)

func (a *App) openImageDocument(path string, record bool) {
	if path == "" {
		return
	}
	a.imageDocument.Open(path, a.imageCacheStore)
	a.propertiesOpen = false
	a.propertyEditKey, a.propertyEditLocation = "", ""
	a.selectedPath = path
	if record {
		a.pushNav(path)
	}
	a.updateTitle()
	a.invalidate()
	if !a.imageDocument.HasImage() {
		a.notice(locales.ImageDecodeError(filepath.Base(path)))
	}
}

func (a *App) imageDocumentView(c *ui.Context) {
	backlinks := a.documentBacklinkSources()
	path := a.imageDocument.Path()
	imagedocument.Render(c, a.imageDocument.ViewModel(len(backlinks)), imagedocument.Options{
		Header:       func() { a.editorHeader(c) },
		HeaderHeight: a.headerH(c),
		OnBacklinks:  a.openLinks,
		OnCopy: func() {
			if a.ed != nil {
				a.ed.CopyImageObject(path)
			}
		},
		OnFullscreen: func() { a.openImagePreview(path) },
	}, func(action imagedocument.Action) {
		a.imageDocument.Apply(action)
		a.invalidate()
	})
}
