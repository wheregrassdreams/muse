package naviconstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorePersistsVaultIconsWithPrivateAtomicFile(t *testing.T) {
	cacheRoot := t.TempDir()
	store, err := New(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	vault := t.TempDir()
	if err := store.SaveVault(vault, map[string]string{"Projects/Atlas.md": "bookmark"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadVault(vault)
	if err != nil || got["Projects/Atlas.md"] != "bookmark" {
		t.Fatalf("loaded preferences = %#v, %v", got, err)
	}
	entries, err := os.ReadDir(filepath.Join(cacheRoot, "navicons"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache entries = %v, %v", entries, err)
	}
	info, err := entries[0].Info()
	if err != nil || info.Mode().Perm() != fileMode {
		t.Fatalf("cache mode = %v, %v", info.Mode(), err)
	}
}

func TestStoreSharesCanonicalVaultIdentityAndSeparatesVaults(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actual := t.TempDir()
	alias := filepath.Join(t.TempDir(), "vault-link")
	if err := os.Symlink(actual, alias); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if err := store.SaveVault(alias, map[string]string{"note.md": "pin"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadVault(actual)
	if err != nil || got["note.md"] != "pin" {
		t.Fatalf("canonical vault lookup = %#v, %v", got, err)
	}
	other := t.TempDir()
	if got, err := store.LoadVault(other); err != nil || len(got) != 0 {
		t.Fatalf("other vault inherited icons = %#v, %v", got, err)
	}
}
