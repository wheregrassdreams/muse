// Package types contains stable document value types shared by document,
// vault indexing, and search adapters. It has no UI or storage dependencies.
package types

type NoteType string

const DefaultNoteType NoteType = "note"

// Metadata preserves authored frontmatter values. Search owns how these values
// are interpreted in queries; this package only defines their shape.
type Metadata struct {
	Type      NoteType
	CreatedAt string
	UpdatedAt string
	Aliases   []string
	Tags      []string
}

func (m Metadata) Clone() Metadata {
	m.Aliases = append([]string(nil), m.Aliases...)
	m.Tags = append([]string(nil), m.Tags...)
	return m
}
