// Package workspacestore persists workspace snapshots in the per-user config
// directory. It validates portable layout data before it reaches the UI.
package workspacestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"muse/workspace"
)

type Store struct{ path string }

var _ workspace.Repository = (*Store)(nil)

func New(path string) *Store { return &Store{path: path} }

func DefaultPath() string {
	if dir := os.Getenv("MUSE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "workspace.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Muse", "workspace.json")
}

func (s *Store) Load() (workspace.State, error) {
	if s == nil || s.path == "" {
		return workspace.State{}, errors.New("workspace store path is empty")
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return workspace.State{Version: 1}, nil
	}
	if err != nil {
		return workspace.State{}, err
	}
	var state workspace.State
	if err := json.Unmarshal(data, &state); err != nil {
		return workspace.State{}, err
	}
	if state.Version == 0 {
		if len(state.Tabs) != 0 {
			return workspace.State{}, fmt.Errorf("%w: version is required for tabbed snapshots", workspace.ErrInvalidState)
		}
		state, err = migrateLegacy(data)
		if err != nil {
			return workspace.State{}, err
		}
	}
	if err := state.Validate(); err != nil {
		return workspace.State{}, err
	}
	return cleanMissingDocuments(state), nil
}

func (s *Store) Save(state workspace.State) error {
	if s == nil || s.path == "" {
		return errors.New("workspace store path is empty")
	}
	if err := state.Validate(); err != nil {
		return err
	}
	if err := validateDocumentPaths(state); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".workspace-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

type legacyState struct {
	Root   string `json:"root,omitempty"`
	Path   string `json:"path,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

func migrateLegacy(data []byte) (workspace.State, error) {
	var old legacyState
	if err := json.Unmarshal(data, &old); err != nil {
		return workspace.State{}, err
	}
	state := workspace.State{Version: 1, VaultPath: old.Root}
	if old.Path == "" {
		return state, nil
	}
	if old.Root == "" {
		state.VaultPath = filepath.Dir(old.Path)
	}
	offset := old.Offset
	if offset < 0 {
		offset = 0
	}
	const tabID, paneID = "main", "main"
	state.Tabs = []workspace.Tab{{
		ID:           tabID,
		ActivePaneID: paneID,
		Layout:       workspace.Leaf(workspace.Pane{ID: paneID, Document: workspace.Document{Path: old.Path, CursorOffset: offset}}),
	}}
	state.ActiveTab = tabID
	return state, nil
}

func cleanMissingDocuments(state workspace.State) workspace.State {
	root := state.VaultPath
	if root != "" {
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			return workspace.State{Version: 1}
		}
	}
	tabs := state.Tabs[:0]
	for _, tab := range state.Tabs {
		if node, ok := cleanNode(root, tab.Layout); ok {
			tab.Layout = node
			if !containsPane(node, tab.ActivePaneID) {
				tab.ActivePaneID = firstPane(node)
			}
			tabs = append(tabs, tab)
		}
	}
	state.Tabs = tabs
	if len(tabs) == 0 {
		state.ActiveTab = ""
		return state
	}
	for _, tab := range tabs {
		if tab.ID == state.ActiveTab {
			return state
		}
	}
	state.ActiveTab = tabs[0].ID
	return state
}

func cleanNode(root string, node workspace.LayoutNode) (workspace.LayoutNode, bool) {
	if node.Pane != nil {
		path := node.Pane.Document.Path
		if !filepath.IsAbs(path) || root == "" || !withinRoot(root, path) {
			return workspace.LayoutNode{}, false
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return workspace.LayoutNode{}, false
		}
		return node, true
	}
	first, firstOK := cleanNode(root, node.Split.First)
	next, nextOK := cleanNode(root, node.Split.Next)
	if !firstOK && !nextOK {
		return workspace.LayoutNode{}, false
	}
	if !firstOK {
		return next, true
	}
	if !nextOK {
		return first, true
	}
	return workspace.Branch(node.Split.Axis, node.Split.Ratio, first, next), true
}

func containsPane(node workspace.LayoutNode, id workspace.PaneID) bool {
	if id == "" {
		return false
	}
	if node.Pane != nil {
		return node.Pane.ID == id
	}
	return containsPane(node.Split.First, id) || containsPane(node.Split.Next, id)
}

func firstPane(node workspace.LayoutNode) workspace.PaneID {
	if node.Pane != nil {
		return node.Pane.ID
	}
	return firstPane(node.Split.First)
}

func withinRoot(root, path string) bool {
	rootAbs, rootErr := filepath.Abs(filepath.Clean(root))
	pathAbs, pathErr := filepath.Abs(filepath.Clean(path))
	if rootErr != nil || pathErr != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func validateDocumentPaths(state workspace.State) error {
	if len(state.Tabs) == 0 {
		return nil
	}
	if !filepath.IsAbs(state.VaultPath) {
		return fmt.Errorf("workspace: vault path must be absolute: %q", state.VaultPath)
	}
	for _, tab := range state.Tabs {
		if err := validateNodePaths(state.VaultPath, tab.Layout); err != nil {
			return err
		}
	}
	return nil
}

func validateNodePaths(root string, node workspace.LayoutNode) error {
	if node.Pane != nil {
		path := node.Pane.Document.Path
		if !filepath.IsAbs(path) || !withinRoot(root, path) {
			return fmt.Errorf("workspace: document path %q is outside vault %q", path, root)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("workspace: document %q is unavailable", path)
		}
		return nil
	}
	if err := validateNodePaths(root, node.Split.First); err != nil {
		return err
	}
	return validateNodePaths(root, node.Split.Next)
}
