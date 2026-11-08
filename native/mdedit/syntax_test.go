package mdedit

import (
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// shownEditor lays src out in an editor without the focus, where no
// syntax shows.
func shownEditor(t *testing.T, src string) *Editor {
	t.Helper()
	e := New(src, Options{FontSize: 16})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			e.View(c).Grow(1)
		})
	}, 1000, 700)
	tt.Frame()
	tt.Frame()
	return e
}

// shown returns how each text unit of src displays, a table's cells each:
// its text, with the runs of bold, italic, struck, highlighted, code and
// linked text as «b:…», «i:…», «s:…», «mk:…», «code:…» and «a:…», and
// objects as [obj].
func shown(t *testing.T, src string) []string {
	t.Helper()
	e := shownEditor(t, src)
	var out []string
	for _, u := range e.units() {
		if u.isText() && u.view != nil {
			// A blank line's leaf shows nothing: it is a caret position, not
			// text.
			if u.kind == kPhantom {
				continue
			}
			out = append(out, styledText(e, u.view.disp))
		}
	}
	return out
}

func styledText(e *Editor, d display) string {
	var b strings.Builder
	open := ""
	for _, p := range d.pieces {
		var tags []string
		sp := p.span
		if sp.Weight >= 600 {
			tags = append(tags, "b")
		}
		if sp.Italic {
			tags = append(tags, "i")
		}
		if sp.Strikethrough {
			tags = append(tags, "s")
		}
		if sp.Background == e.lk.markBg && e.lk.markBg != (ui.Color{}) {
			tags = append(tags, "mk")
		}
		if sp.Font == e.lk.mono {
			tags = append(tags, "code")
		}
		if sp.Color == e.lk.accent {
			tags = append(tags, "a")
		}
		tag := strings.Join(tags, ",")
		if tag != open {
			if open != "" {
				b.WriteString("»")
			}
			if tag != "" {
				b.WriteString("«" + tag + ":")
			}
			open = tag
		}
		if p.obj != nil {
			b.WriteString("[obj]")
		} else {
			b.WriteString(p.text)
		}
	}
	if open != "" {
		b.WriteString("»")
	}
	return b.String()
}

func TestEmphasisBesideCJKPunctuation(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"**注意：**这是**重点**内容。", "«b:注意：»这是«b:重点»内容。"},
		{"**（括号）**中文和中文**（括号）**中文", "«b:（括号）»中文和中文«b:（括号）»中文"},
		{"**“引号”**里面", "«b:“引号”»里面"},
		{"**English:**中文和**中文。**English", "«b:English:»中文和«b:中文。»English"},
		{"**粗体！**后面**省略……**后面", "«b:粗体！»后面«b:省略……»后面"},
		{"*斜体：*中文", "«i:斜体：»中文"},
		{"~~删除：~~后文", "«s:删除：»后文"},
		{"> **提示：**引用中的**加粗**文字", "«b:提示：»引用中的«b:加粗»文字"},
		// What CommonMark reads right stays as it is.
		{"“我说**是**”", "“我说«b:是»”"},
		{"**“名称”**：**值**", "«b:“名称”»：«b:值»"},
		{"这是**加粗**文字，还有*斜体*。", "这是«b:加粗»文字，还有«i:斜体»。"},
		{"a**a«**a", "a**a«**a"},
		{"snake_case_word 和 中文_斜体_中文", "snake_case_word 和 中文_斜体_中文"},
	} {
		got := shown(t, c.src)
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%q shows %q, want %q", c.src, got, c.want)
		}
	}
}

func TestCJKFriendlyLeavesTheSourceAlone(t *testing.T) {
	src := "## **注意：**事项\n\n```\n**注意：**中文\n```\n\n`**注意：**中文` ![**图：**说明](x.png)\n"
	e := shownEditor(t, src)
	if got := e.Headings(); len(got) != 1 || got[0].Text != "注意：事项" {
		t.Fatalf("headings = %+v", got)
	}
	var code *leaf
	for _, l := range e.leaves {
		if l.kind == kCode {
			code = l
		}
	}
	if code == nil || len(code.lines) != 1 || code.lines[0].text != "**注意：**中文" {
		t.Fatalf("code lines = %+v", code)
	}
	last := blocks(e)[len(blocks(e))-1]
	if got := styledText(e, last.view.disp); got != "«code:**注意：**中文» [obj]" {
		t.Fatalf("last paragraph shows %q", got)
	}
	for _, pc := range last.view.disp.pieces {
		if pc.obj != nil && pc.obj.alt != "图：说明" {
			t.Fatalf("image alt = %q", pc.obj.alt)
		}
	}
}

