package files

import (
	"path/filepath"
	"sync"

	vaultfiles "muse/vault/files"
)

// CatalogSession is the read capability needed to project catalog entries
// for the active vault. The concrete lifecycle remains owned by application/vault.
type CatalogSession interface {
	Children(directory string) []vaultfiles.Entry
	Entry(path string) (vaultfiles.Entry, bool)
}

// CatalogVersion identifies one ready projection of a vault's file inventory.
// An unready inventory is never cached as an empty directory.
type CatalogVersion struct {
	Ready    bool
	Revision uint64
}

// DirectoryCatalog memoizes directory projections for one active vault
// session and catalog revision. The catalog remains the source of truth;
// explicit invalidation is available for local mutations ahead of watcher
// reconciliation.
type DirectoryCatalog struct {
	mu         sync.RWMutex
	session    CatalogSession
	version    CatalogVersion
	entries    map[string][]vaultfiles.Entry
	generation uint64
}

func NewDirectoryCatalog() *DirectoryCatalog {
	return &DirectoryCatalog{entries: make(map[string][]vaultfiles.Entry)}
}

func (c *DirectoryCatalog) Children(session CatalogSession, directory string, version CatalogVersion) []vaultfiles.Entry {
	if c == nil || session == nil {
		return nil
	}
	key := filepath.ToSlash(filepath.Clean(directory))
	c.mu.Lock()
	if c.session != session || c.version != version {
		c.session = session
		c.version = version
		c.entries = make(map[string][]vaultfiles.Entry)
		c.generation++
	}
	if !version.Ready {
		c.mu.Unlock()
		return nil
	}
	if entries, ok := c.entries[key]; ok {
		out := append([]vaultfiles.Entry(nil), entries...)
		c.mu.Unlock()
		return out
	}
	generation := c.generation
	c.mu.Unlock()

	entries := session.Children(key)
	c.mu.Lock()
	if c.generation == generation && c.session == session && c.version == version {
		c.entries[key] = append([]vaultfiles.Entry(nil), entries...)
	}
	c.mu.Unlock()
	return entries
}

func (c *DirectoryCatalog) Invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.generation++
	c.entries = make(map[string][]vaultfiles.Entry)
	c.mu.Unlock()
}

func (c *DirectoryCatalog) CloseSession(session CatalogSession) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if session == nil || c.session == session {
		c.session = nil
		c.version = CatalogVersion{}
		c.generation++
		c.entries = make(map[string][]vaultfiles.Entry)
	}
	c.mu.Unlock()
}
