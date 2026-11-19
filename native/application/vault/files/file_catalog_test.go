package files

import (
	"path/filepath"
	"testing"

	vaultfiles "muse/vault/files"
)

type fileCatalogFixture struct{ entries []vaultfiles.Entry }

func (c fileCatalogFixture) Rebuild() error          { return nil }
func (c fileCatalogFixture) Update(string)           {}
func (c fileCatalogFixture) Remove(string)           {}
func (c fileCatalogFixture) Revision() uint64        { return 1 }
func (c fileCatalogFixture) Paths() []string         { return nil }
func (c fileCatalogFixture) ImagePaths() []string    { return nil }
func (c fileCatalogFixture) FindImage(string) string { return "" }
func (c fileCatalogFixture) Children(directory string) []vaultfiles.Entry {
	var result []vaultfiles.Entry
	for _, entry := range c.entries {
		if filepath.ToSlash(filepath.Dir(filepath.FromSlash(entry.Path))) == directory || directory == "" && filepath.Dir(entry.Path) == "." {
			result = append(result, entry)
		}
	}
	return result
}
func (c fileCatalogFixture) Entry(path string) (vaultfiles.Entry, bool) {
	for _, entry := range c.entries {
		if entry.Path == path {
			return entry, true
		}
	}
	return vaultfiles.Entry{}, false
}

func TestFileUseCasesOwnCatalogPathResolution(t *testing.T) {
	root := t.TempDir()
	session := fileCatalogFixture{entries: []vaultfiles.Entry{
		{Path: "notes", Kind: vaultfiles.EntryDirectory},
		{Path: "notes/a.md", Kind: vaultfiles.EntryRegular},
	}}
	useCases := NewFileUseCases(root, nil, nil)
	catalog := NewDirectoryCatalog()
	children := useCases.CatalogChildren(session, catalog, root)
	if len(children) != 1 || children[0].Path != "notes" {
		t.Fatalf("root catalog children = %+v, want notes", children)
	}
	entry, ok := useCases.CatalogEntry(session, filepath.Join(root, "notes", "a.md"))
	if !ok || entry.Kind != vaultfiles.EntryRegular {
		t.Fatalf("catalog entry = %+v, %v", entry, ok)
	}
	directory := filepath.Join(root, "notes")
	if !useCases.IsCatalogDirectory(session, directory) || useCases.IsCatalogDirectory(session, filepath.Join(directory, "a.md")) {
		t.Fatal("catalog directory lookup did not preserve entry kinds")
	}
	if relative, ok := useCases.CatalogRelativePath(filepath.Join(directory, "a.md")); !ok || relative != "notes/a.md" {
		t.Fatalf("catalog relative path = %q, %v", relative, ok)
	}
	if _, ok := useCases.CatalogRelativePath(filepath.Join(filepath.Dir(root), "outside.md")); ok {
		t.Fatal("catalog relative path accepted an outside-vault path")
	}
	if _, ok := useCases.CatalogEntry(session, filepath.Join(filepath.Dir(root), "outside.md")); ok {
		t.Fatal("catalog resolved a path outside the active vault")
	}
}
