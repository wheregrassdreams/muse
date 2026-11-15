package cachepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootUsesDedicatedOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "test-cache")
	t.Setenv(RootEnv, override)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "xdg"))
	got, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(override)
	if got != want {
		t.Fatalf("Root() = %q, want override %q", got, want)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatalf("Root() created the cache directory: stat err = %v", err)
	}
}

func TestRootUsesXDGCacheHome(t *testing.T) {
	t.Setenv(RootEnv, "")
	xdg := filepath.Join(t.TempDir(), "xdg-cache")
	t.Setenv("XDG_CACHE_HOME", xdg)
	got, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(filepath.Join(xdg, appDir))
	if got != want {
		t.Fatalf("Root() = %q, want XDG cache path %q", got, want)
	}
}

func TestRootUsesXDGDefault(t *testing.T) {
	t.Setenv(RootEnv, "")
	t.Setenv("XDG_CACHE_HOME", "relative-path-is-invalid")
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(filepath.Join(home, ".cache", appDir))
	if got != want {
		t.Fatalf("Root() = %q, want XDG default %q", got, want)
	}
}
