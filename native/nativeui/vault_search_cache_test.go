package nativeui

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"muse/cachepath"
)

func TestOpenVaultSearchUsesSharedCacheRoot(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "shared-cache")
	legacy := filepath.Join(t.TempDir(), "legacy-search-cache")
	t.Setenv(cachepath.RootEnv, shared)
	// MUSE_SEARCH_CACHE was a search-only override. It is deliberately ignored
	// now so every disposable Muse cache follows the shared cache root.
	t.Setenv("MUSE_SEARCH_CACHE", legacy)
	vault := t.TempDir()
	idx, err := openVaultSearch(vault)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.Abs(vault)
	if err != nil {
		t.Fatal(err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(canonical); resolveErr == nil {
		canonical = resolved
	}
	hash := sha256.Sum256([]byte(filepath.Clean(canonical)))
	want := filepath.Join(shared, "vaultsearch", hex.EncodeToString(hash[:]))
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("shared vault search cache missing at %q: %v", want, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy search-only cache was used or created: %v", err)
	}
}
