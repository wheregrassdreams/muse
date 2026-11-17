package editor

// TestFullSyntaxFixture opens the all-syntax document the way a reader
// does and checks that each construct reaches the display: styled runs
// keep their style, block syntax is hidden away from the caret, and the
// blocks with native painters (math, Mermaid, HTML, tables) produce neutral
// render models for their component adapters.

import (
	"os"
	"strings"
	"testing"

	syntax "muse/markdown/syntax"
)

func TestFullSyntaxFixture(t *testing.T) {
	data, err := os.ReadFile(editorTestdataPath("all-syntax.md"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	src := string(data)
	e := shownEditor(t, src)

	if e.Source() != src {
		t.Fatalf("source changed on open: %d bytes, want %d", len(e.Source()), len(src))
	}

	// Every heading of the fixture is found, including both setext forms.
	wantHeadings := []Heading{
		{Level: 1, Text: "Markdown 全语法测试"},
		{Level: 2, Text: "二级标题"},
		{Level: 3, Text: "三级标题"},
		{Level: 4, Text: "四级标题"},
		{Level: 5, Text: "五级标题"},
		{Level: 6, Text: "六级标题"},
		{Level: 1, Text: "Setext 一级标题"},
		{Level: 2, Text: "Setext 二级标题"},
	}
	got := e.Headings()
	if len(got) < len(wantHeadings) {
		t.Fatalf("headings = %+v", got)
	}
	for i, w := range wantHeadings {
		if got[i].Level != w.Level || got[i].Text != w.Text {
			t.Errorf("heading %d = level %d %q, want level %d %q",
				i, got[i].Level, got[i].Text, w.Level, w.Text)
		}
	}

	var text []string
	kinds := map[leafKind]int{}
	for _, l := range e.leaves {
		kinds[l.kind]++
	}
	for _, u := range e.units() {
		if u.isText() && u.view != nil {
			text = append(text, styledText(e, u.view.disp))
		}
	}
	all := strings.Join(text, "\n")

	for _, want := range []string{
		"«b:粗体»",
		"«i:斜体»",
		"«b,i:粗斜体»",
		"«s:删除线»",
		"«mk:高亮»",
		"«code:行内代码»",
		"«a:链接 ↗»",
		"«b:注意：»",
		"«b:重点»",
		"«s:删除：»",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("display is missing %s", want)
		}
	}
	for _, raw := range []string{"**粗体**", "==高亮==", "~~删除线~~", "`行内代码`"} {
		if strings.Contains(all, raw) {
			t.Errorf("display still shows the markup %q", raw)
		}
	}
	// A closing tag of the HTML section on a line of its own is a block of
	// its own to CommonMark; it stays hidden, as its opening tag does.
	for _, l := range e.leaves {
		if l.kind == kHTML && l.view != nil && strings.Contains(l.view.disp.text, "</details>") {
			t.Errorf("display still shows the closing tag %q", l.view.disp.text)
		}
	}

	// The blocks with their own drawing produce it.
	if kinds[kTable] == 0 {
		t.Errorf("no table leaf: %v", kinds)
	}
	if kinds[kMath] == 0 {
		t.Errorf("no math leaf: %v", kinds)
	}
	if kinds[kHTML] == 0 {
		t.Errorf("no HTML leaf: %v", kinds)
	}
	var diagram bool
	for _, l := range e.leaves {
		v := l.view
		if v == nil && isMermaid(l) {
			// Long documents virtualize offscreen blocks. Build this block's
			// display view directly so the fixture still checks diagram projection.
			v = e.viewWith(l, e.opts.MaxWidth, false, syntax.IncrementalAlignNone, -1)
		}
		if v != nil && v.diagramModel != nil {
			diagram = true
		}
	}
	if !diagram {
		t.Error("the Mermaid block did not produce a neutral diagram model")
	}

	// The footnote is a container, not body text.
	var footnote bool
	for _, l := range e.leaves {
		for _, c := range l.conts {
			if c.kind == cFootnote {
				footnote = true
			}
		}
	}
	if !footnote {
		t.Error("the footnote definition is not a footnote container")
	}
}
