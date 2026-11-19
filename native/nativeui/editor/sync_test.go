package editor

import (
	"fmt"
	"strings"
	"testing"

	"muse/editor/source"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
)

func fullParserMask(text string) string {
	projection := source.New(exts, source.ParserText)
	fm := source.FrontmatterEnd(text)
	var images []syntax.IncrementalInline
	if fm == 0 {
		images, _ = syntax.LargeDataImageOnlySource(text)
	}
	projection.Set(text, fm, images)
	return projection.MaskedSource()
}

// docShape is the parse of a document as text, for comparing an editor
// that edited its way to a source with one that parsed the source whole.
func docShape(d source.SyntaxView) string {
	var b strings.Builder
	var walk func(i, depth int)
	walk = func(i, depth int) {
		for c := range d.Children(i) {
			blk := d.Block(c)
			fmt.Fprintf(&b, "%d:%d:%d:%d:%d:%t:%s\n", depth, blk.Kind, blk.Start, blk.End, blk.Level, blk.Fenced, blk.Language())
			walk(c, depth+1)
		}
	}
	walk(0, 0)
	return b.String()
}

// leavesShape is the leaves of an editor as text, for comparing an editor
// that edited its way to a source with one that parsed the source whole.
// Every field the display and the caret depend on is in it; the implicit
// links back to a table are not, as the cells are printed where they are.
func leavesShape(e *Editor) string {
	var b strings.Builder
	var one func(l *leaf)
	one = func(l *leaf) {
		fmt.Fprintf(&b, "%d:%d:%d:%d:%d:%d:%d:%t:%t:%t:%t:%t:%t:%d:%d:%d:%q:",
			l.kind, l.id, l.start, l.end, l.lineStart, l.syntaxStart, l.level,
			l.fenced, l.pending, l.setext, l.frontMatter, l.whole, l.table != nil,
			l.fenceEndOffset(), l.infoStartOffset(), l.infoEndOffset(), l.language())
		for _, c := range l.conts {
			fmt.Fprintf(&b, "c%d,%d,%d,%t,%d,%d,%t,%d,%d,%d,%q;",
				c.kind, c.id, c.start, c.ordered, c.num, c.marker, c.tight, c.depth, c.task, c.taskAt, c.label)
		}
		fmt.Fprintf(&b, "f%v l", l.firstOf)
		for _, ln := range l.codeLines() {
			fmt.Fprintf(&b, "%d,%q;", ln.off, ln.text)
		}
		if t := l.tab; t != nil {
			fmt.Fprintf(&b, "t%v,%v", t.align, t.delim)
			for _, row := range t.rows {
				b.WriteString("[")
				for _, cell := range row {
					one(cell)
				}
				b.WriteString("]")
			}
		}
		b.WriteString("\n")
	}
	for _, l := range e.leaves {
		one(l)
	}
	return b.String()
}

// TestIncrementalSyncMatchesFreshParse edits a document step by step and
// checks after each step that the editor holds the same parse and the same
// leaves a fresh one makes of the same source. Some steps edit one line,
// some add and remove lines, and one edits the front matter, so both the
// incremental path and the full reparse path run.
func TestIncrementalSyncMatchesFreshParse(t *testing.T) {
	src := "---\ntitle: 测试\n---\n\n# 标题\n\n第一段 **注意：**内容。\n\n- [ ] 待办\n- 项目二\n\n```go\nfmt.Println(1)\n```\n\n> 引用\n"
	e := New(src, Options{})
	steps := []struct {
		find, repl string
	}{
		{"第一段", "第一段新"},
		{"项目二", "项目 2"},
		{"[ ] 待办", "[x] 待办"},
		{"fmt.Println(1)", "fmt.Println(2)"},
		{"# 标题", "## 标题"},
		{"标题", "新标题"},
		{"引用", "引用后"},
		{"- [x]", "- [ ]"},
		{"title: 测试", "title: 新"},
	}
	for i, st := range steps {
		at := strings.Index(e.Source(), st.find)
		if at < 0 {
			t.Fatalf("step %d: %q is not in %q", i, st.find, e.Source())
		}
		e.Replace(at, at+len(st.find), st.repl)
		if e.pending != nil || e.pendingMulti {
			t.Fatalf("step %d: the edit was not given to the parser", i)
		}
		if e.p.Source() != e.Source() {
			t.Fatalf("step %d: parser source is stale", i)
		}
		if want := fullParserMask(e.Source()); e.p.MaskedSource() != want {
			t.Fatalf("step %d: mask = %q, want %q", i, e.p.MaskedSource(), want)
		}
		if e.p.Parser().Source() != e.p.MaskedSource() {
			t.Fatalf("step %d: parser source = %q, mask = %q", i, e.p.Parser().Source(), e.p.MaskedSource())
		}
		fresh := New(e.Source(), Options{})
		if got, want := docShape(e.p.Parser()), docShape(fresh.p.Parser()); got != want {
			t.Fatalf("step %d: parse differs after %q:\n%s\nwant:\n%s", i, st.find, got, want)
		}
		if got, want := fmt.Sprint(e.Headings()), fmt.Sprint(fresh.Headings()); got != want {
			t.Fatalf("step %d: headings = %s, want %s", i, got, want)
		}
		if got, want := leavesShape(e), leavesShape(fresh); got != want {
			t.Fatalf("step %d: leaves differ after %q:\n%s\nwant:\n%s", i, st.find, got, want)
		}
	}
}