func TestBoldShortcutBesideCJKPunctuation(t *testing.T) {
	h := newHarness(t, "注意：这是内容")
	h.e.Select(0, len("注意："))
	h.frame()
	h.key(ui.Cmd, ui.KeyB)
	h.want("**注意：**这是内容")
	h.caretAt(len(h.e.Source()))
	if got := styledText(h.e, h.e.leaves[0].view.disp); got != "«b:注意：»这是内容" {
		t.Fatalf("shows %q", got)
	}
	h.e.Select(2, 2+len("注意："))
	h.frame()
	h.key(ui.Cmd, ui.KeyB)
	h.want("注意：这是内容")
}

func TestBareAutolinksShowAsLinks(t *testing.T) {
	got := shown(t, "访问 https://example.com 或 www.example.com 或 <https://a.com/?q=\\*>")
	want := "访问 «a:https://example.com» 或 «a:www.example.com» 或 «a:https://a.com/?q=\\*»"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("shows %q, want %q", got, want)
	}
}

func TestCodeSpanOverLinesHidesPrefixes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"> `跨行\n> 代码` 在引用里", "«code:跨行\n代码» 在引用里"},
		{"- `跨行\n  代码` 在列表里", "«code:跨行\n代码» 在列表里"},
		{"- a\n - b\n  - c\n   - d\n    - e", "d\n- e"},
		{"para\n    indented continuation", "para\nindented continuation"},
	} {
		got := shown(t, c.src)
		if got[len(got)-1] != c.want {
			t.Errorf("%q shows %q, want %q", c.src, got, c.want)
		}
	}
}

func TestEscapedPipeInCellCode(t *testing.T) {
	got := shown(t, "| 代码 | 说明 |\n|---|---|\n| `a\\|b` | 管道 |")
	if len(got) != 4 || got[2] != "«code:a|b»" {
		t.Fatalf("cells show %q", got)
	}
}

func TestTabIndentedCodeInContainers(t *testing.T) {
	for _, c := range []struct{ src, line string }{
		{"- foo\n\n\t\tbar\n", "bar"},
		{">\t\tfoo\n", "foo"},
		{"-\t\tfoo\n", "foo"},
	} {
		e := shownEditor(t, c.src)
		var code *leaf
		for _, l := range e.leaves {
			if l.kind == kCode {
				code = l
			}
		}
		if code == nil || len(code.lines) != 1 || code.lines[0].text != c.line || e.src[code.lines[0].off:code.lines[0].off+len(c.line)] != c.line {
			t.Errorf("%q: code = %+v", c.src, code)
		}
	}
}

func TestPendingMarkersShowAsText(t *testing.T) {
	e := shownEditor(t, "段落\n\n-\n\n>\n\n1.")
	for _, l := range blocks(e)[1:] {
		if !l.pending || len(l.conts) != 0 || len(l.firstOf) != 0 || l.x != e.leaves[0].x {
			t.Errorf("leaf at %d: pending %v, conts %v, x %.1f", l.start, l.pending, l.conts, l.x)
		}
	}
	h := newHarness(t, "")
	h.typ("-")
	if l := h.e.leaves[0]; !l.pending || len(l.conts) != 0 {
		t.Fatalf("- alone makes an item: %+v", l.conts)
	}
	h.typ(" a")
	h.want("- a")
	if l := h.e.leaves[0]; l.pending || !l.opensItem() {
		t.Fatal("- a is not an item")
	}
}

func TestTypingAtTheEndOfAHeadingWithClosingHashes(t *testing.T) {
	h := newHarness(t, "# Title #")
	h.key(ui.Cmd, ui.KeyRight)
	h.typ("x")
	h.want("# Titlex #")
}

