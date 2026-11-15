package vault

import (
	"os"
	"path/filepath"
	"testing"

	"muse/vault/index"
)

func TestLinkGraphTracksVaultIndexLifecycle(t *testing.T) {
	root := t.TempDir()
	write := func(name, source string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("source.md", "[[target]]")
	write("target.md", "# Target")

	idx := index.New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	graph, err := NewLinkGraph(root, idx)
	if err != nil {
		t.Fatal(err)
	}
	if got := graph.Backlinks("target.md"); len(got) != 1 || got[0].Source != "source.md" {
		t.Fatalf("initial backlinks = %+v", got)
	}

	if err := idx.UpdateText("source.md", "[[other]]"); err != nil {
		t.Fatal(err)
	}
	if err := graph.Apply("source.md"); err != nil {
		t.Fatal(err)
	}
	if got := graph.Backlinks("target.md"); len(got) != 0 {
		t.Fatalf("updated graph kept stale backlink: %+v", got)
	}

	if err := os.Remove(filepath.Join(root, "source.md")); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(filepath.Join(root, "source.md")); err != nil {
		t.Fatal(err)
	}
	graph.Remove("source.md")
	if got := graph.Outgoing("source.md"); len(got) != 0 {
		t.Fatalf("deleted source retained outgoing links: %+v", got)
	}

	graph.Close()
	if _, ok := graph.Resolve("", "target"); ok {
		t.Fatal("closed vault graph still resolved links")
	}
}

func TestLinkGraphRebuildReplacesPreviousVault(t *testing.T) {
	rootA, rootB := t.TempDir(), t.TempDir()
	write := func(root, name, source string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(rootA, "source.md", "[[only-a]]")
	idx := index.New()
	if err := idx.Rebuild(rootA); err != nil {
		t.Fatal(err)
	}
	graph, err := NewLinkGraph(rootA, idx)
	if err != nil {
		t.Fatal(err)
	}
	write(rootB, "source.md", "[[only-b]]")
	if err := idx.Rebuild(rootB); err != nil {
		t.Fatal(err)
	}
	graph.Close()
	indexB := index.New()
	if err := indexB.Rebuild(rootB); err != nil {
		t.Fatal(err)
	}
	graphB, err := NewLinkGraph(rootB, indexB)
	if err != nil {
		t.Fatal(err)
	}
	if got := graphB.Outgoing("source.md"); len(got) != 1 || got[0].Target != "only-b" {
		t.Fatalf("new vault graph = %+v", got)
	}
}