// TestSyncAfterUndoKeepsMask checks that a source put back by undo or redo
// is given to the parser whole, even when an edit was still waiting for
// it.
func TestSyncAfterUndoKeepsMask(t *testing.T) {
	e := New("abcdef\n", Options{})
	e.Replace(0, 0, "X")
	e.begin(stepEdit)
	e.replace(2, 3, "Y")
	e.commit(e.Caret(), e.Caret())
	e.Undo()
	if want := fullParserMask(e.Source()); e.p.MaskedSource() != want {
		t.Fatalf("after undo: mask = %q, want %q", e.p.MaskedSource(), want)
	}
	if got := e.p.Parser().Source(); got != e.p.MaskedSource() {
		t.Fatalf("after undo: parser source = %q, mask = %q", got, e.p.MaskedSource())
	}
	e.Redo()
	if want := fullParserMask(e.Source()); e.p.MaskedSource() != want {
		t.Fatalf("after redo: mask = %q, want %q", e.p.MaskedSource(), want)
	}
	if got := e.p.Parser().Source(); got != e.p.MaskedSource() {
		t.Fatalf("after redo: parser source = %q, mask = %q", got, e.p.MaskedSource())
	}
}

// TestCarriedViewFollowsSameLengthEdit changes text without moving any
// offset after it: the leaf has the same span and block, but its view must
// not be the one before the edit.
func TestCarriedViewFollowsSameLengthEdit(t *testing.T) {
	e := New("abc\n\n| 甲 | 乙 |\n| --- | --- |\n| ab | cd |\n", Options{})
	e.colW = 720
	e.layoutIfNeeded()
	// Change one byte, so the shape of the document, and every offset in
	// it, stays exactly where it was.
	e.begin(stepEdit)
	e.replace(0, 1, "x")
	e.commit(1, 1)
	e.layoutIfNeeded()
	if got := e.leaves[0].view.disp.text; !strings.HasPrefix(got, "xbc") {
		t.Fatalf("paragraph displays %q, want it to start with %q", got, "xbc")
	}
	at := strings.Index(e.Source(), "ab |")
	if at < 0 {
		t.Fatal("the table cell is gone")
	}
	e.begin(stepEdit)
	e.replace(at, at+1, "z")
	e.commit(at+1, at+1)
	e.layoutIfNeeded()
	var table *leaf
	for _, l := range e.leaves {
		if l.kind == kTable {
			table = l
		}
	}
	if table == nil {
		t.Fatal("the table is gone")
	}
	if got := table.tab.rows[1][0].view.disp.text; !strings.HasPrefix(got, "zb") {
		t.Fatalf("table cell displays %q, want it to start with %q", got, "zb")
	}
}

// TestSyncRebuildsMaskAfterSeveralEdits exercises the path where one
// committed step replaces more than one range: the parser cannot be given
// one range, so the mask is built again from the whole source.
func TestSyncRebuildsMaskAfterSeveralEdits(t *testing.T) {
	e := New("abc def\n", Options{})
	e.begin(stepEdit)
	e.replace(0, 0, "[")
	e.replace(len(e.Source()), len(e.Source()), "]")
	if !e.pendingMulti {
		t.Fatal("two ranges were not marked for a full reparse")
	}
	e.commit(1, 1)
	if want := fullParserMask(e.Source()); e.p.MaskedSource() != want {
		t.Fatalf("mask = %q, want %q", e.p.MaskedSource(), want)
	}
	if got := e.p.Parser().Source(); got != e.p.MaskedSource() {
		t.Fatalf("parser source = %q, mask = %q", got, e.p.MaskedSource())
	}
}
