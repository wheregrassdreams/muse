package navicons

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"muse/cachepath"
)

func TestStorePersistsNormalizedVaultRelativeIcons(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv(cachepath.RootEnv, cacheRoot)
	vault := t.TempDir()

	store, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get(vault, "Projects/Atlas/Overview.md"); got != "" {
		t.Fatalf("new item icon = %q, want empty", got)
	}
	if err := store.Set(vault, `Projects\Atlas//Overview.md`, "bookmark"); err != nil {
		t.Fatal(err)
	}
	if got := store.Get(vault, "Projects/Atlas/Overview.md"); got != "bookmark" {
		t.Fatalf("normalized item icon = %q, want bookmark", got)
	}

	store, err = New()
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get(vault, "Projects/Atlas/Overview.md"); got != "bookmark" {
		t.Fatalf("reopened store icon = %q, want persisted bookmark", got)
	}

	canonicalRoot, err := canonicalVaultRoot(vault)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(cacheRoot, "navicons", vaultKey(canonicalRoot)+".json")
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("store file missing at shared cache path: %v", err)
	}
	if info.Mode().Perm() != fileMode {
		t.Fatalf("store file mode = %o, want %o", info.Mode().Perm(), fileMode)
	}
	if len(filepath.Base(file)) != 64+len(".json") {
		t.Fatalf("vault cache filename is not a fixed-size hash: %q", filepath.Base(file))
	}
}

func TestStoreIsolatesVaultNamespacesAndDeletesIcons(t *testing.T) {
	t.Setenv(cachepath.RootEnv, t.TempDir())
	store, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vaultA, vaultB := t.TempDir(), t.TempDir()
	if err := store.Set(vaultA, "same/path.md", "star"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(vaultB, "same/path.md", "heart"); err != nil {
		t.Fatal(err)
	}
	if got := store.Get(vaultA, "same/path.md"); got != "star" {
		t.Fatalf("vault A icon = %q, want star", got)
	}
	if got := store.Get(vaultB, "same/path.md"); got != "heart" {
		t.Fatalf("vault B icon = %q, want heart", got)
	}
	if err := store.Delete(vaultA, "same/path.md"); err != nil {
		t.Fatal(err)
	}
	if got := store.Get(vaultA, "same/path.md"); got != "" {
		t.Fatalf("deleted icon = %q, want empty", got)
	}
	if got := store.Get(vaultB, "same/path.md"); got != "heart" {
		t.Fatalf("deleting vault A changed vault B icon to %q", got)
	}
	if err := store.Delete(vaultA, "same/path.md"); err != nil {
		t.Fatalf("deleting an already absent icon should be a no-op: %v", err)
	}
}

func TestStoreRejectsOutsideAndInvalidItemPaths(t *testing.T) {
	t.Setenv(cachepath.RootEnv, t.TempDir())
	store, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vault := t.TempDir()
	for _, item := range []string{"", ".", "../outside.md", "folder/../../outside.md", "/absolute.md", `C:\outside.md`, "C:relative.md"} {
		if err := store.Set(vault, item, "bookmark"); err == nil {
			t.Errorf("Set accepted invalid item path %q", item)
		}
		if got := store.Get(vault, item); got != "" {
			t.Errorf("Get(%q) = %q for invalid path, want empty", item, got)
		}
		if err := store.Delete(vault, item); err == nil {
			t.Errorf("Delete accepted invalid item path %q", item)
		}
	}
	for _, iconID := range []string{"", " \t ", strings.Repeat("x", 257), "bad\nkey"} {
		if err := store.Set(vault, "note.md", iconID); err == nil {
			t.Errorf("Set accepted invalid icon ID %q", iconID)
		}
	}
}

func TestStoreCanonicalizesSymlinkVaultRoot(t *testing.T) {
	t.Setenv(cachepath.RootEnv, t.TempDir())
	store, err := New()
	if err != nil {
		t.Fatal(err)
	}
	actualRoot := t.TempDir()
	alias := filepath.Join(t.TempDir(), "vault-link")
	if err := os.Symlink(actualRoot, alias); err != nil {
		t.Skipf("cannot create symlink in this environment: %v", err)
	}
	if err := store.Set(alias, "note.md", "pin"); err != nil {
		t.Fatal(err)
	}
	if got := store.Get(actualRoot, "note.md"); got != "pin" {
		t.Fatalf("icon through canonical vault root = %q, want pin", got)
	}
}

func TestStoreConcurrentSetAndGet(t *testing.T) {
	t.Setenv(cachepath.RootEnv, t.TempDir())
	store, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vault := t.TempDir()
	const count = 16
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			item := filepath.ToSlash(filepath.Join("Notes", strings.Repeat("n", i+1)+".md"))
			if err := store.Set(vault, item, "icon"); err != nil {
				t.Errorf("Set(%q): %v", item, err)
				return
			}
			if got := store.Get(vault, item); got != "icon" {
				t.Errorf("Get(%q) = %q, want icon", item, got)
			}
		}()
	}
	wg.Wait()
}
