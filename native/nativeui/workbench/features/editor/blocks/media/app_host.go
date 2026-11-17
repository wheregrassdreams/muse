// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

// saveImage stores an image pasted into the document in an assets folder
// next to it, and returns the path the document links it by. An unsaved
// document keeps it in the temporary folder, by its absolute path.
func (a *App) saveImage(data []byte, ext string) (string, error) {
	if ext == "" {
		ext = ".png"
	}
	dir := a.docDir()
	rel := dir != ""
	base := "image-" + time.Now().Format("20060102-150405")
	if rel {
		dir = filepath.Join(dir, "assets")
		root := a.root
		if root == "" {
			root = a.docDir()
		}
		files := applicationvault.NewFileUseCases(root, a.fileOperations, a.isVaultDirectory)
		path, err := files.CreateAttachment(applicationvault.CreateAttachmentCommand{
			Directory: dir, Name: base, Extension: ext, Content: data,
		})
		if err != nil {
			return "", err
		}
		return "assets/" + filepath.Base(path), nil
	}
	if a.temporaryImageStore == nil {
		return "", errors.New("temporary image store is not configured")
	}
	path, err := a.temporaryImageStore.SaveTemporaryImage(data, ext)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(path), nil
}
