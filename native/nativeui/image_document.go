package nativeui

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"muse/locales"
	"muse/nativeui/components/doctypes"
	"muse/nativeui/surfaces/imagedocument"
	"muse/nativeui/window"
)

func (a *App) openImageDocument(path string, record bool) {
	if path == "" {
		return
	}
	a.closeImageDocument()
	content, hasImage := imagedocument.NewContent(path, a.imageCacheStore, imagedocument.SurfaceOptions{
		Header:        func(c *ui.Context) { a.editorHeader(c) },
		HeaderHeight:  func(c *ui.Context) float32 { return a.headerH(c) },
		BacklinkCount: func() int { return len(a.documentBacklinkSources()) },
		OnBacklinks:   a.openLinks,
		OnFullscreen:  a.openImagePreview,
		OnCopy: func(path string) {
			if a.ed != nil {
				a.ed.CopyImageObject(path)
			}
		},
		OnClose: func() { a.imageDocumentHandle = window.Handle{} },
	})
	handle, err := a.windows.Open(content, window.Placement{Host: "main", Pane: "editor", Layer: window.PaneContent})
	if err != nil {
		a.notice(err.Error())
		return
	}
	a.imageDocumentHandle = handle
	a.propertiesOpen = false
	a.propertyEditKey, a.propertyEditLocation = "", ""
	a.selectedPath = path
	if record {
		a.pushNav(path)
	}
	a.updateTitle()
	a.invalidate()
	if !hasImage {
		a.notice(locales.ImageDecodeError(filepath.Base(path)))
	}
}

func (a *App) closeImageDocument() {
	if a.windows != nil && !a.imageDocumentHandle.IsZero() {
		_ = a.windows.Close(a.imageDocumentHandle)
	}
	a.imageDocumentHandle = window.Handle{}
}

func (a *App) imageDocumentOpen() bool {
	return a.windows != nil && a.windows.Contains(a.imageDocumentHandle)
}

func (a *App) imageDocumentPath() string {
	if !a.imageDocumentOpen() {
		return ""
	}
	if a.navAt >= 0 && a.navAt < len(a.navHist) && doctypes.IsImagePath(a.navHist[a.navAt]) {
		return a.navHist[a.navAt]
	}
	return a.selectedPath
}
