package nativeui

import (
	"time"

	applicationdocument "muse/application/document"
	applicationvault "muse/application/vault"
	"muse/locales"
)

func (a *App) newFileIn(directory string) {
	if a == nil {
		return
	}
	source, err := applicationdocument.NewNoteSource(time.Now())
	if err != nil {
		a.notice(locales.CommonCreateFailed(err.Error()))
		a.invalidate()
		return
	}
	path, err := a.vaultFileUseCases().CreateFile(applicationvault.CreateFileCommand{
		Directory: directory, Name: locales.DocumentUntitled(), Extension: ".md", Content: []byte(source),
	})
	if err != nil {
		a.notice(locales.CommonCreateFailed(err.Error()))
		a.invalidate()
		return
	}
	a.clearTreeCache()
	a.requestOpen(path)
}

func (a *App) newFolderIn(directory string) {
	if a == nil {
		return
	}
	if _, err := a.vaultFileUseCases().CreateFolder(applicationvault.CreateFolderCommand{
		Directory: directory, Name: locales.FilesNewFolderDefault(),
	}); err != nil {
		a.notice(locales.CommonCreateFailed(err.Error()))
	} else {
		a.clearTreeCache()
	}
	a.invalidate()
}
