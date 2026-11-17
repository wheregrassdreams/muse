package vault

import (
	"path/filepath"
	"sync"

	vaultfiles "muse/vault/files"
)

// DirectoryCatalog memoizes directory projections for one active vault
// session. The catalog itself remains the source of truth; callers invalidate
// after FilesRefreshed or any successful local mutation.
type DirectoryCatalog struct {
	mu         sync.RWMutex
	session    *Session
	entries    map[string][]vaultfiles.Entry
	generation uint64
}

func NewDirectoryCatalog() *DirectoryCatalog {
	return &DirectoryCatalog{entries: make(map[string][]vaultfiles.Entry)}
}

func (c *DirectoryCatalog) Children(session *Session, directory string) []vaultfiles.Entry {
	if c == nil || session == nil {
		return nil
	}
	key := filepath.ToSlash(filepath.Clean(directory))
	c.mu.RLock()
	if c.session == session {
		if entries, ok := c.entries[key]; ok {
			out := append([]vaultfiles.Entry(nil), entries...)
			c.mu.RUnlock()
			return out
		}
	}
	generation := c.generation
	c.mu.RUnlock()

	entries := session.Children(key)
	c.mu.Lock()
	if c.session != session {
		c.session = session
		c.entries = make(map[string][]vaultfiles.Entry)
		c.generation++
		generation = c.generation
	}
	if c.generation == generation {
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

func (c *DirectoryCatalog) CloseSession(session *Session) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if session == nil || c.session == session {
		c.session = nil
		c.generation++
		c.entries = make(map[string][]vaultfiles.Entry)
	}
	c.mu.Unlock()
}
