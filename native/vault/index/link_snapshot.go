package index

import (
	"muse/vault/links"
)

// LinkSnapshot projects immutable parsed structure into the narrow input
// contract consumed by vault/links. It intentionally omits source text,
// search fields, and index storage details.
func (i *Index) LinkSnapshot() []links.Document {
	i.mu.RLock()
	defer i.mu.RUnlock()
	paths := i.pathsLocked()
	out := make([]links.Document, 0, len(paths))
	for _, path := range paths {
		doc := i.docs[path]
		out = append(out, links.Document{
			Path: doc.Path, Title: doc.Title,
			Aliases:  append([]string(nil), doc.Aliases...),
			Headings: append([]links.Heading(nil), doc.Headings...),
			Blocks:   append([]links.BlockID(nil), doc.Blocks...),
			Links:    append([]links.Link(nil), doc.Links...),
		})
	}
	return out
}

// LinkDocument returns the structure projection for one indexed note.
func (i *Index) LinkDocument(path string) (links.Document, bool) {
	doc, ok := i.Document(path)
	if !ok {
		return links.Document{}, false
	}
	return links.Document{
		Path: doc.Path, Title: doc.Title,
		Aliases:  append([]string(nil), doc.Aliases...),
		Headings: append([]links.Heading(nil), doc.Headings...),
		Blocks:   append([]links.BlockID(nil), doc.Blocks...),
		Links:    append([]links.Link(nil), doc.Links...),
	}, true
}
