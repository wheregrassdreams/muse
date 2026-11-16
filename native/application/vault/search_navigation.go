package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"muse/document"
	"muse/search"
)

var ErrInvalidSearchHitPath = errors.New("application/vault: search hit path is outside the vault")

type SearchHitStatus uint8

const (
	SearchHitCurrent SearchHitStatus = iota + 1
	SearchHitNeedsRefresh
)

// IsCurrentSearchResult reports whether a completion still belongs to the
// query currently shown by this vault session. Consumers can use this before
// applying an asynchronously delivered result to their own presentation state.
func (s *Session) IsCurrentSearchResult(result Result) bool {
	if s == nil || result.Kind != QueryCompleted {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.closed &&
		result.Token == s.snapshot.Search.Token &&
		result.Query == s.snapshot.Search.Query &&
		result.Literal == s.snapshot.Search.Literal
}

// ValidateSearchHit checks that an indexed hit still refers to the same
// document revision. A stale hit refreshes the index and returns
// SearchHitNeedsRefresh so the caller can rerun the current query. A removed
// file returns its filesystem error without changing the index.
func (s *Session) ValidateSearchHit(repository document.Repository, hit search.Hit) (string, SearchHitStatus, error) {
	if s == nil || repository == nil {
		return "", 0, ErrSessionNotReady
	}
	full, err := s.searchHitPath(hit.Path)
	if err != nil {
		return "", 0, err
	}
	note, err := repository.Read(full)
	if err == nil && string(note.Revision) == hit.Revision {
		return full, SearchHitCurrent, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return full, 0, err
	}
	if refreshErr := s.Refresh(full); refreshErr != nil {
		return full, 0, refreshErr
	}
	return full, SearchHitNeedsRefresh, nil
}

// ValidateSearchNavigation applies a search hit to the active document when
// its revision still matches, or verifies the durable note before opening it.
// If the indexed revision is stale, it refreshes that note and tells the caller
// to rerun the current query before navigating.
func (s *Session) ValidateSearchNavigation(repository document.Repository, hit search.Hit, currentPath string, currentRevision document.Revision) (SearchHitStatus, error) {
	if s == nil {
		return 0, ErrSessionNotReady
	}
	full, err := s.searchHitPath(hit.Path)
	if err != nil {
		return 0, err
	}
	if filepath.Clean(full) == filepath.Clean(currentPath) && currentRevision == document.Revision(hit.Revision) {
		return SearchHitCurrent, nil
	}
	_, status, err := s.ValidateSearchHit(repository, hit)
	return status, err
}

func (s *Session) searchHitPath(path string) (string, error) {
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
