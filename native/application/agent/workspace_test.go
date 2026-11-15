package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type testWorkspace struct{ root, current, source string }

func (w testWorkspace) Root() string                               { return w.root }
func (w testWorkspace) CurrentDocument() (string, string)          { return w.current, w.source }
func (w testWorkspace) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (w testWorkspace) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }

func TestWorkspaceToolsBoundPathsAndReadCurrentBuffer(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "other.md"), []byte("# Other\nneedle"), 0600); err != nil {
		t.Fatal(err)
	}
	s := WorkspaceReadTools{Workspace: testWorkspace{root: root, current: filepath.Join(root, "current.md"), source: "# Unsaved\nlatest"}}
	got, err := s.ReadDocument(context.Background(), ToolInput{Path: "", Offset: 1, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["path"] != filepath.Join(root, "current.md") {
		t.Fatalf("current path: %#v", got)
	}
	if _, err := s.ReadDocument(context.Background(), ToolInput{Path: "../outside.md"}); err == nil {
		t.Fatal("accepted path outside workspace")
	}
	search, err := s.SearchDocuments(context.Background(), ToolInput{Path: "other.md", Query: "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if search.(map[string]any)["matches"] != 1 {
		t.Fatalf("search result: %#v", search)
	}
}

func TestWorkspaceToolsOutlineAndTreeBounds(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "note.md"), []byte("# Title\n## Second"), 0600); err != nil {
		t.Fatal(err)
	}
	s := WorkspaceReadTools{Workspace: testWorkspace{root: root, current: filepath.Join(root, "sub", "note.md"), source: "# Title\n## Second"}}
	outline, err := s.ReadOutline(context.Background(), ToolInput{})
	if err != nil {
		t.Fatal(err)
	}
	if outline.(map[string]any)["outline"] != "H1 Title\nH2 Second" {
		t.Fatalf("outline: %#v", outline)
	}
	tree, err := s.WorkspaceTree(context.Background(), ToolInput{Depth: 3})
	if err != nil {
		t.Fatal(err)
	}
	if tree.(map[string]any)["entries"] != 2 {
		t.Fatalf("tree: %#v", tree)
	}
}
