package vaultsearchstore

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreUsesInjectedSharedCacheRoot(t *testing.T) {
	cacheRoot := filepath.Join(t.TempDir(), "shared-cache")
	store, err := New(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	vaultRoot := t.TempDir()
	index, err := store.Open(vaultRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.Abs(filepath.Clean(vaultRoot))
	if err != nil {
		t.Fatal(err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(canonical); resolveErr == nil {
		canonical = resolved
	}
	key := sha256.Sum256([]byte(canonical))
	want := filepath.Join(cacheRoot, "vaultsearch", hex.EncodeToString(key[:]))
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("shared vault search cache missing at %q: %v", want, err)
	}
}
