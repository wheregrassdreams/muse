// Package vaultindexsource adapts a vault root on the local filesystem to the
// read-only source contract consumed by vault/index.
package vaultindexsource

import (
	"errors"
	"fmt"
	"io"
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
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	return fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		projected := vaultindex.Entry{Path: path, Kind: entryKind(entry.Type(), entry.IsDir())}
		if err := visit(projected); err != nil {
			if errors.Is(err, vaultindex.ErrSkipDir) && projected.Kind == vaultindex.EntryDirectory {
				return fs.SkipDir
			}
			return err
		}
		return nil
	})
}

func (s *Source) Stat(path string) (vaultindex.Entry, error) {
	rel, err := s.resolve(path)
	if err != nil {
		return vaultindex.Entry{}, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return vaultindex.Entry{}, err
	}
	defer root.Close()
	parent, name, closeParent, err := openParentRoot(root, rel)
	if err != nil {
		return vaultindex.Entry{}, err
	}
	if closeParent {
		defer parent.Close()
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return vaultindex.Entry{}, err
	}
	return vaultindex.Entry{Path: rel, Kind: entryKind(info.Mode(), info.IsDir())}, nil
}

func (s *Source) Read(path string) ([]byte, error) {
	rel, err := s.resolve(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	parent, name, closeParent, err := openParentRoot(root, rel)
	if err != nil {
		return nil, err
	}
	if closeParent {
		defer parent.Close()
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if entryKind(info.Mode(), info.IsDir()) != vaultindex.EntryRegular {
		return nil, fs.ErrInvalid
	}
	file, err := parent.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	currentInfo, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if currentInfo.Mode()&fs.ModeSymlink != 0 || !os.SameFile(openedInfo, currentInfo) {
		return nil, fs.ErrInvalid
	}
	return io.ReadAll(file)
}

func (s *Source) resolve(path string) (relative string, err error) {
	if s == nil || s.root == "" {
		return "", errors.New("vaultindexsource: root is required")
	}
	if path == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", fmt.Errorf("vaultindexsource: expected relative path, got %q", path)
	}
	normalized := filepath.Clean(filepath.FromSlash(path))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("vaultindexsource: path escapes root: %q", path)
	}
	relative = filepath.ToSlash(normalized)
	return relative, nil
}

func openParentRoot(root *os.Root, relative string) (parent *os.Root, name string, closeParent bool, err error) {
	parts := strings.Split(relative, "/")
	current := root
	owned := false
	for _, part := range parts[:len(parts)-1] {
		info, statErr := current.Lstat(part)
		if statErr != nil {
			if owned {
				_ = current.Close()
			}
			return nil, "", false, statErr
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			if owned {
				_ = current.Close()
			}
			return nil, "", false, errors.New("vaultindexsource: path traverses symlink")
		}
		if !info.IsDir() {
			if owned {
				_ = current.Close()
			}
			return nil, "", false, fs.ErrNotExist
		}
		next, openErr := current.OpenRoot(part)
		if openErr != nil {
			if owned {
				_ = current.Close()
			}
			return nil, "", false, openErr
		}
		openedInfo, infoErr := next.Stat(".")
		currentInfo, currentErr := current.Lstat(part)
		if infoErr != nil || currentErr != nil || currentInfo.Mode()&fs.ModeSymlink != 0 || !os.SameFile(openedInfo, currentInfo) {
			_ = next.Close()
			if owned {
				_ = current.Close()
			}
			if infoErr != nil {
				return nil, "", false, infoErr
			}
			if currentErr != nil {
				return nil, "", false, currentErr
			}
			return nil, "", false, errors.New("vaultindexsource: path traverses symlink or changed during lookup")
		}
		if owned {
			_ = current.Close()
		}
		current, owned = next, true
	}
	return current, parts[len(parts)-1], owned, nil
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
