package images

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSourceDecodesEscapesAndConfinesVaultPaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Attachments", "photo space.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := ResolveSource("photo%20space.png", SourceOptions{BaseDir: filepath.Dir(path), Root: root})
	if got != path {
		t.Fatalf("escaped source resolved to %q, want %q", got, path)
	}
	if got := ResolveSource("../outside.png", SourceOptions{BaseDir: root, Root: root}); got != "" {
		t.Fatalf("vault traversal resolved to %q", got)
	}
	const remote = "https://example.test/a%20b.png"
	if got := ResolveSource(remote, SourceOptions{Root: root}); got != remote {
		t.Fatalf("remote source changed to %q", got)
	}
	const data = "data:image/png;base64,abc"
	if got := ResolveSource(data, SourceOptions{Root: root}); got != data {
		t.Fatalf("data source changed")
	}
}

func TestResolveSourceUsesBareNameLookupOnly(t *testing.T) {
	root := t.TempDir()
	attachment := filepath.Join(root, "Attachments", "icon.svg")
	if err := os.MkdirAll(filepath.Dir(attachment), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(attachment, []byte("<svg/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	lookupCalls := 0
	lookup := func(name string) string {
		lookupCalls++
		if name == "icon.svg" {
			return attachment
		}
		return ""
	}
	got := ResolveSource("icon.svg", SourceOptions{BaseDir: root, Root: root, Lookup: lookup})
	if got != attachment || lookupCalls != 1 {
		t.Fatalf("bare attachment resolution=(%q,%d calls), want (%q,1)", got, lookupCalls, attachment)
	}
	lookupCalls = 0
	ResolveSource("nested/icon.svg", SourceOptions{BaseDir: root, Root: root, Lookup: lookup})
	if lookupCalls != 0 {
		t.Fatalf("path reference invoked attachment lookup %d times", lookupCalls)
	}
}
