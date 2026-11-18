// Package source preserves Muse's historical import path for the independent
// Markdown source model. Source synchronization and masking live in
// muse-editor/markdown/source.
package source

import model "github.com/wheregrassdreams/muse-editor/markdown/source"

type Comment = model.Comment
type Document = model.Document
type ImageInlineCache = model.ImageInlineCache
type SyntaxView = model.SyntaxView

var (
	CommentsFor              = model.CommentsFor
	FindComment              = model.FindComment
	FrontmatterEnd           = model.FrontmatterEnd
	OffsetInNormalizedSource = model.OffsetInNormalizedSource
	ParserText               = model.ParserText
)

var New = model.New
