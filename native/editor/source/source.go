// Package source synchronizes editable Markdown source revisions with the
// syntax parser while preserving source byte offsets.
package source

import (
	"iter"
	"strings"

	"muse/markdown/syntax"
)

type Comment struct{ Start, End int }

// SyntaxView is the read-only Markdown projection synchronized with a source
// revision. It deliberately excludes parser mutation methods so callers must
// apply edits through Document, which keeps source masking and parser state in
// lockstep. Returned inline slices and child iterators are borrowed read-only
// parser data and are valid only until the next Document mutation.
type SyntaxView interface {
	Source() string
	Footnote(label string) (int, bool)
	Len() int
	Block(index int) syntax.IncrementalBlock
	Children(index int) iter.Seq[int]
	Inlines(index int) []syntax.IncrementalInline
	LineCount(index int) int
	Line(index, line int) (string, int)
}

type Document struct {
	doc        *syntax.IncrementalDocument
	extensions syntax.IncrementalExtensions
	source     string
	mask       []byte
	identity   bool
	comments   []Comment
	frontEnd   int
	imageOnly  []syntax.IncrementalInline
	transform  func(string) string
}

func New(extensions syntax.IncrementalExtensions, transform func(string) string) *Document {
	return &Document{doc: syntax.NewIncrementalDocument(extensions), extensions: extensions, transform: transform}
}

func (d *Document) Clear() {
	if d == nil {
		return
	}
	d.doc = syntax.NewIncrementalDocument(d.extensions)
	d.source, d.mask = "", nil
	d.identity, d.frontEnd = false, 0
	d.comments, d.imageOnly = nil, nil
}

// Set rebuilds the parser snapshot from source. frontEnd is the source byte
// offset after nonempty frontmatter. imageOnly is an optional precomputed
// single-line projection for the editor's large data-image fast path.
func (d *Document) Set(source string, frontEnd int, imageOnly []syntax.IncrementalInline) {
	if d.doc == nil {
		d.doc = syntax.NewIncrementalDocument(syntax.IncrementalDefault)
	}
	d.source = source
	d.frontEnd = frontEnd
	d.comments = CommentsFor(source)
	d.imageOnly = append(d.imageOnly[:0], imageOnly...)
	masked := source
	if len(imageOnly) == 0 {
		masked = d.transformText(blankComments(maskFrontMatter(source, frontEnd), d.comments))
	}
	if masked == source {
		d.mask, d.identity = nil, true
	} else {
		d.mask, d.identity = nil, false
	}
	if len(imageOnly) > 0 {
		d.doc.SetSingleLine(masked)
	} else {
		d.doc.Set(masked)
	}
}

func (d *Document) transformText(source string) string {
	if d.transform != nil {
		return d.transform(source)
	}
	return source
}

func (d *Document) Source() string {
	if d == nil {
		return ""
	}
	return d.source
}

// FrontmatterEnd reports the byte offset after a nonempty YAML frontmatter
// block at the beginning of source. Empty delimiter pairs remain Markdown
// rules for the editor's normal syntax view.
func FrontmatterEnd(src string) int {
	if !strings.HasPrefix(src, "---\n") && !strings.HasPrefix(src, "---\r\n") {
		return 0
	}
	for off := strings.IndexByte(src, '\n') + 1; off < len(src); {
		end := lineEndAt(src, off)
		line := strings.TrimRight(src[off:end], " \t\r")
		if line == "---" || line == "..." {
			if off == 4 || off == 5 {
				return 0
			}
			return end
		}
		off = end + 1
	}
	return 0
}
func (d *Document) FrontmatterEnd() int {
	if d == nil {
		return 0
	}
	return d.frontEnd
}
func (d *Document) IsIdentityMask() bool { return d != nil && d.identity }
func (d *Document) Comments() []Comment {
	if d == nil {
		return nil
	}
	return append([]Comment(nil), d.comments...)
}
func (d *Document) ImageInlines() []syntax.IncrementalInline {
	if d == nil {
		return nil
	}
	return append([]syntax.IncrementalInline(nil), d.imageOnly...)
}
func (d *Document) Parser() SyntaxView {
	if d == nil || d.doc == nil {
		return nil
	}
	return parserView{document: d.doc}
}

