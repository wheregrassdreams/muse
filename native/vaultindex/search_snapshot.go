package vaultindex

// SearchSnapshot is the immutable, narrow projection needed by a full-text
// search index. It deliberately omits headings, blocks, and links so search
// consumers do not clone the semantic graph just to mirror searchable fields.
type SearchSnapshot struct {
	Path, Title, Source        string
	Type, CreatedAt, UpdatedAt string
	Aliases, Tags              []string
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
			Path:      doc.Path,
			Title:     doc.Title,
			Source:    doc.Source,
			Type:      doc.Type,
			CreatedAt: doc.CreatedAt,
			UpdatedAt: doc.UpdatedAt,
			Aliases:   append([]string(nil), doc.Aliases...),
			Tags:      append([]string(nil), doc.Tags...),
		})
	}
	return out
}
