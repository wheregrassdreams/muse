package files

import (
	"time"

	documentsave "muse/application/document"
	"muse/locales"
)

// CreateUntitledNote applies the application's default note type, name, and
// initial source before delegating the filesystem operation to this vault.
func (u *FileUseCases) CreateUntitledNote(directory string) (string, error) {
	source, err := documentsave.NewNoteSource(time.Now())
	if err != nil {
		return "", err
	}
	return u.CreateFile(CreateFileCommand{
		Directory: directory, Name: locales.DocumentUntitled(), Extension: ".md", Content: []byte(source),
	})
}

// CreateDefaultFolder applies the application's default folder name and
// delegates collision handling and path validation to the vault operation.
func (u *FileUseCases) CreateDefaultFolder(directory string) (string, error) {
	return u.CreateFolder(CreateFolderCommand{Directory: directory, Name: locales.FilesNewFolderDefault()})
}
