package nativeui

import (
	"path/filepath"

	applicationvault "muse/application/vault"
	"muse/locales"
	vaultfiles "muse/vault/files"
)

// vaultFileUseCases binds filesystem mutations to the current workspace and
// adapts the vault catalog's directory facts into the application capability.
func (a *App) vaultFileUseCases() *applicationvault.FileUseCases {
	if a == nil {
		return applicationvault.NewFileUseCases("", nil, nil)
	}
	return applicationvault.NewFileUseCases(a.root, a.fileOperations, a.isVaultDirectory)
}

func (a *App) isVaultDirectory(path string) bool {
	if a == nil || path == "" {
		return false
	}
	if a.catalogIsDirectory(path) {
		return true
	}
	kind, err := a.pathKind(path)
	return err == nil && kind == vaultfiles.EntryDirectory
}

func (a *App) applyVaultDrop(paths []string, destination string, intent applicationvault.DropIntent) {
	mutation, err := a.vaultFileUseCases().DropFiles(applicationvault.DropFilesCommand{
		Sources: paths, Destination: destination, Intent: intent,
	})
	if mutation.Kind == applicationvault.FileMutationMove {
		if len(mutation.Moves) > 0 {
			a.remapTreePaths(mutation.Moves)
			a.clearTreeCache()
			a.notice(locales.FilesMovedToFolder(filepath.Base(destination)))
			a.invalidate()
		}
		if err != nil {
			a.notice(locales.CommonMoveFailed(err.Error()))
		}
		return
	}
	if err != nil {
		a.notice(locales.CommonImportFailed(err.Error()))
	}
	if len(paths) > 0 {
		a.clearTreeCache()
		a.invalidate()
	}
}
