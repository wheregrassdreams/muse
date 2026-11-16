package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestQuickOpenFuzzyJump(t *testing.T) {
	dir := t.TempDir()
	for path, source := range map[string]string{
		"note.md":      "# note",
		"aardvark.txt": "x",
		"笔记A.md":       "# A",
		"子/笔记B.md":     "# B",
	} {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, tt, _ := runtimeFixtureAt(t, dir, filepath.Join(dir, "note.md"))
	a.openQuickOpen()
	tt.Frame()
	if !a.quickOpenOpen() {
		t.Fatal("Quick Open did not open")
	}
	// Empty-query results use stable path order.
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.quickOpenOpen() || a.documentPath() != filepath.Join(dir, "aardvark.txt") {
		t.Fatalf("empty query selected %q, open=%v", a.documentPath(), a.quickOpenOpen())
	}
	a.openQuickOpen()
	tt.Frame()
	tt.Type("B")
	tt.Frame()
	candidates := a.quickOpenCandidates("B")
	if len(candidates) != 1 || filepath.Base(candidates[0].path) != "笔记B.md" {
		t.Fatalf("unexpected fuzzy candidates: %+v", candidates)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.quickOpenOpen() || a.documentPath() != filepath.Join(dir, "子", "笔记B.md") {
		t.Fatalf("selected note was not opened: path=%q open=%v", a.documentPath(), a.quickOpenOpen())
	}
}

func TestQuickOpenUsesVaultCatalogForCollapsedFiles(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "closed", "nested-note.md")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("# nested"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _, _ := runtimeFixtureAt(t, dir, nested)
	candidates := a.quickOpenCandidates("nested")
	if len(candidates) != 1 || candidates[0].path != nested {
		t.Fatalf("collapsed nested file missing from Quick Open: %+v", candidates)
	}
}

func TestQuickOpenMatchesIndexedTitleAndAlias(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Projects", "Atlas", "Note.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\naliases: [Launch plan]\n---\n# Project Overview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _, _ := runtimeFixtureAt(t, dir, path)
	candidates := a.quickOpenCandidates("launch")
	if len(candidates) != 1 || candidates[0].path != path || candidates[0].title != "Project Overview" {
		t.Fatalf("alias/title projection mismatch: %+v", candidates)
	}
}

func TestQuickOpenRefreshesAfterVaultSwitch(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	firstPath := filepath.Join(first, "old-note.md")
	if err := os.WriteFile(firstPath, []byte("# Old"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, _ := runtimeFixtureAt(t, first, firstPath)
	old := a.quickOpenCandidates("old")
	if len(old) != 1 {
		t.Fatalf("initial candidates=%+v", old)
	}
	oldSession := a.vault

	secondPath := filepath.Join(second, "next-note.md")
	if err := os.WriteFile(secondPath, []byte("# Next"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.setWorkspace(second)
	// The old session is closed before the replacement starts.
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	a.openPathNow(secondPath)
	waitDocument(t, tt, func() bool { return a.vault != nil && a.vaultSnapshot.Ready && a.vaultSnapshot.Root == second })
	if err := oldSession.Close(); err != nil {
		t.Fatal(err)
	}
	newItems := a.quickOpenCandidates("next")
	if len(newItems) != 1 || newItems[0].path != secondPath || newItems[0].path == old[0].path {
		t.Fatalf("new vault reused stale candidates: %+v", newItems)
	}
}

func TestCloseQuickOpenClosesDialog(t *testing.T) {
	a := New()
	a.closeQuickOpen()
	if a.quickOpenOpen() {
		t.Fatal("closing Quick Open left the dialog open")
	}
}
