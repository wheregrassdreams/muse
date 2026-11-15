// Package navicons persists per-vault navigation item icon choices.
package navicons

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"muse/cachepath"
)

const (
	fileMode = 0o600
	dirMode  = 0o700
)

// Store persists icon IDs in a cache file isolated by canonical vault root.
// The vault itself is never modified.
type Store struct {
	dir     string
	mu      sync.Mutex
	byVault map[string]map[string]string
}

// New creates the store under cachepath.Root()/navicons.
func New() (*Store, error) {
	root, err := cachepath.Root()
	if err != nil {
		return nil, fmt.Errorf("navicons: resolve cache root: %w", err)
	}
	dir := filepath.Join(root, "navicons")
	return &Store{dir: dir, byVault: make(map[string]map[string]string)}, nil
}

// Get returns the icon ID assigned to itemPath in vaultRoot, or "" if there
// is no assignment or either path is invalid.
func (s *Store) Get(vaultRoot, itemPath string) string {
	if s == nil {
		return ""
	}
	root, err := canonicalVaultRoot(vaultRoot)
	if err != nil {
		return ""
	}
	item, err := normalizeItemPath(itemPath)
	if err != nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	vaultKey := vaultKey(root)
	items, err := s.loadVaultLocked(vaultKey)
	if err != nil {
		return ""
	}
	return items[item]
}

// Set assigns a non-empty icon ID to a vault-relative item path. The update
// is atomically persisted before it becomes visible from this Store.
func (s *Store) Set(vaultRoot, itemPath, iconID string) error {
	if s == nil {
		return errors.New("navicons: nil store")
	}
	root, err := canonicalVaultRoot(vaultRoot)
	if err != nil {
		return err
	}
	item, err := normalizeItemPath(itemPath)
	if err != nil {
		return err
	}
	iconID = strings.TrimSpace(iconID)
	if err := validateIconID(iconID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	vaultKey := vaultKey(root)
	items, err := s.loadVaultLocked(vaultKey)
	if err != nil {
		return err
	}
	next := clone(items)
	next[item] = iconID
	if err := s.writeVaultLocked(vaultKey, next); err != nil {
		return err
	}
	s.byVault[vaultKey] = next
	return nil
}

// Delete removes an icon assignment. Removing a missing assignment is a
// no-op; deleting an item does not alter the vault file or its contents.
func (s *Store) Delete(vaultRoot, itemPath string) error {
	if s == nil {
		return errors.New("navicons: nil store")
	}
	root, err := canonicalVaultRoot(vaultRoot)
	if err != nil {
		return err
	}
	item, err := normalizeItemPath(itemPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	vaultKey := vaultKey(root)
	items, err := s.loadVaultLocked(vaultKey)
	if err != nil {
		return err
	}
	if _, ok := items[item]; !ok {
		return nil
	}
	next := clone(items)
	delete(next, item)
	if err := s.writeVaultLocked(vaultKey, next); err != nil {
		return err
	}
	s.byVault[vaultKey] = next
	return nil
}

func (s *Store) loadVaultLocked(key string) (map[string]string, error) {
	if items, ok := s.byVault[key]; ok {
		return items, nil
	}
	file := filepath.Join(s.dir, key+".json")
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		items := make(map[string]string)
		s.byVault[key] = items
		return items, nil
	}
	if err != nil {
		return nil, fmt.Errorf("navicons: read store: %w", err)
	}
	items := make(map[string]string)
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("navicons: decode store: %w", err)
	}
	for item, iconID := range items {
		normalized, err := normalizeItemPath(item)
		if err != nil || normalized != item {
			return nil, fmt.Errorf("navicons: invalid item path in store: %q", item)
		}
		if err := validateIconID(iconID); err != nil {
			return nil, fmt.Errorf("navicons: invalid icon ID in store for %q: %w", item, err)
		}
	}
	s.byVault[key] = items
	return items, nil
}

func (s *Store) writeVaultLocked(key string, items map[string]string) error {
	if err := os.MkdirAll(s.dir, dirMode); err != nil {
		return fmt.Errorf("navicons: create cache directory: %w", err)
	}
	data, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("navicons: encode store: %w", err)
	}
	file := filepath.Join(s.dir, key+".json")
	tmp, err := os.CreateTemp(s.dir, ".navicons-*.tmp")
	if err != nil {
		return fmt.Errorf("navicons: create temporary store: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(fileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("navicons: set cache permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("navicons: write temporary store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("navicons: sync temporary store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("navicons: close temporary store: %w", err)
	}
	if err := os.Rename(tmpPath, file); err != nil {
		return fmt.Errorf("navicons: publish store: %w", err)
	}
	return nil
}

func canonicalVaultRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("navicons: empty vault root")
	}
	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("navicons: resolve vault root: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
}

func normalizeItemPath(itemPath string) (string, error) {
	if itemPath == "" || strings.IndexByte(itemPath, 0) >= 0 {
		return "", errors.New("navicons: empty or invalid item path")
	}
	itemPath = strings.ReplaceAll(itemPath, "\\", "/")
	if path.IsAbs(itemPath) || isDrivePrefix(itemPath) {
		return "", fmt.Errorf("navicons: item path must be vault-relative: %q", itemPath)
	}
	normalized := path.Clean(itemPath)
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("navicons: item path escapes vault root: %q", itemPath)
	}
	return normalized, nil
}

func isDrivePrefix(s string) bool {
	return len(s) >= 2 && ((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z')) && s[1] == ':'
}

func validateIconID(iconID string) error {
	if iconID == "" {
		return errors.New("navicons: empty icon ID")
	}
	if len(iconID) > 256 {
		return errors.New("navicons: icon ID is too long")
	}
	for _, r := range iconID {
		if unicode.IsControl(r) {
			return errors.New("navicons: icon ID contains a control character")
		}
	}
	return nil
}

func vaultKey(root string) string {
	digest := sha256.Sum256([]byte(root))
	return hex.EncodeToString(digest[:])
}

func clone(items map[string]string) map[string]string {
	next := make(map[string]string, len(items)+1)
	for key, value := range items {
		next[key] = value
	}
	return next
}
