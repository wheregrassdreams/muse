package vault

import (
	"errors"
	"path/filepath"
	"strings"
)

var ErrInvalidSearchHitPath = errors.New("application/vault: search hit path is outside the vault")

// ResolveSearchHitPath confines an indexed relative path to this vault. Query
// freshness and document revision decisions belong to application/search.
func (s *Session) ResolveSearchHitPath(path string) (string, error) {
	if s == nil || s.root == "" {
		return "", ErrSessionNotReady
	}
	rel := filepath.Clean(filepath.FromSlash(path))
	if rel == "." || filepath.IsAbs(rel) {
		return "", ErrInvalidSearchHitPath
	}
	full := filepath.Clean(filepath.Join(s.root, rel))
	within, err := filepath.Rel(s.root, full)
	if err != nil || within == ".." || filepath.IsAbs(within) || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", ErrInvalidSearchHitPath
	}
	return full, nil
}
