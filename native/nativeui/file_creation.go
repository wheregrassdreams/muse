package nativeui

import (
	"time"

	applicationdocument "muse/application/document"
	"muse/locales"
)

func (a *App) newFileIn(directory string) {
	if a == nil || a.fileOperations == nil {
		if a != nil {
			a.notice(locales.CommonCreateFailed("vault file operations are not configured"))
			a.invalidate()
		}
		return
	}
	source, err := applicationdocument.NewNoteSource(time.Now())
	if err != nil {
		a.notice(locales.CommonCreateFailed(err.Error()))
		a.invalidate()
		return
	}
	path, err := a.fileOperations.CreateFile(a.root, directory, locales.DocumentUntitled(), ".md", []byte(source))
	if err != nil {
		a.notice(locales.CommonCreateFailed(err.Error()))
		a.invalidate()
		return
	}
	a.clearTreeCache()
	a.requestOpen(path)
}

func (a *App) newFolderIn(directory string) {
	if a == nil || a.fileOperations == nil {
		if a != nil {
			a.notice(locales.CommonCreateFailed("vault file operations are not configured"))
			a.invalidate()
		}
		return
	}
	if _, err := a.fileOperations.CreateFolder(a.root, directory, locales.FilesNewFolderDefault()); err != nil {
		a.notice(locales.CommonCreateFailed(err.Error()))
	} else {
		a.clearTreeCache()
	}
	a.invalidate()
}
