package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"muse/nativeui/panels/filetree"
)

func TestFileRowsCacheReusesAndInvalidatesFlattenedTree(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "notes")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Home.md"), []byte("home"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "Child.md"), []byte("child"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := newFileTreeTestApp(t, root)
	a.root = root
	first := a.fileRows()
	second := a.fileRows()
	if len(first) != 2 || len(second) != 2 || &first[0] != &second[0] {
		t.Fatalf("unchanged tree did not reuse its flattened rows: first=%p second=%p rows=%+v", &first[0], &second[0], second)
	}

	a.fileTreeState.SetExpanded(folder, true)
	expanded := a.fileRows()
	childVisible := false
	for _, row := range expanded {
		childVisible = childVisible || row.Path == filepath.Join(folder, "Child.md")
	}
	if len(expanded) != 3 || !childVisible {
		t.Fatalf("expanded tree rows = %+v", expanded)
	}

	if err := os.WriteFile(filepath.Join(root, "New.md"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.clearTreeCache()
	refreshed := a.fileRows()
	newVisible := false
	for _, row := range refreshed {
		newVisible = newVisible || row.Path == filepath.Join(root, "New.md")
	}
	if len(refreshed) != 4 || !newVisible {
		t.Fatalf("tree refresh did not rebuild rows after filesystem invalidation: %+v", refreshed)
	}
}

func BenchmarkExpandedFileRows(b *testing.B) {
	root := b.TempDir()
	const folders, filesPerFolder = 48, 48
	for i := range folders {
		folder := filepath.Join(root, filepath.Join("folder", string(rune('A'+i/26))+string(rune('A'+i%26))))
		if err := os.MkdirAll(folder, 0o755); err != nil {
			b.Fatal(err)
		}
		for j := range filesPerFolder {
			path := filepath.Join(folder, filepath.Base(folder)+string(rune('A'+j/26))+string(rune('A'+j%26))+".md")
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}

	newTree := func() *App {
		a := &App{root: root, dirs: make(map[string][]string), isDir: make(map[string]bool), fileTreeState: filetree.NewHandle()}
		a.fileTreeState.SetExpanded(filepath.Join(root, "folder"), true)
		for i := range folders {
			a.fileTreeState.SetExpanded(filepath.Join(root, filepath.Join("folder", string(rune('A'+i/26))+string(rune('A'+i%26)))), true)
		}
		if rows := a.fileRows(); len(rows) != folders*(filesPerFolder+1)+1 {
			b.Fatalf("expanded row count = %d", len(rows))
		}
		return a
	}

	b.Run("cached", func(b *testing.B) {
		a := newTree()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = a.fileRows()
		}
	})
	b.Run("rebuild", func(b *testing.B) {
		a := newTree()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			a.fileTreeState.InvalidateRows()
			_ = a.fileRows()
		}
	})
}
