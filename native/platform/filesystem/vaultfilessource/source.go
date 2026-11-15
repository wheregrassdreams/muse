// Package vaultfilessource adapts a local vault directory to vault/files' typed
// catalog source port.
package vaultfilessource

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	vaultfiles "muse/vault/files"
)

// Source enumerates and stats paths under one local vault root.
type Source struct {
	root string
}

// New creates a filesystem source. The directory is validated during Walk so
// the source can be assembled before an asynchronous catalog rebuild.
func New(root string) (*Source, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Source{root: filepath.Clean(absolute)}, nil
}

func (s *Source) Root() string { return s.root }

// Walk enumerates entries in deterministic order without following symlinks.
func (s *Source) Walk(visit func(vaultfiles.Entry) error) error {
	if s == nil || s.root == "" {
		return errors.New("vaultfilessource: root is required")
	}
	if visit == nil {
		return errors.New("vaultfilessource: walk callback is required")
	}
	info, err := os.Stat(s.root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("vaultfilessource: root is not a directory: %s", s.root)
	}
	return filepath.WalkDir(s.root, func(abs string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(s.root, abs)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		projected := vaultfiles.Entry{Path: filepath.ToSlash(rel), Kind: entryKind(entry.Type(), entry.IsDir())}
		if err := visit(projected); err != nil {
			if errors.Is(err, vaultfiles.ErrSkipDir) && projected.Kind == vaultfiles.EntryDirectory {
				return filepath.SkipDir
			}
			return err
		}
		return nil
	})
}

// Stat returns metadata for a vault-relative path. It classifies a final
// symlink without following it and rejects paths reached through symlinked
// parent directories.
func (s *Source) Stat(path string) (vaultfiles.Entry, error) {
	abs, rel, err := s.resolve(path)
	if err != nil {
		return vaultfiles.Entry{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return vaultfiles.Entry{}, err
	}
	return vaultfiles.Entry{Path: rel, Kind: entryKind(info.Mode(), info.IsDir())}, nil
}

func (s *Source) resolve(path string) (absolute, relative string, err error) {
	if s == nil || s.root == "" {
		return "", "", errors.New("vaultfilessource: root is required")
	}
	if path == "" || strings.Contains(path, "\\") || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", "", fmt.Errorf("vaultfilessource: expected relative path, got %q", path)
	}
	normalized := filepath.Clean(filepath.FromSlash(path))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("vaultfilessource: path escapes root: %q", path)
	}
	relative = filepath.ToSlash(normalized)
	current := s.root
	parts := strings.Split(relative, "/")
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", "", err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return "", "", fmt.Errorf("vaultfilessource: path traverses symlink: %q", path)
		}
		if !info.IsDir() {
			return "", "", fs.ErrNotExist
		}
	}
	return filepath.Join(s.root, normalized), relative, nil
}

func entryKind(mode fs.FileMode, isDir bool) vaultfiles.EntryKind {
	if mode&fs.ModeSymlink != 0 {
		return vaultfiles.EntrySymlink
	}
	if isDir {
		return vaultfiles.EntryDirectory
	}
	if mode.IsRegular() {
		return vaultfiles.EntryRegular
	}
	return vaultfiles.EntryOther
}
