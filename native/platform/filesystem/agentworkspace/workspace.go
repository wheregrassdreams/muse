// Package agentworkspace adapts a local vault directory to assistant workspace
// capabilities while keeping current editor state explicitly injected.
package agentworkspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"muse/application/agent"
)

type CurrentDocument func() (path, source string)

type Workspace struct {
	root    string
	current CurrentDocument
}

func New(root string, current CurrentDocument) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("agentworkspace: root is required")
	}
	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("agentworkspace: root is not a directory")
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &Workspace{root: filepath.Clean(canonical), current: current}, nil
}

func (w *Workspace) Root() string {
	if w == nil {
		return ""
	}
	return w.root
}
func (w *Workspace) CurrentDocument() (string, string) {
	if w == nil || w.current == nil {
		return "", ""
	}
	return w.current()
}

func (w *Workspace) ResolveWithinRoot(path string) (string, error) {
	if w == nil || w.root == "" {
		return "", errors.New("agentworkspace: workspace is unavailable")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(w.root, path)
	}
	path = filepath.Clean(path)
	ancestor := path
	missing := []string{}
	for {
		_, err := os.Lstat(ancestor)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", errors.New("agentworkspace: no existing ancestor")
		}
		missing = append([]string{filepath.Base(ancestor)}, missing...)
		ancestor = parent
	}
	canonical, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	for _, part := range missing {
		canonical = filepath.Join(canonical, part)
	}
	if !within(w.root, canonical) {
		return "", errors.New("agentworkspace: symlink escapes workspace")
	}
	return filepath.Clean(canonical), nil
}

func (w *Workspace) ReadFile(path string) ([]byte, error) {
	resolved, err := w.ResolveWithinRoot(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}
func (w *Workspace) ReadDir(path string) ([]os.DirEntry, error) {
	resolved, err := w.ResolveWithinRoot(path)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(resolved)
}

// Exists reports whether a workspace path exists without following a final
// symlink. Callers still resolve the path first, so parent symlinks cannot
// escape the workspace.
func (w *Workspace) Exists(path string) (bool, error) {
	resolved, err := w.ResolveWithinRoot(path)
	if err != nil {
		return false, err
	}
	_, err = os.Lstat(resolved)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// CreateFolder creates a workspace directory after resolving its existing
// ancestor and verifying that the resulting path remains within the root.
func (w *Workspace) CreateFolder(path string) error {
	resolved, err := w.ResolveWithinRoot(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(resolved, 0o755)
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

var _ agent.WorkspacePort = (*Workspace)(nil)
