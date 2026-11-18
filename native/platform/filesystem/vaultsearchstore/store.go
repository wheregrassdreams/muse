package vaultsearchstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"muse/platform/filesystem/atomicfile"
	"muse/search"
)

// Store resolves one persistent search-index directory per vault and opens it
// through search's consumer-owned persistence contract.
type Store struct{ cacheRoot string }

func New(cacheRoot string) (*Store, error) {
	if cacheRoot == "" {
		return nil, errors.New("vaultsearchstore: cache root is required")
	}
	return &Store{cacheRoot: cacheRoot}, nil
}

func (s *Store) Open(root string) (*search.Index, error) {
	canonical, err := canonicalPath(root)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(canonical))
	dir := filepath.Join(s.cacheRoot, "vaultsearch", hex.EncodeToString(key[:]))
	storage, err := NewAt(dir)
	if err != nil {
		return nil, err
	}
	return search.OpenWithStorage(storage)
}

// NewAt creates a filesystem adapter for a specific persistent index root.
// It is useful for tools that manage an index outside a vault-scoped Store.
func NewAt(root string) (*Filesystem, error) {
	canonical, err := canonicalPath(root)
	if err != nil {
		return nil, err
	}
	return &Filesystem{root: canonical}, nil
}

// Filesystem implements search.Persistence. Search chooses relative artifact
// names and recovery policy; this adapter performs their OS file operations.
type Filesystem struct{ root string }

func (f *Filesystem) Root() string { return f.root }

func (f *Filesystem) MkdirAll(relative string, perm fs.FileMode) error {
	path, err := f.resolve(relative)
	if err != nil {
		return err
	}
	return os.MkdirAll(path, perm)
}

func (f *Filesystem) Stat(relative string) (fs.FileInfo, error) {
	path, err := f.resolve(relative)
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
}

func (f *Filesystem) ReadFile(relative string) ([]byte, error) {
	path, err := f.resolve(relative)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (f *Filesystem) WriteAtomic(relative string, data []byte, perm fs.FileMode) error {
	path, err := f.resolve(relative)
	if err != nil {
		return err
	}
	stage, err := atomicfile.Write(path, ".manifest-*.tmp", data, perm)
	if err != nil {
		return fmt.Errorf("%s: %w", stage, err)
	}
	return nil
}

func (f *Filesystem) WriteAndSync(relative string, data []byte, perm fs.FileMode) error {
	path, err := f.resolve(relative)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func (f *Filesystem) Rename(oldRelative, newRelative string) error {
	oldPath, err := f.resolve(oldRelative)
	if err != nil {
		return err
	}
	newPath, err := f.resolve(newRelative)
	if err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func (f *Filesystem) RemoveAndSync(relative string) error {
	path, err := f.resolve(relative)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	return errors.Join(syncErr, closeErr)
}

func (f *Filesystem) TryLock(relative string) (search.Lock, bool, error) {
	path, err := f.resolve(relative)
	if err != nil {
		return nil, false, err
	}
	lock := flock.New(path)
	locked, err := lock.TryLock()
	if err != nil {
		return nil, false, err
	}
	if !locked {
		return nil, false, nil
	}
	return lock, true, nil
}

func (f *Filesystem) resolve(relative string) (string, error) {
	if !filepath.IsLocal(relative) {
		return "", fmt.Errorf("vaultsearchstore: invalid relative path %q", relative)
	}
	return filepath.Join(f.root, relative), nil
}

func canonicalPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("vaultsearchstore: path is required")
	}
	canonical, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(canonical); resolveErr == nil {
		canonical = resolved
	}
	return canonical, nil
}

var _ search.Persistence = (*Filesystem)(nil)
