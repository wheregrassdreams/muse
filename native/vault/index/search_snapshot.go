package index

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"muse/document"
	"muse/document/types"
	"muse/search"
)

// SearchSnapshot is the immutable, narrow projection needed by a full-text
// search index. It deliberately omits headings, blocks, and links so search
// consumers do not clone the semantic graph just to mirror searchable fields.
type SearchSnapshot struct {
	Path, Title, Source string
	Revision            document.Revision
	Metadata            types.Metadata
}

// SearchSnapshot returns the fields needed to reconcile an external search
// index, in stable path order. Source is shared as an immutable Go string;
// mutable metadata slices are copied for the caller.
func (i *Index) SearchSnapshot() []SearchSnapshot {
	i.mu.RLock()
	defer i.mu.RUnlock()
	paths := i.pathsLocked()
	out := make([]SearchSnapshot, 0, len(paths))
	for _, path := range paths {
		doc := i.docs[path]
		out = append(out, SearchSnapshot{
			Path:     doc.Path,
			Title:    doc.Title,
			Source:   doc.Source,
			Revision: doc.Revision,
			Metadata: doc.Metadata(),
		})
	}
	return out
}

// SearchDocuments implements search.Corpus without exposing index storage or
// the richer link/heading graph. Path-derived IDs preserve the current vault
// identity semantics; a move is represented as delete plus insert.
func (i *Index) SearchDocuments() []search.Document {
	i.mu.RLock()
	defer i.mu.RUnlock()
	paths := i.pathsLocked()
	out := make([]search.Document, 0, len(paths))
	for _, path := range paths {
		doc := i.docs[path]
		cleanPath := filepath.ToSlash(filepath.Clean(doc.Path))
		id := sha256.Sum256([]byte(cleanPath))
		out = append(out, search.Document{
			ID: hex.EncodeToString(id[:]), Path: doc.Path, Title: doc.Title,
			Source: doc.Source, Revision: string(doc.Revision), Metadata: doc.Metadata(),
		})
	}
	return out
}

var _ search.Corpus = (*Index)(nil)
