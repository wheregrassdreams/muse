package vault

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"unicode"
)

// IconPreferences validates and caches per-vault icon choices while delegating
// durable storage to the injected filesystem port.
type IconPreferences struct {
	repository NavigationIconRepository
	mu         sync.Mutex
	byVault    map[string]map[string]string
}

func NewIconPreferences(repository NavigationIconRepository) (*IconPreferences, error) {
	if repository == nil {
		return nil, errors.New("application/vault: navigation icon repository is required")
	}
	return &IconPreferences{repository: repository, byVault: make(map[string]map[string]string)}, nil
}

func (s *IconPreferences) Get(vaultRoot, itemPath string) string {
	if s == nil {
		return ""
	}
	item, err := normalizeIconItemPath(itemPath)
	if err != nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.loadLocked(vaultRoot)
	if err != nil {
		return ""
	}
	return items[item]
}

func (s *IconPreferences) Set(vaultRoot, itemPath, iconID string) error {
	if s == nil {
		return errors.New("application/vault: nil icon preferences")
	}
	item, err := normalizeIconItemPath(itemPath)
	if err != nil {
		return err
	}
	iconID = strings.TrimSpace(iconID)
	if err := validateIconID(iconID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.loadLocked(vaultRoot)
	if err != nil {
		return err
	}
	next := cloneIconMap(items)
	next[item] = iconID
	if err := s.repository.SaveVault(vaultRoot, next); err != nil {
		return err
	}
	s.byVault[vaultRoot] = next
	return nil
}

func (s *IconPreferences) Delete(vaultRoot, itemPath string) error {
	if s == nil {
		return errors.New("application/vault: nil icon preferences")
	}
	item, err := normalizeIconItemPath(itemPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.loadLocked(vaultRoot)
	if err != nil {
		return err
	}
	if _, ok := items[item]; !ok {
		return nil
	}
	next := cloneIconMap(items)
	delete(next, item)
	if err := s.repository.SaveVault(vaultRoot, next); err != nil {
		return err
	}
	s.byVault[vaultRoot] = next
	return nil
}

func (s *IconPreferences) loadLocked(root string) (map[string]string, error) {
	if items, ok := s.byVault[root]; ok {
		return items, nil
	}
	items, err := s.repository.LoadVault(root)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = make(map[string]string)
	}
	for item, iconID := range items {
		normalized, err := normalizeIconItemPath(item)
		if err != nil || normalized != item {
			return nil, fmt.Errorf("application/vault: invalid stored item path %q", item)
		}
		if err := validateIconID(iconID); err != nil {
			return nil, fmt.Errorf("application/vault: invalid stored icon for %q: %w", item, err)
		}
	}
	s.byVault[root] = items
	return items, nil
}

func normalizeIconItemPath(value string) (string, error) {
	if value == "" || strings.IndexByte(value, 0) >= 0 {
		return "", errors.New("application/vault: empty or invalid item path")
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if path.IsAbs(value) || isDrivePrefix(value) {
		return "", fmt.Errorf("application/vault: item path must be vault-relative: %q", value)
	}
	normalized := path.Clean(value)
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("application/vault: item path escapes vault root: %q", value)
	}
	return normalized, nil
}

func validateIconID(iconID string) error {
	if iconID == "" || len(iconID) > 256 {
		return errors.New("application/vault: empty or oversized icon ID")
	}
	for _, r := range iconID {
		if unicode.IsControl(r) {
			return errors.New("application/vault: icon ID contains a control character")
		}
	}
	return nil
}

func isDrivePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func cloneIconMap(items map[string]string) map[string]string {
	next := make(map[string]string, len(items)+1)
	for key, value := range items {
		next[key] = value
	}
	return next
}
