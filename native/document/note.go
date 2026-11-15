// Package document owns durable note identity, source, metadata, and revision
// semantics. It deliberately has no dependency on editor state or rendering.
package document

import (
	"crypto/sha256"
	"encoding/hex"

	"muse/document/frontmatter"
	"muse/document/types"
)

type Revision string

// Note is the durable path-identified note record. This repository does not
// persist a separate stable note ID: moving a note changes its identity and is
// represented as delete plus create. Path may be blank for an untitled note.
// Source is an immutable Go string, so copying a Note does not copy a large
// Markdown buffer.
type Note struct {
	Path     string
	Source   string
	Metadata types.Metadata
	Revision Revision
}

func NewNote(path, source string, revision Revision) Note {
	metadata := frontmatter.Parse(source).Metadata()
	return Note{Path: path, Source: source, Metadata: metadata, Revision: revision}
}

func (n Note) DirtyComparedTo(saved Revision) bool { return n.Revision != saved }

// RevisionOf computes a disk-conflict fingerprint. Call it at load/save or
// confirmed external-change boundaries, never for each editor transaction.
func RevisionOf(source string) Revision {
	sum := sha256.Sum256([]byte(source))
	return Revision(hex.EncodeToString(sum[:]))
}
