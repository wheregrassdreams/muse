package nativeui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"muse/nativeui/panels/imagepreview"
)

func (a *App) openImagePreview(src string) {
	if src == "" {
		return
	}
	a.imagePreview.Open(src, a.imagePreviewLabel(src), a.imageCacheStore, a.onMain, a.invalidate)
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
	a.imagePreview.Close()
	a.restoreEditorFocus = true
	a.invalidate()
}

func (a *App) imagePreviewDialog(c *ui.Context) {
	if next := a.imagePreview.Advance(c.Now()); next > 0 {
		c.After(next)
	}
	imagepreview.Render(c, a.imagePreview.ViewModel(), func(action imagepreview.Action) {
		switch action.Kind {
		case imagepreview.Close:
			a.closeImagePreview()
		case imagepreview.Copy:
			if a.ed != nil {
				a.ed.CopyImageObject(action.Source)
			}
		default:
			a.imagePreview.Apply(action)
			a.invalidate()
		}
	})
}
