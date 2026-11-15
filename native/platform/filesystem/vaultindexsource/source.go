// Package vaultindexsource adapts a vault root on the local filesystem to the
// read-only source contract consumed by vault/index.
package vaultindexsource

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	vaultindex "muse/vault/index"
)

// Source is a filesystem-backed vault index source. It owns path resolution,
// recursive enumeration, metadata lookup, and file reads.
type Source struct {
	root string
}

// New creates a source rooted at root. The directory is validated by Walk so
// callers can construct it before asynchronously starting an index rebuild.
func New(root string) (*Source, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Source{root: filepath.Clean(absolute)}, nil
}

func (s *Source) Root() string { return s.root }

// Walk enumerates local entries without following symlinks. Entries are
// sorted by vault-relative path for stable rebuilds and test behavior.
func (s *Source) Walk(visit func(vaultindex.Entry) error) error {
	if s == nil || s.root == "" {
		return errors.New("vaultindexsource: root is required")
	}
	if visit == nil {
		return errors.New("vaultindexsource: walk callback is required")
	}
	info, err := os.Stat(s.root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("vaultindexsource: root is not a directory: %s", s.root)
	}
	return filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		projected := vaultindex.Entry{Path: filepath.ToSlash(rel), Kind: entryKind(entry.Type(), entry.IsDir())}
		if err := visit(projected); err != nil {
			if errors.Is(err, vaultindex.ErrSkipDir) && projected.Kind == vaultindex.EntryDirectory {
				return filepath.SkipDir
			}
			return err
		}
		return nil
	})
}

func (s *Source) Stat(path string) (vaultindex.Entry, error) {
	_, rel, err := s.resolve(path)
	if err != nil {
		return vaultindex.Entry{}, err
	}
	parts := strings.Split(rel, "/")
	current := s.root
	for n, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return vaultindex.Entry{}, err
		}
		kind := entryKind(info.Mode(), info.IsDir())
		if kind == vaultindex.EntrySymlink {
			return vaultindex.Entry{Path: rel, Kind: vaultindex.EntrySymlink}, nil
		}
		if n < len(parts)-1 && !info.IsDir() {
			return vaultindex.Entry{}, fs.ErrNotExist
		}
		if n == len(parts)-1 {
			return vaultindex.Entry{Path: rel, Kind: kind}, nil
		}
	}
	return vaultindex.Entry{}, fs.ErrNotExist
}

func (s *Source) Read(path string) ([]byte, error) {
	abs, _, err := s.resolve(path)
	if err != nil {
		return nil, err
	}
	entry, err := s.Stat(path)
	if err != nil {
		return nil, err
	}
	if entry.Kind != vaultindex.EntryRegular {
		return nil, fs.ErrInvalid
	}
	return os.ReadFile(abs)
}

func (s *Source) resolve(path string) (absolute, relative string, err error) {
	if s == nil || s.root == "" {
		return "", "", errors.New("vaultindexsource: root is required")
	}
	if path == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", "", fmt.Errorf("vaultindexsource: expected relative path, got %q", path)
	}
	normalized := filepath.Clean(filepath.FromSlash(path))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("vaultindexsource: path escapes root: %q", path)
	}
	absolute = filepath.Join(s.root, normalized)
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
			return "", "", fmt.Errorf("vaultindexsource: path traverses symlink: %q", path)
		}
		if !info.IsDir() {
			return "", "", fs.ErrNotExist
		}
	}
	return absolute, relative, nil
}

func entryKind(mode fs.FileMode, isDir bool) vaultindex.EntryKind {
	if mode&fs.ModeSymlink != 0 {
		return vaultindex.EntrySymlink
	}
	if isDir {
		return vaultindex.EntryDirectory
	}
	if mode.IsRegular() {
		return vaultindex.EntryRegular
	}
	return vaultindex.EntryOther
}
