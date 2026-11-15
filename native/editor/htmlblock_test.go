package mdedit

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// htmlLeaf returns the editor's HTML block leaf.
func htmlLeaf(t *testing.T, e *Editor) *leaf {
	t.Helper()
	for _, l := range e.leaves {
		if l.kind == kHTML && l.view != nil {
			return l
		}
	}
	t.Fatal("no HTML block")
	return nil
}

// htmlShown returns how the editor draws its HTML block.
func htmlShown(t *testing.T, src string) string {
	t.Helper()
	e := shownEditor(t, src)
	return styledText(e, htmlLeaf(t, e).view.disp)
}

func TestHTMLBlockReadsAsHTML(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// A details block shows its summary in bold, its tags hidden.
		{"<details>\n<summary>点我</summary>\n正文\n</details>\n", "«b:点我»\n正文"},
		// A block tag starts a line, and the newlines of the source in it
		// are spaces, as HTML reads them.
		{"<div>\nline one\nline two\n</div>\n", "line one line two"},
		{"<div>\n  <h3>标题</h3>\n  <p>段落</p>\n</div>\n", "«b:标题»\n段落"},
		// <br> breaks a line, and two in a row keep the empty line.
		{"<div>a<br>b<br><br>c</div>\n", "a\nb\n\nc"},
		// The inline tags style their text.
		{"<div><b>粗</b>和<i>斜</i>和<code>x</code></div>\n", "«b:粗»和«i:斜»和«code:x»"},
		// <pre> keeps its whitespace and its lines.
		{"<pre>\nline  1\n  line 2\n</pre>\n", "«code:line  1\n  line 2»"},
		// Script, style and comments show nothing, over lines too.
		{"<div><script>bad()</script>visible<!-- hidden --></div>\n", "visible"},
		{"<div>\nvisible\n<!-- hidden\nstill hidden -->\n</div>\n", "visible"},
		{"<div><script>\nbad()\n</script>visible</div>\n", "visible"},
	} {
		if got := htmlShown(t, c.src); got != c.want {
			t.Errorf("%q shows %q, want %q", c.src, got, c.want)
		}
	}
}

func TestHTMLBlockStylesText(t *testing.T) {
	e := shownEditor(t, `<div style="color: #c00; font-weight: bold">红</div>`+"\n")
	d := htmlLeaf(t, e).view.disp
	if len(d.pieces) != 1 || d.pieces[0].text != "红" {
		t.Fatalf("pieces %+v", d.pieces)
	}
	if got := d.pieces[0].span; got.Color != ui.RGBA(204, 0, 0, 1) || got.Weight < 600 {
		t.Fatalf("span %+v", got)
	}
	// A quotation's text is muted.
	e = shownEditor(t, "<blockquote>引用</blockquote>\n")
	if got := htmlLeaf(t, e).view.disp.pieces[0].span.Color; got == e.lk.text {
		t.Fatal("a quotation's text is not muted")
	}
}

func TestHTMLBlockDrawsItsImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.png")
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	img.Set(0, 0, color.Black)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	e := shownEditor(t, `<div><img src="`+path+`" width="40"></div>`+"\n")
	if obj := htmlLeaf(t, e).view.block; obj != nil {
		if obj.w != 40 || obj.h != 20 {
			t.Fatalf("image is %.0f×%.0f, want 40×20", obj.w, obj.h)
		}
		return
	}
	d := htmlLeaf(t, e).view.disp
	if len(d.pieces) != 1 || d.pieces[0].obj == nil || d.pieces[0].obj.w != 40 {
		t.Fatalf("pieces %+v", d.pieces)
	}
}

func TestHTMLBlockShowsItsSourceAtTheCaret(t *testing.T) {
	h := newHarness(t, "<div>\n文字\n</div>\n\n后文\n")
	h.caretAt(len(h.e.Source()) - 1) // in 后文
	if got := htmlLeaf(t, h.e).view.disp.text; got != "文字" {
		t.Fatalf("away from the caret it shows %q", got)
	}
	h.caretAt(6) // in 文字
	if got := htmlLeaf(t, h.e).view.disp.text; got != "<div>\n文字\n</div>" {
		t.Fatalf("at the caret it shows %q", got)
	}
	h.caretAt(0) // at the tag
	if got := htmlLeaf(t, h.e).view.disp.text; got != "<div>\n文字\n</div>" {
		t.Fatalf("at the tag it shows %q", got)
	}
}

// closingLeaf returns the editor's HTML leaf that holds a closing tag on
// its own: a block of its own when a blank line separates the tag from the
// block it closes, as CommonMark reads it.
func closingLeaf(t *testing.T, e *Editor) *leaf {
	t.Helper()
	for _, l := range e.leaves {
		if l.kind == kHTML && strings.HasPrefix(strings.TrimSpace(e.src[l.start:l.end]), "</") {
			return l
		}
	}
	t.Fatal("no closing tag block")
	return nil
}

func TestHTMLBlockAloneClosingTag(t *testing.T) {
	const src = "<details>\n文字\n\n</details>\n\n后文\n"
	e := shownEditor(t, src)
	l := closingLeaf(t, e)
	if got := styledText(e, l.view.disp); got != "" {
		t.Fatalf("away from the caret the closing tag shows %q", got)
	}
	// It is not left as a gap in the document.
	if l.view.layout != nil || l.view.h != 0 || l.h != 0 {
		t.Fatalf("the closing tag takes %.0f points", l.h)
	}

	// The caret can still reach it, and then it shows its source.
	h := newHarness(t, src)
	l = closingLeaf(t, h.e)
	h.caretAt(l.start + 1)
	l = closingLeaf(t, h.e)
	if got := styledText(h.e, l.view.disp); got != "</details>" {
		t.Fatalf("at the caret the closing tag shows %q", got)
	}
	if l.h == 0 {
		t.Fatal("at the caret the closing tag takes no room")
	}
}
