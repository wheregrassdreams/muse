package nativeui

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"

	doctypes "muse/document/types"
	"muse/locales"
	imagedocument "muse/nativeui/workbench/features/imagedocument"
	"muse/nativeui/workbench/hosting"
)

func (a *App) openImageDocument(path string, record bool) {
	if path == "" {
		return
	}
	a.closeImageDocument()
	content, hasImage := imagedocument.NewContent(path, a.imageCacheStore, imagedocument.SurfaceOptions{
		LocalFiles:    a.imageLocalFiles,
		Header:        func(c *ui.Context) { a.editorHeader(c) },
		HeaderHeight:  func(c *ui.Context) float32 { return a.headerH(c) },
		BacklinkCount: func() int { return len(a.documentBacklinkSources()) },
		OnBacklinks:   a.openLinks,
		OnFullscreen:  a.openImagePreview,
		OnCopy: func(path string) {
			if a.noteEditor() != nil {
				a.noteEditor().ClipboardCommands().CopyImageObject(path)
			}
		},
		OnClose: func() { a.imageDocumentHandle = window.Handle{} },
	})
	handle, err := a.windows.Open(content, window.Placement{Host: "main", Pane: a.focusedPane(), Layer: window.PaneContent})
	if err != nil {
		a.notice(err.Error())
		return
	}
	a.imageDocumentHandle = handle
	a.propertiesPanel.Close()
	a.propertiesPanel.CancelEdit()
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
	if path, ok := a.navigationHistory.Current(); ok && doctypes.IsImagePath(path) {
		return path
	}
	return a.selectedPath
}
