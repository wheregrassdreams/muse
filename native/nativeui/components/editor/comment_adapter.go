package editor

import "muse/editor/source"

// cmt is the source module's immutable comment range, consumed by the
// presenter to reveal or hide the corresponding source bytes.
type cmt = source.Comment

func commentRegions(sourceText string) []cmt { return source.CommentsFor(sourceText) }

func commentAt(comments []cmt, offset int) (cmt, bool) {
	return source.FindComment(comments, offset)
}
