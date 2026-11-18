// Package findreplace preserves Muse's historical import path for the
// backend-neutral text search and replacement model.
package findreplace

import model "github.com/wheregrassdreams/muse-editor/text/findreplace"

const MaxMatches = model.MaxMatches

type Match = model.Match

var (
	Find         = model.Find
	ReplaceAll   = model.ReplaceAll
	SnippetParts = model.SnippetParts
)
