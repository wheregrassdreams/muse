package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testWorkspace struct{ root, current, source string }

func (w testWorkspace) Root() string                               { return w.root }
func (w testWorkspace) CurrentDocument() (string, string)          { return w.current, w.source }
func (w testWorkspace) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (w testWorkspace) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (w testWorkspace) ResolveWithinRoot(path string) (string, error) {
	root, err := filepath.EvalSymlinks(w.root)
	if err != nil {
		return "", err
	}
	missing := []string{}
	ancestor := path
	for {
		if _, statErr := os.Lstat(ancestor); statErr == nil {
			break
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", errors.New("no existing ancestor")
		}
		missing = append([]string{filepath.Base(ancestor)}, missing...)
		ancestor = parent
	}
	real, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	for _, part := range missing {
		real = filepath.Join(real, part)
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("outside workspace")
	}
	return real, nil
}

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

func TestWorkspaceToolsRejectSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	s := WorkspaceReadTools{Workspace: testWorkspace{root: root}}
	if _, err := s.ReadDocument(context.Background(), ToolInput{Path: "escape/secret.md"}); err == nil {
		t.Fatal("read through symlink outside workspace succeeded")
	}
}
