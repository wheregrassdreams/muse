package search

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// localTestStorage lets in-package benchmarks open a real Tantivy directory
// without reintroducing a production filesystem implementation in search.
type localTestStorage struct{ root string }

func openLocalTestIndex(root string) (*Index, error) {
	return OpenWithStorage(localTestStorage{root: root})
}

func (s localTestStorage) Root() string { return s.root }

func (s localTestStorage) path(relative string) (string, error) {
	if !filepath.IsLocal(relative) {
		return "", fmt.Errorf("invalid test storage path %q", relative)
	}
	return filepath.Join(s.root, relative), nil
}

func (s localTestStorage) MkdirAll(relative string, perm fs.FileMode) error {
	path, err := s.path(relative)
	if err != nil {
		return err
	}
	return os.MkdirAll(path, perm)
}

func (s localTestStorage) Stat(relative string) (fs.FileInfo, error) {
	path, err := s.path(relative)
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
}

func (s localTestStorage) ReadFile(relative string) ([]byte, error) {
	path, err := s.path(relative)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s localTestStorage) WriteAtomic(relative string, data []byte, perm fs.FileMode) error {
	path, err := s.path(relative)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(perm); err != nil {
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
	return os.Rename(tmp.Name(), path)
}

func (s localTestStorage) WriteAndSync(relative string, data []byte, perm fs.FileMode) error {
	path, err := s.path(relative)
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

func (s localTestStorage) Rename(oldRelative, newRelative string) error {
	oldPath, err := s.path(oldRelative)
	if err != nil {
		return err
	}
	newPath, err := s.path(newRelative)
	if err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func (s localTestStorage) RemoveAndSync(relative string) error {
	path, err := s.path(relative)
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
	return errors.Join(dir.Sync(), dir.Close())
}

func (s localTestStorage) TryLock(relative string) (Lock, bool, error) {
	path, err := s.path(relative)
	if err != nil {
		return nil, false, err
	}
	lock := flock.New(path)
	locked, err := lock.TryLock()
	if err != nil || !locked {
		return nil, locked, err
	}
	return lock, true, nil
}
