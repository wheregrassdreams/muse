package nativeui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWiki(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "areas", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"Ideas.md", "areas/Ideas.md", "areas/deep/Ideas.md", "Notes.md"} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := []fileRow{
		{path: filepath.Join(dir, "areas", "deep", "Ideas.md")},
		{path: filepath.Join(dir, "areas", "Ideas.md")},
		{path: filepath.Join(dir, "Ideas.md")},
		{path: filepath.Join(dir, "Notes.md")},
	}

	// A bare title finds the note, the shortest path winning.
	if got, want := resolveWiki(dir, dir, files, "Ideas"), filepath.Join(dir, "Ideas.md"); got != want {
		t.Errorf("Ideas resolves to %q, want %q", got, want)
	}
	// An anchor is dropped…
	if got, want := resolveWiki(dir, dir, files, "Notes#Heading"), filepath.Join(dir, "Notes.md"); got != want {
		t.Errorf("Notes#Heading resolves to %q, want %q", got, want)
	}
	if got, want := resolveWiki(dir, dir, files, "Notes^block"), filepath.Join(dir, "Notes.md"); got != want {
		t.Errorf("Notes^block resolves to %q, want %q", got, want)
	}
	// …and a target no note carries resolves to nothing.
	if got := resolveWiki(dir, dir, files, "Missing"); got != "" {
		t.Errorf("Missing resolves to %q", got)
	}
	if got := resolveWiki(dir, dir, files, "  "); got != "" {
		t.Errorf("an empty target resolves to %q", got)
	}
}
