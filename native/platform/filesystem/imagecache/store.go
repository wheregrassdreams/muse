package imagecache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var validKey = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Store struct{ dir string }

func New(cacheRoot string) (*Store, error) {
	if cacheRoot == "" {
		return nil, errors.New("imagecache: cache root is required")
	}
	return &Store{dir: filepath.Join(cacheRoot, "images")}, nil
}

func (s *Store) Read(key string) ([]byte, error) {
	if !validKey.MatchString(key) {
		return nil, errors.New("imagecache: invalid cache key")
	}
	return os.ReadFile(filepath.Join(s.dir, key))
}

func (s *Store) Write(key string, data []byte) error {
	if !validKey.MatchString(key) {
		return errors.New("imagecache: invalid cache key")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("imagecache: create cache directory: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, ".image-*.tmp")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer os.Remove(path)
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
	return os.Rename(path, filepath.Join(s.dir, key))
}

func (s *Store) Delete(key string) error {
	if !validKey.MatchString(key) {
		return errors.New("imagecache: invalid cache key")
	}
	if err := os.Remove(filepath.Join(s.dir, key)); errors.Is(err, os.ErrNotExist) {
		return nil
	} else {
		return err
	}
}
