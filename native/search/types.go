// Package search owns the query language, metadata filters, ranking, and
// persistent full-text execution contracts for notes.
package search

import (
	"context"
	"errors"

	"muse/document/types"
	"muse/search/fields"
	"muse/search/rank"
)

// Document is a disk snapshot or a query-time overlay. ID is stable across
// path changes; Revision identifies the caller's snapshot version.
type Document struct {
	ID, Path, Title, Source, Revision string
	Metadata                          types.Metadata
}

// Hit describes a matched document. Start and End are UTF-8 byte offsets into
// the original Source; Search hits may have no exact source range.
type Hit struct {
	Path, Revision, Snippet string
	Start, End              int
	Score                   float64
}

// Index is a persistent Tantivy-backed index. Instances are created by
// OpenWithStorage.
type Index struct{ state *indexState }

// Corpus is a search-owned port implemented by vault indexes. It exposes an
// immutable projection and keeps the vault's discovery/link graph private.
type Corpus interface {
	SearchDocuments() []Document
}

// QueryOptions injects query semantics and ranking policies during app
// composition. Nil registries select the built-in metadata fields and no
// extra rankers.
type QueryOptions struct {
	Fields  *fields.Registry
	Rankers *rank.Registry
}

// OpenWithStorage opens or creates an index using the supplied persistence
// capability. Search owns query and recovery policy; the platform adapter
// supplies the concrete filesystem operations.
func OpenWithStorage(storage Persistence) (*Index, error) {
	if storage == nil {
		return nil, errors.New("search: persistence capability is required")
	}
	return openIndex(storage)
}

// Reconcile makes the disk index match docs, skipping unchanged revisions.
func (i *Index) Reconcile(ctx context.Context, docs []Document) error {
	return i.reconcile(ctx, docs)
}

// ReconcileCorpus reconciles from a module that implements the search-owned
// corpus port, such as vault/index.Index.
func (i *Index) ReconcileCorpus(ctx context.Context, corpus Corpus) error {
	if corpus == nil {
		return errors.New("search: corpus is required")
	}
	return i.Reconcile(ctx, corpus.SearchDocuments())
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

// Search finds relevant documents. Plain terms are treated as literal input;
// recognized typed filters use the search query language. Overlays shadow
// persisted documents by ID.
func (i *Index) Search(ctx context.Context, query string, limit int, overlays ...Document) ([]Hit, error) {
	return i.SearchQuery(ctx, query, limit, QueryOptions{}, overlays...)
}

// Literal finds Unicode case-insensitive substring matches and source spans.
// Overlays shadow persisted documents by ID and are never persisted.
func (i *Index) Literal(ctx context.Context, query string, limit int, overlays ...Document) ([]Hit, error) {
	return i.literal(ctx, query, limit, overlays...)
}

// Close releases the index and its native resources.
func (i *Index) Close() error { return i.close() }
