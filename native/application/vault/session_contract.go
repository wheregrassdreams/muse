package vault

import (
	"time"

	"muse/search"
	vaultindex "muse/vault/index"
	vaultlinks "muse/vault/links"
	vaultwatch "muse/vault/watch"
)

// IndexPort is the narrow index capability needed by one vault session.
// Implementations own parsing and document projections; the application
// session coordinates lifecycle and never reads files itself.
type IndexPort interface {
	Rebuild() error
	Revision() uint64
	Update(path string) error
	ParseSnapshot(path, source string) (vaultindex.Document, error)
	ApplySnapshot(vaultindex.Document)
	Remove(path string)
	Document(path string) (vaultindex.Document, bool)
	NoteContent(path string) (title, source string, ok bool)
	SearchDocuments() []search.Document
	LinkSnapshot() []vaultlinks.Document
	LinkDocument(path string) (vaultlinks.Document, bool)
}

type IndexFactory func(root string) (IndexPort, error)

// CatalogPort is the per-vault path inventory used by file navigation and
// Quick Open. Its concrete source is selected by the composition root.
type CatalogPort interface {
	Rebuild() error
	Update(path string)
	Remove(path string)
	Revision() uint64
	Paths() []string
	ImagePaths() []string
	FindImage(name string) string
}

type CatalogFactory func(root string) (CatalogPort, error)

type WatcherPort interface {
	Events() <-chan vaultwatch.Event
	WatchFile(path string) error
	Close() error
}

type SessionDependencies struct {
	NewIndex         func(root string) (IndexPort, error)
	NewCatalog       func(root string) (CatalogPort, error)
	NewWatcher       func(root string) (WatcherPort, error)
	SearchReconciler SearchReconciler
}

type SearchSnapshot struct {
	Query   string
	Literal bool
	Token   uint64
	Hits    []search.Hit
	Busy    bool
	Error   string
	Elapsed time.Duration
	Cost    time.Duration
}

// Snapshot is an immutable value projection of a vault session. Slice fields
// are copied by Session.Snapshot, so a UI frame cannot mutate session state.
type Snapshot struct {
	Root            string
	Ready           bool
	IndexRevision   uint64
	FilesRevision   uint64
	SearchAvailable bool
	Search          SearchSnapshot
	LinksOpen       bool
	LinksBusy       bool
	Incoming        []vaultlinks.Link
	Outgoing        []vaultlinks.Link
	Error           string
}

type ResultKind uint8

const (
	SessionReady ResultKind = iota
	QueryStarted
	QueryCompleted
	FilesRefreshed
	ActiveDocumentApplied
	LinksRefreshed
	SessionClosed
)

// Result carries the query token even for non-query updates. Consumers must
// discard QueryCompleted values whose token is no longer current.
type Result struct {
	Kind     ResultKind
	Token    uint64
	Query    string
	Literal  bool
	Changed  []string
	Document *vaultindex.Document
	Err      error
	Elapsed  time.Duration
	Snapshot Snapshot
}