// parserView hides Set and Edit while preserving the parser's zero-copy read
// operations for the editor and presentation adapters.
type parserView struct{ document *syntax.IncrementalDocument }

var _ SyntaxView = parserView{}

func (v parserView) Source() string { return v.document.Source() }
func (v parserView) Footnote(label string) (int, bool) {
	return v.document.Footnote(label)
}
func (v parserView) Len() int { return v.document.Len() }
func (v parserView) Block(index int) syntax.IncrementalBlock {
	return *v.document.Block(index)
}
func (v parserView) Children(index int) iter.Seq[int] {
	return v.document.Children(index)
}
func (v parserView) Inlines(index int) []syntax.IncrementalInline {
	return v.document.Inlines(index)
}
func (v parserView) LineCount(index int) int { return v.document.LineCount(index) }
func (v parserView) Line(index, line int) (string, int) {
	return v.document.Line(index, line)
}
func (d *Document) MaskedSource() string {
	if d == nil {
		return ""
	}
	if d.doc != nil {
		return d.doc.Source()
	}
	if d.identity {
		return d.source
	}
	return string(d.mask)
}

// Edit incrementally applies one source edit. It returns false when masking
// or syntax context makes a local update unsafe; callers must then Set.
func (d *Document) Edit(source string, from, to int, replacement string) bool {
	if d == nil || d.doc == nil || from <= 0 || to < from || to > len(d.source) {
		return false
	}
	if d.frontEnd > 0 && (from <= d.frontEnd || to <= d.frontEnd) {
		return false
	}
	if touchesComment(d.comments, from, to) || !commentsStay(d.source, source, from, to, len(replacement)) {
		return false
	}
	if d.identity {
		end := to + len(source) - len(d.source)
		if end < from || end > len(source) {
			return false
		}
		startLine, endLine := lineStartAt(source, from), lineEndAt(source, end)
		if line := source[startLine:endLine]; d.transformText(line) != line {
			return false
		}
		d.doc.Edit(from, to, replacement)
		d.source = source
		d.imageOnly = nil
		return true
	}
	delta := len(replacement) - (to - from)
	for i := range d.comments {
		if c := &d.comments[i]; c.Start >= to {
			c.Start += delta
			c.End += delta
		}
	}
	mask := d.mask
	if mask == nil {
		mask = []byte(d.doc.Source())
	}
	next, mFrom, mTo, repl, ok := editMask(mask, source, from, to, d.frontEnd, d.comments, d.transformText)
	if !ok {
		for i := range d.comments {
			if c := &d.comments[i]; c.Start >= to {
				c.Start -= delta
				c.End -= delta
			}
		}
		return false
	}
	d.mask = next
	d.doc.Edit(mFrom, mTo, repl)
	d.source = source
	d.imageOnly = nil
	return true
}

// ApplyEdit applies a replacement to the document's current source revision.
// It is intended for ordered editor transaction deltas, whose later ranges
// are expressed against the source produced by earlier deltas. False means a
// local incremental update is unsafe and the caller should rebuild with Set.
func (d *Document) ApplyEdit(from, to int, replacement string) bool {
	if d == nil || from < 0 || to < from || to > len(d.source) {
		return false
	}
	next := d.source[:from] + replacement + d.source[to:]
	return d.Edit(next, from, to, replacement)
}

func lineStartAt(src string, off int) int {
	off = max(0, min(off, len(src)))
	return strings.LastIndexByte(src[:off], '\n') + 1
}
func lineEndAt(src string, off int) int {
	off = max(0, min(off, len(src)))
	if i := strings.IndexByte(src[off:], '\n'); i >= 0 {
		return off + i
	}
	return len(src)
}

func maskFrontMatter(src string, end int) string {
	if end <= 0 {
		return src
	}
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < end && i < len(src); i++ {
		if src[i] == '\n' || src[i] == '\r' {
			b.WriteByte(src[i])
		} else {
			b.WriteByte(' ')
		}
	}
	if end < len(src) {
		b.WriteString(src[end:])
	}
	return b.String()
}
