package nativeui

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"

	doctypes "muse/document/types"
	"muse/locales"
	"muse/markdown/images"
	imagedocumentui "muse/nativeui/workbench/features/imagedocument"
	window "muse/nativeui/workbench/hosting"
)

func newImageDocumentController(a *App) *imagedocumentui.Controller {
	return imagedocumentui.NewController(imagedocumentui.Ports{
		Host: a.windows,
		Placement: func() window.Placement {
			return window.Placement{Host: "main", Pane: a.focusedPane(), Layer: window.PaneContent}
		},
		Cache: func() images.CacheStore { return a.imageCacheStore },
		Files: func() images.LocalFiles { return a.imageLocalFiles },
		Surface: imagedocumentui.SurfaceOptions{
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
		},
		OnSelectedPath: func(path string) {
			if a.propertiesPanel != nil {
				a.propertiesPanel.Close()
				a.propertiesPanel.CancelEdit()
			}
			a.selectedPath = path
		},
		OnRecordHistory: a.pushNav,
		OnNotice: func(err error) {
			if err != nil {
				a.notice(err.Error())
			}
		},
		OnDecodeError: func(path string) { a.notice(locales.ImageDecodeError(filepath.Base(path))) },
		OnChanged: func() {
			a.updateTitle()
			a.invalidate()
		},
	})
}

func (a *App) openImageDocument(path string, record bool) {
	if path == "" || a.imageDocument == nil {
		return
	}
	_ = a.imageDocument.Open(path, record)
}

func (a *App) closeImageDocument() {
	if a.imageDocument != nil {
		a.imageDocument.Close()
	}
}

func (a *App) imageDocumentOpen() bool {
	return a.imageDocument != nil && a.imageDocument.IsOpen()
}

func (a *App) imageDocumentPath() string {
	if !a.imageDocumentOpen() {
		return ""
	}
	if path, ok := a.navigationHistory.Current(); ok && doctypes.IsImagePath(path) {
		return path
	}
	if path := a.imageDocument.Path(); path != "" {
		return path
	}
	return a.selectedPath
}
