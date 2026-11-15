package vault

import (
	"os"
	"path/filepath"
	"testing"

	vaultfiles "muse/vault/files"
	vaultindex "muse/vault/index"
)

func TestQuickOpenBuildsPathTitleAndAliasCandidates(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Projects", "Atlas", "Note.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\naliases: [Launch plan]\n---\n# Project Overview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := vaultfiles.New(root)
	if err := files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	index := vaultindex.New()
	if err := index.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	quickOpen := NewQuickOpen(root, files, index)

	got := quickOpen.Search("launch")
	if len(got) != 1 {
		t.Fatalf("alias query matches = %+v", got)
	}
	if got[0].Path != path || got[0].PathText != "Projects/Atlas/Note.md" || got[0].Title != "Project Overview" {
		t.Fatalf("candidate projection = %+v", got[0])
	}

	byTitle := quickOpen.Search("overview")
	if len(byTitle) != 1 || byTitle[0].Path != path {
		t.Fatalf("title query matches = %+v", byTitle)
	}
	all := quickOpen.Search("")
	if len(all) != 1 || all[0].Path != path {
		t.Fatalf("empty query candidates = %+v", all)
	}
}

func TestQuickOpenRefreshesCandidateSnapshotForFileAndIndexRevisions(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.md")
	if err := os.WriteFile(firstPath, []byte("# First"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := vaultfiles.New(root)
	if err := files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	index := vaultindex.New()
	if err := index.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	quickOpen := NewQuickOpen(root, files, index)
	initial := quickOpen.Search("first")
	if len(initial) != 1 || initial[0].Path != firstPath {
		t.Fatalf("initial matches = %+v", initial)
	}

	secondPath := filepath.Join(root, "second.md")
	if err := os.WriteFile(secondPath, []byte("---\naliases: [new alias]\n---\n# Second title\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := index.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	byAlias := quickOpen.Search("new alias")
	if len(byAlias) != 1 || byAlias[0].Path != secondPath || byAlias[0].Title != "Second title" {
		t.Fatalf("matches after catalog/index refresh = %+v", byAlias)
	}
	all := quickOpen.Search("")
	if len(all) != 2 {
		t.Fatalf("candidate count after file revision = %d, want 2", len(all))
	}

	if err := os.WriteFile(secondPath, []byte("---\naliases: [changed alias]\n---\n# Changed title\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := index.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	byChangedAlias := quickOpen.Search("changed alias")
	if len(byChangedAlias) != 1 || byChangedAlias[0].Path != secondPath || byChangedAlias[0].Title != "Changed title" {
		t.Fatalf("matches after index-only refresh = %+v", byChangedAlias)
	}
	if stale := quickOpen.Search("new alias"); len(stale) != 0 {
		t.Fatalf("old index metadata remained searchable: %+v", stale)
	}
}
