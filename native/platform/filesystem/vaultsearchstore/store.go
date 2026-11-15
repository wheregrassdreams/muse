package vaultsearchstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"muse/search"
)

type Store struct{ cacheRoot string }

func New(cacheRoot string) (*Store, error) {
	if cacheRoot == "" {
		return nil, errors.New("vaultsearchstore: cache root is required")
	}
	return &Store{cacheRoot: cacheRoot}, nil
}

func (s *Store) Open(root string) (*search.Index, error) {
	canonical, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(canonical); resolveErr == nil {
		canonical = resolved
	}
	key := sha256.Sum256([]byte(canonical))
	dir := filepath.Join(s.cacheRoot, "vaultsearch", hex.EncodeToString(key[:]))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return search.Open(dir)
}