func TestHTMLColorsAndImages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.png")
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(0, 0, color.Black)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	src := `文字 <span style="color:red; font-weight:bold">红</span> <font color="#00f">蓝</font> 图 <img src="` + path + `" width="40"> 完` +
		"\n\n" + `<img src="` + path + `" alt="图" style="zoom:50%;" />`
	e := shownEditor(t, src)
	p := e.leaves[0].view.disp
	if strings.Contains(p.text, "<") {
		t.Fatalf("tags show: %q", p.text)
	}
	var red, blue bool
	for _, pc := range p.pieces {
		switch pc.text {
		case "红":
			red = pc.span.Color == ui.RGB(255, 0, 0) && pc.span.Weight >= 600
		case "蓝":
			blue = pc.span.Color == ui.RGB(0, 0, 255)
		}
		if pc.obj != nil && (pc.obj.w != 40 || pc.obj.h != 20) {
			t.Errorf("inline image is %.0f×%.0f, want 40×20", pc.obj.w, pc.obj.h)
		}
	}
	if !red || !blue {
		t.Errorf("colors: red %v, blue %v", red, blue)
	}
	block := blocks(e)[1].view.block
	if block == nil || block.w != 100 || block.h != 50 {
		t.Fatalf("zoomed image = %+v, want 100×50", block)
	}
}

func TestInlineImageTagRevealsAtCaret(t *testing.T) {
	h := newHarness(t, `前 <img src="missing.png" width="20"> 后`)
	h.caretAt(0)
	if h.shows("<img") {
		t.Fatal("the tag shows away from the caret")
	}
	h.caretAt(strings.Index(h.e.Source(), "src"))
	if !h.shows(`<img src="missing.png" width="20">`) {
		t.Fatalf("the tag is hidden at the caret: %q", h.e.leaves[0].view.disp.text)
	}
}

func TestParserSourceFollowsEdits(t *testing.T) {
	h := newHarness(t, "---\ntitle: 测试\n---\n\n**注意：**这是**重点**内容。\n\n> **提示：**引用\n")
	rng := rand.New(rand.NewPCG(1, 2))
	pieces := []string{"**", "：", "中", "\n", "（", "）", " ", "a", "~~", "\n\n", "> ", "*"}
	swapped := 0
	for i := 0; i < 400; i++ {
		src := h.e.Source()
		at := rng.IntN(len(src) + 1)
		for at > 0 && at < len(src) && !utf8.RuneStart(src[at]) {
			at--
		}
		if rng.IntN(3) == 0 && at < len(src) {
			_, n := utf8.DecodeRuneInString(src[at:])
			h.e.Replace(at, at+n, "")
		} else {
			h.e.Replace(at, at, pieces[rng.IntN(len(pieces))])
		}
		h.e.sync()
		if want := parserText(maskFrontMatter(h.e.src, h.e.fm)); h.e.doc.Source() != want {
			t.Fatalf("after edit %d of %q:\nparsed %q\nwant   %q", i, h.e.src, h.e.doc.Source(), want)
		} else if want != h.e.src {
			swapped++
		}
	}
	if swapped < 100 {
		t.Fatalf("only %d edits left characters to swap", swapped)
	}
}

func TestEscapedBracketsAreNotMath(t *testing.T) {
	for _, c := range []struct {
		src  string
		want []string
	}{
		{"\\[注] 说明\n\n后文段落", []string{"[注] 说明", "后文段落"}},
		{"\\[1\\] 参考文献", []string{"[1] 参考文献"}},
		{"- \\[注] 列表里\n- 第二项", []string{"[注] 列表里", "第二项"}},
		{"> \\[注] 引用里\n\n后文", []string{"[注] 引用里", "后文"}},
		{"公式 \\[a+b\\] 行内", []string{"公式 [obj] 行内"}},
	} {
		if got := shown(t, c.src); strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%q shows %q, want %q", c.src, got, c.want)
		}
	}
	for _, src := range []string{"\\[\nx^2\n\\]", "\\[ x^2 \\]", "\\[ x \\in [0, 1]\n\\]"} {
		e := shownEditor(t, src)
		if len(e.leaves) != 1 || e.leaves[0].kind != kMath {
			t.Errorf("%q is not a block of math: %+v", src, e.leaves[0])
		}
	}
}
