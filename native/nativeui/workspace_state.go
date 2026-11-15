package nativeui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// workspaceState stores only durable navigation state. Draft contents remain
// in the document store; restoring a missing file must never recreate it.
type workspaceState struct {
	Root   string `json:"root,omitempty"`
	Path   string `json:"path,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

func workspaceStatePath() string {
	if dir := os.Getenv("MUSE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "workspace.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Muse", "workspace.json")
}

func loadWorkspaceState() workspaceState {
	data, err := os.ReadFile(workspaceStatePath())
	if err != nil {
		return workspaceState{}
	}
	var s workspaceState
	if json.Unmarshal(data, &s) != nil {
		return workspaceState{}
	}
	if s.Root != "" {
		if st, err := os.Stat(s.Root); err != nil || !st.IsDir() {
			s.Root = ""
		}
	}
	if s.Path != "" {
		if st, err := os.Stat(s.Path); err != nil || !st.Mode().IsRegular() {
			s.Path = ""
		}
	}
	if s.Root != "" && s.Path != "" && !isWithinRoot(s.Root, s.Path) {
		s.Path = ""
	}
	return s
}

func saveWorkspaceState(s workspaceState) error {
	path := workspaceStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (a *App) persistWorkspaceState() {
	if a == nil {
		return
	}
	s := workspaceState{Root: a.root, Path: a.path}
	if a.ed != nil {
		s.Offset = a.ed.SelectionRange().End
	}
	if s.Root == "" && s.Path != "" {
		s.Root = filepath.Dir(s.Path)
	}
	_ = saveWorkspaceState(s)
}

func (a *App) restoreWorkspaceState() {
	s := loadWorkspaceState()
	if s.Root != "" {
		a.setWorkspace(s.Root)
	}
	if s.Path != "" {
		a.openPathNow(s.Path)
		if a.path == s.Path {
			a.ed.Select(s.Offset, s.Offset)
			a.ed.ScrollToOffset(s.Offset)
		}
	}
}
