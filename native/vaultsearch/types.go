// Package vaultsearch provides a persistent full-text index for vault documents.
package vaultsearch

import "context"

// Document is a disk snapshot or a query-time overlay. ID is stable across
// path changes; Revision identifies the caller's snapshot version.
type Document struct {
	ID, Path, Title, Source, Revision string
	Type, CreatedAt, UpdatedAt        string
	Aliases, Tags                     []string
}

// Hit describes a matched document. Start and End are UTF-8 byte offsets into
// the original Source; Search hits may have no exact source range.
type Hit struct {
	Path, Revision, Snippet string
	Start, End              int
	Score                   float64
}

// Index is a persistent Tantivy-backed index. Instances are created by Open.
type Index struct{ state *indexState }

// Open opens or creates an index under dir.
func Open(dir string) (*Index, error) { return openIndex(dir) }

// Reconcile makes the disk index match docs, skipping unchanged revisions.
func (i *Index) Reconcile(ctx context.Context, docs []Document) error {
	return i.reconcile(ctx, docs)
}

// NeedsReconcile reports whether docs differ from the durable snapshot without
// changing the index. It is useful for choosing a full-rebuild execution path.
func (i *Index) NeedsReconcile(docs []Document) (bool, error) {
	return i.needsReconcile(docs)
}

// Apply applies disk snapshot upserts and deletions and makes them searchable.
func (i *Index) Apply(ctx context.Context, upserts []Document, deletes []string) error {
	return i.apply(ctx, upserts, deletes)
}

// Search finds relevant documents. Query text is treated as literal input,
// never as Tantivy query syntax. Overlays shadow persisted documents by ID.
func (i *Index) Search(ctx context.Context, query string, limit int, overlays ...Document) ([]Hit, error) {
	return i.search(ctx, query, limit, overlays...)
}

// Literal finds Unicode case-insensitive substring matches and source spans.
// Overlays shadow persisted documents by ID and are never persisted.
func (i *Index) Literal(ctx context.Context, query string, limit int, overlays ...Document) ([]Hit, error) {
	return i.literal(ctx, query, limit, overlays...)
}

// Close releases the index and its native resources.
func (i *Index) Close() error { return i.close() }
