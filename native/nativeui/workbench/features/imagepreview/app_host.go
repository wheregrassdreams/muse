// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (a *App) imagePreviewController() *imagepreviewui.Controller {
	if a == nil {
		return nil
	}
	if a.imagePreview == nil {
		a.imagePreview = imagepreviewui.NewController(imagepreviewui.ControllerOptions{
			Windows: a.windows, Placement: window.Placement{Host: "main", Layer: window.ModalOverlay},
			ImageCache: a.imageCacheStore, LocalFiles: a.imageLocalFiles,
			Label: a.imagePreviewLabel,
			CopyImage: func(source string) {
				if editor := a.noteEditor(); editor != nil {
					editor.ClipboardCommands().CopyImageObject(source)
				}
			},
			OnMain: a.onMain, Invalidate: a.invalidate,
			OnError: func(err error) { a.notice(err.Error()) },
		})
	}
	return a.imagePreview
}

func (a *App) openImagePreview(source string) {
	if controller := a.imagePreviewController(); controller != nil {
		_ = controller.Open(source)
	}
}

func (a *App) closeImagePreview() {
	if a != nil && a.imagePreview != nil {
		a.imagePreview.Close()
	}
}

func (a *App) imagePreviewOpen() bool {
	return a != nil && a.imagePreview != nil && a.imagePreview.IsOpen()
}

func (a *App) imagePreviewLabel(source string) string {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "data:") {
		return source
	}
	base := a.docDir()
	if base == "" {
		return filepath.ToSlash(source)
	}
	if absolute, err := filepath.Abs(source); err == nil {
		source = absolute
	}
	if relative, err := filepath.Rel(base, source); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(source)
}
