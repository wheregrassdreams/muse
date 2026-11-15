// Package recentstore persists recently opened workspace paths as JSON.
package recentstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	workspaceapp "muse/application/workspace"
)

type Store struct{ path string }

var _ workspaceapp.RecentRepository = (*Store)(nil)

func New(path string) *Store { return &Store{path: path} }

func DefaultPath() string {
	if dir := os.Getenv("MUSE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "recent.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Muse", "recent.json")
}

func (s *Store) Load() ([]workspaceapp.RecentEntry, error) {
	if s == nil || s.path == "" {
		return nil, errors.New("recent store path is empty")
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []workspaceapp.RecentEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) Save(entries []workspaceapp.RecentEntry) error {
	if s == nil || s.path == "" {
		return errors.New("recent store path is empty")
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".recent-*.tmp")
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

func (s *Store) Exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info != nil
}
