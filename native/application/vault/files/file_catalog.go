package files

import (
	"path/filepath"

	vaultfiles "muse/vault/files"
)

// CatalogChildren returns the cached immediate children for an absolute
// directory in the active vault. Path containment and conversion to the
// vault-relative catalog key belong to the vault application boundary.
func (u *FileUseCases) CatalogChildren(session CatalogSession, catalog *DirectoryCatalog, directory string) []vaultfiles.Entry {
	if u == nil || session == nil || catalog == nil || !u.OwnsPath(directory) {
		return nil
	}
	relative, err := filepath.Rel(filepath.Clean(u.root), filepath.Clean(directory))
	if err != nil || filepath.IsAbs(relative) || hasParentPrefix(relative) {
		return nil
	}
	if relative == "." {
		relative = ""
	}
	return catalog.Children(session, filepath.ToSlash(relative))
}

// CatalogEntry resolves an absolute vault path through the current session's
// catalog and rejects paths outside the active vault.
func (u *FileUseCases) CatalogEntry(session CatalogSession, path string) (vaultfiles.Entry, bool) {
	if u == nil || session == nil || !u.OwnsPath(path) {
		return vaultfiles.Entry{}, false
	}
	relative, err := filepath.Rel(filepath.Clean(u.root), filepath.Clean(path))
	if err != nil || relative == "." || filepath.IsAbs(relative) || hasParentPrefix(relative) {
		return vaultfiles.Entry{}, false
	}
	return session.Entry(filepath.ToSlash(relative))
}

// IsCatalogDirectory reports the cached kind for a vault path. It does not
// fall back to filesystem probing, so callers can use it for selection policy
// without introducing synchronous I/O in the UI path.
func (u *FileUseCases) IsCatalogDirectory(session CatalogSession, path string) bool {
	entry, ok := u.CatalogEntry(session, path)
	return ok && entry.Kind == vaultfiles.EntryDirectory
}

// CatalogRelativePath translates an owned absolute path to the relative
// catalog key used by vault-scoped preferences.
func (u *FileUseCases) CatalogRelativePath(path string) (string, bool) {
	if u == nil || !u.OwnsPath(path) {
		return "", false
	}
	relative, err := filepath.Rel(filepath.Clean(u.root), filepath.Clean(path))
	if err != nil || relative == "." || filepath.IsAbs(relative) || hasParentPrefix(relative) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

// PathKind first consults the active vault catalog, then falls back to the
// filesystem capability for dropped paths that have not entered the catalog.
func (u *FileUseCases) PathKind(session CatalogSession, path string) (vaultfiles.EntryKind, error) {
	if entry, ok := u.CatalogEntry(session, path); ok {
		return entry.Kind, nil
	}
	if u == nil || u.operations == nil {
		return 0, ErrFileOperationsUnavailable
	}
	return u.operations.Kind(path)
}
