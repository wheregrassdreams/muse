package naviconstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"muse/platform/filesystem/atomicfile"
)

const fileMode = 0o600
const dirMode = 0o700

type Store struct {
	dir string
	mu  sync.Mutex
}

func New(cacheRoot string) (*Store, error) {
	if cacheRoot == "" {
		return nil, errors.New("naviconstore: cache root is required")
	}
	return &Store{dir: filepath.Join(cacheRoot, "navicons")}, nil
}

func (s *Store) LoadVault(vaultRoot string) (map[string]string, error) {
	key, err := canonicalVaultKey(vaultRoot)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(s.dir, key+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("naviconstore: read: %w", err)
	}
	items := make(map[string]string)
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("naviconstore: decode: %w", err)
	}
	return items, nil
}

func (s *Store) SaveVault(vaultRoot string, icons map[string]string) error {
	key, err := canonicalVaultKey(vaultRoot)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, dirMode); err != nil {
		return fmt.Errorf("naviconstore: create directory: %w", err)
	}
	data, err := json.Marshal(icons)
	if err != nil {
		return fmt.Errorf("naviconstore: encode: %w", err)
	}
	file := filepath.Join(s.dir, key+".json")
	stage, err := atomicfile.Write(file, ".navicons-*.tmp", data, fileMode)
	if err != nil {
		switch stage {
		case atomicfile.CreateTemp:
			return fmt.Errorf("naviconstore: create temporary file: %w", err)
		case atomicfile.RenameFile:
			return fmt.Errorf("naviconstore: publish store: %w", err)
		}
		return err
	}
	return nil
}

func canonicalVaultKey(root string) (string, error) {
	if root == "" {
		return "", errors.New("naviconstore: vault root is required")
	}
	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	hash := sha256.Sum256([]byte(filepath.Clean(abs)))
	return hex.EncodeToString(hash[:]), nil
}
