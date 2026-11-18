package imagecache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"muse/platform/filesystem/atomicfile"
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
	_, err := atomicfile.Write(filepath.Join(s.dir, key), ".image-*.tmp", data, 0o600)
	return err
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
