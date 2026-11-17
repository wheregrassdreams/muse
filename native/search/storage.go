package search

import (
	"io/fs"
)

// Persistence is the filesystem capability consumed by the persistent search
// index. The search package owns manifest encoding and recovery policy; the
// platform adapter owns these concrete file operations.
//
// Root returns the absolute directory used as Tantivy's on-disk index root.
// Tantivy currently accepts a filesystem path rather than a virtual directory,
// so its internal file I/O remains path-backed even though Go-side persistence
// is routed through this port.
type Persistence interface {
	Root() string
	MkdirAll(relative string, perm fs.FileMode) error
	Stat(relative string) (fs.FileInfo, error)
	ReadFile(relative string) ([]byte, error)
	WriteAtomic(relative string, data []byte, perm fs.FileMode) error
	WriteAndSync(relative string, data []byte, perm fs.FileMode) error
	Rename(oldRelative, newRelative string) error
	RemoveAndSync(relative string) error
	TryLock(relative string) (Lock, bool, error)
}

// Lock is held for the lifetime of an open search index.
type Lock interface {
	Unlock() error
}
