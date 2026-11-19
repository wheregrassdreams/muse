package filetree

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestBuildRowsOwnsCatalogFilteringSortingExpansionAndCache(t *testing.T) {
	root := filepath.Join("/vault")
	childrenByDirectory := map[string][]CatalogEntry{
		root: {
			{Path: "z.md", Kind: CatalogFile},
			{Path: "notes", Kind: CatalogDirectory},
			{Path: "image.png", Kind: CatalogFile},
			{Path: "a.md", Kind: CatalogFile},
		},
		filepath.Join(root, "notes"): {{Path: "notes/child.md", Kind: CatalogFile}},
	}
	calls := 0
	port := CatalogPort{
		Children: func(directory string) []CatalogEntry {
			calls++
			return childrenByDirectory[directory]
		},
		VisibleFile: func(path string) bool { return filepath.Ext(path) == ".md" },
	}
	handle := NewHandle()

	first := BuildRows(handle, root, port)
	second := BuildRows(handle, root, port)
	if len(first) != 3 || &first[0] != &second[0] || calls != 1 {
		t.Fatalf("cached rows = %+v / %+v, calls=%d; want one projection and three rows", first, second, calls)
	}
	if first[0].Path != filepath.Join(root, "notes") || !first[0].Directory || first[1].Path != filepath.Join(root, "a.md") {
		t.Fatalf("projected order/kinds = %+v", first)
	}

	handle.SetExpanded(filepath.Join(root, "notes"), true)
	expanded := BuildRows(handle, root, port)
	if len(expanded) != 4 || expanded[1].Path != filepath.Join(root, "notes", "child.md") || expanded[1].Depth != 1 {
		t.Fatalf("expanded projection = %+v", expanded)
	}

	childrenByDirectory[root] = append(childrenByDirectory[root], CatalogEntry{Path: "new.md", Kind: CatalogFile})
	handle.InvalidateRows()
	refreshed := BuildRows(handle, root, port)
	newFound := false
	for _, row := range refreshed {
		newFound = newFound || row.Path == filepath.Join(root, "new.md")
	}
	if len(refreshed) != 5 || !newFound {
		t.Fatalf("refreshed projection = %+v", refreshed)
	}
}

func BenchmarkBuildRowsExpandedCatalog(b *testing.B) {
	root := b.TempDir()
	children := map[string][]CatalogEntry{}
	for i := range 48 {
		folder := filepath.Join(root, fmt.Sprintf("folder-%02d", i))
		children[root] = append(children[root], CatalogEntry{Path: filepath.Base(folder), Kind: CatalogDirectory})
		entries := make([]CatalogEntry, 48)
		for j := range entries {
			entries[j] = CatalogEntry{Path: filepath.ToSlash(filepath.Join(filepath.Base(folder), fmt.Sprintf("note-%02d.md", j))), Kind: CatalogFile}
		}
		children[folder] = entries
	}
	port := CatalogPort{Children: func(directory string) []CatalogEntry { return children[directory] }}
	handle := NewHandle()
	for i := range 48 {
		handle.SetExpanded(filepath.Join(root, fmt.Sprintf("folder-%02d", i)), true)
	}
	if rows := BuildRows(handle, root, port); len(rows) != 48*49 {
		b.Fatalf("expanded row count = %d, want %d", len(rows), 48*49)
	}
	b.Run("cached", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = BuildRows(handle, root, port)
		}
	})
}
