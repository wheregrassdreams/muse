package render

import (
	editorrender "github.com/wheregrassdreams/muse-editor/markdown/render"
	"github.com/wheregrassdreams/muse-editor/markdown/syntax"
)

// Backend-neutral Markdown projection models are owned by muse-editor.
type Document = editorrender.Document
type Block = editorrender.Block
type Item = editorrender.Item
type Cell = editorrender.Cell

func Project(doc *syntax.Doc) *Document           { return editorrender.Project(doc) }
func ProjectBlocks(blocks []syntax.Block) []Block { return editorrender.ProjectBlocks(blocks) }
