package nativeui

import (
	"path/filepath"
	"strings"

	imagepreview "muse/nativeui/workbench/features/imagepreview"
	"muse/nativeui/workbench/hosting"
)

func (a *App) openImagePreview(src string) {
	if src == "" {
		return
	}
	a.closeImagePreview()
	content := imagepreview.NewContent(src, a.imagePreviewLabel(src), a.imageCacheStore, a.onMain, a.invalidate, func(source string) {
		if a.noteEditor() != nil {
			a.noteEditor().ClipboardCommands().CopyImageObject(source)
		}
	}, func() { a.imagePreviewHandle = window.Handle{} })
	content.SetLocalFiles(a.imageLocalFiles)
	handle, err := a.windows.Open(content, window.Placement{Host: "main", Layer: window.ModalOverlay})
	if err != nil {
		a.notice(err.Error())
		return
	}
	a.imagePreviewHandle = handle
	a.invalidate()
}

func (a *App) imagePreviewLabel(src string) string {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "data:") {
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

func (a *App) closeImagePreview() {
	if a.windows != nil && !a.imagePreviewHandle.IsZero() {
		_ = a.windows.Close(a.imagePreviewHandle)
	}
	a.imagePreviewHandle = window.Handle{}
	a.invalidate()
}

func (a *App) imagePreviewOpen() bool {
	return a.windows != nil && a.windows.Contains(a.imagePreviewHandle)
}
