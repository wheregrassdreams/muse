package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	applicationvault "muse/application/vault"
	vaultfilesapp "muse/application/vault/files"
	doctypes "muse/document/types"
	explorer "muse/nativeui/workbench/features/explorer"
	vaultfiles "muse/vault/files"
)

func vaultRowsForIntegration(a *App, root string) []explorer.Row {
	files := vaultfilesapp.NewFileUseCases(root, a.fileOperations, nil)
	return a.fileTreeState.Rows(root, explorer.CatalogPort{
		Pending:  a.vault == nil || !a.vaultSnapshot.CatalogReady || filepath.Clean(a.vaultSnapshot.Root) != filepath.Clean(root),
		Revision: a.vaultSnapshot.FilesRevision,
		Children: func(directory string) []explorer.CatalogEntry {
			if a.vault == nil || !a.vaultSnapshot.CatalogReady || filepath.Clean(a.vaultSnapshot.Root) != filepath.Clean(root) {
				return nil
			}
			entries := files.CatalogChildren(a.vault, a.directoryCatalog, directory, vaultfilesapp.CatalogVersion{
				Ready: a.vaultSnapshot.CatalogReady, Revision: a.vaultSnapshot.FilesRevision,
			})
			children := make([]explorer.CatalogEntry, 0, len(entries))
			for _, entry := range entries {
				switch entry.Kind {
				case vaultfiles.EntryDirectory:
					children = append(children, explorer.CatalogEntry{Path: entry.Path, Kind: explorer.CatalogDirectory})
				case vaultfiles.EntryRegular:
					children = append(children, explorer.CatalogEntry{Path: entry.Path, Kind: explorer.CatalogFile})
				}
			}
			return children
		},
		VisibleFile: doctypes.IsVisibleWorkspacePath,
	})
}

func TestCatalogVersionRefreshesTreeCachedBeforeVaultStartup(t *testing.T) {
	root := t.TempDir()
	note := filepath.Join(root, "first.md")
	if err := os.WriteFile(note, []byte("# first"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := newFileTreeTestApp(t, root)
	session := a.vault
	a.vault = nil
	a.vaultSnapshot = applicationvault.Snapshot{}

	a.setWorkspace(root)
	if got := vaultRowsForIntegration(a, root); len(got) != 0 {
		t.Fatalf("rows before vault startup = %v, want empty", got)
	}

	a.vault = session
	a.vaultSnapshot = session.Snapshot()
	got := vaultRowsForIntegration(a, root)
	if len(got) != 1 || got[0].Path != note {
		t.Fatalf("rows after SessionReady = %+v, want [%s]", got, note)
	}
}

func TestCloseVaultSessionReleasesWikiSuggestionData(t *testing.T) {
	root := t.TempDir()
	a := newFileTreeTestApp(t, root)
	session := a.vault

	a.closeVaultSessionNow()

	if a.vault != nil {
		t.Fatal("closeVaultSessionNow left a vault session attached")
	}
	if session.Snapshot().Ready || session.WikiSuggestions() != nil {
		t.Fatal("closeVaultSessionNow retained data for the closed session")
	}
}

func TestWorkspaceSwitchReleasesPreviousVaultSession(t *testing.T) {
	oldRoot, newRoot := t.TempDir(), t.TempDir()
	a := newFileTreeTestApp(t, oldRoot)
	session := a.vault

	a.setWorkspace(newRoot)

	if a.vault == session {
		t.Fatal("workspace switch retained the previous vault session")
	}
}
