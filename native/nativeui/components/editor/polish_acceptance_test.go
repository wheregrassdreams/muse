package editor

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// These acceptance paths use the same playground paragraphs as the reported
// screenshots, independently of each worker's focused regression fixtures.
func playgroundParagraph(t *testing.T, prefix string) string {
	t.Helper()
	b, err := os.ReadFile("../testdata/playground.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, paragraph := range strings.Split(string(b), "\n\n") {
		if strings.HasPrefix(paragraph, prefix) {
			return paragraph
		}
	}
	t.Fatalf("missing playground paragraph %q", prefix)
	return ""
}

func TestPolishAcceptancePlaygroundSelection(t *testing.T) {
	src := playgroundParagraph(t, "这是一段普通正文")
	h := newHarness(t, src+"\n\nend")
	for _, reverse := range []bool{false, true} {
		h.e.setSessionSelection(0, len(src))
		if reverse {
			h.e.setSessionSelection(h.e.Caret(), h.e.Session.Selection().Anchor)
		}
		h.frame()
		for _, syntax := range []string{"**粗体**", "*斜体*", "~~删除线~~", "`行内代码`", "[链接](https://example.com)", "[[双链|带标签]]", "$a^2+b^2=c^2$"} {
			if !strings.Contains(h.e.leaves[0].view.disp.text, syntax) {
				t.Errorf("selected source is hidden: %q in %q", syntax, h.e.leaves[0].view.disp.text)
			}
		}
		if h.e.SelectedText() != src || h.e.Source() != src+"\n\nend" {
			t.Fatal("reveal changed source or selection endpoints")
		}
	}
	h.shot("acceptance-playground-selection")
}

func TestPolishAcceptancePlaygroundGeometry(t *testing.T) {
	h := newHarness(t, playgroundParagraph(t, "一个很长的段落"))
	opts := h.e.Options()
	opts.MaxWidth = 430
	h.e.SetOptions(opts)
	h.frame()
	l := h.e.leaves[0]
	lines := l.view.layout.Lines()
	if len(lines) < 5 {
		t.Fatalf("geometry fixture needs at least five wrapped lines, got %d", len(lines))
	}
	h.e.SetKeymap("vim")
	first := lines[0]
	off := h.e.hitTest(l.x+170, l.y+l.view.ty+first.Rect.Y+first.Rect.H/2)
	h.caretAt(off)
	h.e.ReplaceText(nil, "j")
	h.frame()
	// A horizontal move after the first vertical move must establish a new X.
	h.e.ReplaceText(nil, "h")
	h.frame()
	want, ok := h.e.caretRect(h.e.Caret())
	if !ok {
		t.Fatal("missing origin caret")
	}
	for i := 0; i < 3; i++ {
		before, _ := h.e.caretRect(h.e.Caret())
		h.e.ReplaceText(nil, "j")
		h.frame()
		got, _ := h.e.caretRect(h.e.Caret())
		if got.Y <= before.Y || math.Abs(float64(got.X-want.X)) > 12 {
			t.Fatalf("j %d is not geometric: before=%+v got=%+v targetX=%f", i+1, before, got, want.X)
		}
	}
	h.shot("acceptance-playground-geometric-vim")
}

func TestPolishAcceptanceExpandedTaskClickEditsSource(t *testing.T) {
	src := "- [ ] abc\n\nend"
	h := newHarness(t, src)
	h.e.Select(0, len("- [ ] abc"))
	h.frame()
	r, ok := h.e.caretRect(2)
	if !ok {
		t.Fatal("expanded task marker has no native caret")
	}
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, Clicks: 1, X: r.X + 0.5, Y: r.Y + r.H/2})
	h.e.input(ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, X: r.X + 0.5, Y: r.Y + r.H/2})
	if h.e.Source() != src || h.e.Caret() != 2 {
		t.Fatalf("clicking task source toggled checkbox or moved endpoint: caret=%d source=%q", h.e.Caret(), h.e.Source())
	}
	h.shot("acceptance-expanded-task-source")
}

func TestPolishAcceptancePlaygroundHeadingSelection(t *testing.T) {
	src := "# 各类内容一览\n\n这是一段正文。\n\n## 下一节\n\n结束"
	h := newHarness(t, src)
	h.e.Select(0, len(src))
	h.frame()
	if text := h.e.leaves[0].view.disp.text; !strings.Contains(text, "# 各类内容一览") {
		t.Fatalf("selected heading did not expose its Markdown marker: %q", text)
	}
	if h.e.SelectedText() != src || h.e.Source() != src {
		t.Fatal("heading reveal changed the native source selection")
	}
	h.shot("acceptance-selected-heading-source")
}

func TestPolishAcceptanceVisualSelectionIncludesBothCharacters(t *testing.T) {
	for _, tc := range []struct {
		name, src, start, motion, want string
	}{
		{"CJK single", "单换行在排版里", "在", "", "在"},
		{"CJK left", "单换行在排版里", "在", "h", "行在"},
		{"CJK right", "单换行在排版里", "在", "l", "在排"},
		{"emoji left", "a😀b", "😀", "h", "a😀"},
		{"emoji right", "a😀b", "😀", "l", "😀b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, tc.src)
			h.e.SetKeymap("vim")
			at := strings.Index(tc.src, tc.start)
			h.e.Select(at, at)
			h.e.ReplaceText(nil, "v")
			if tc.motion != "" {
				h.e.ReplaceText(nil, tc.motion)
			}
			h.frame()
			if got := h.e.SelectedText(); got != tc.want {
				t.Fatalf("Visual range excludes a character: got %q want %q", got, tc.want)
			}
			if !h.e.Copy() {
				t.Fatal("Visual copy failed")
			}
			if b, _ := h.clip.Read(transfer.Text); string(b) != tc.want {
				t.Fatalf("Visual copy differs from painted range: got %q want %q", b, tc.want)
			}
			h.shot("acceptance-visual-" + tc.name)
			h.e.ReplaceText(nil, "d")
			want := strings.Replace(tc.src, tc.want, "", 1)
			if h.e.Source() != want {
				t.Fatalf("Visual delete differs from copied range: got %q want %q", h.e.Source(), want)
			}
		})
	}
}

func TestPolishAcceptanceVisualTableRoundTrip(t *testing.T) {
	src := "| header | value |\n| --- | --- |\n| first | one |\n| second | two |\n| third | three |\n| fourth-long-text | four |"
	h := newHarness(t, src)
	table := documentTestTable(t, h.e)
	h.e.SetKeymap("vim")
	at := table.tab.rows[4][0].start + len("fourth-")
	h.e.Select(at, at)
	h.e.ReplaceText(nil, "v")
	h.frame()
	initialText := h.e.SelectedText()
	if initialText != "l" {
		t.Fatalf("initial Visual character = %q, want l", initialText)
	}
	moves := 0
	for ; moves < 100; moves++ {
		h.e.ReplaceText(nil, "h")
		if s := h.e.tableSelection; s != nil && s.cellRegion {
			break
		}
	}
	if moves >= 100 {
		t.Fatal("did not enter previous row structure")
	}
	s := h.e.tableSelection
	if s == nil || !s.cellRegion {
		t.Fatalf("cross-cell Visual motion did not create a cell region: %+v", s)
	}
	for i := 0; i < 20; i++ {
		s = h.e.tableSelection
		if s.focusRow == s.anchorRow && s.focusCol == s.anchorCol {
			return
		}
		key := "h"
		if s.focusCol < s.anchorCol {
			key = "l"
		} else if s.focusRow < s.anchorRow {
			key = "j"
		} else if s.focusRow > s.anchorRow {
			key = "k"
		}
		h.e.ReplaceText(nil, key)
		if s := h.e.tableSelection; s == nil || !s.cellRegion {
			t.Fatalf("reverse motion downgraded the cell region at step %d: %+v", i, s)
		}
	}
	t.Fatalf("reverse motion did not return to the cell anchor: initial text=%q selection=%+v", initialText, h.e.tableSelection)
}

func TestPolishAcceptanceExternalIconWrapsWithLabel(t *testing.T) {
	const src = "prefix [link](https://example.test) trailing text"
	wrapped := false
	for width := float32(80); width <= 155; width += 3 {
		h := newHarness(t, src)
		opts := h.e.Options()
		opts.MaxWidth = width
		opts.DisableSmartTypography = true
		h.e.SetOptions(opts)
		h.caretAt(0)
		h.frame()
		v := h.e.leaves[0].view
		wrapped = wrapped || len(v.layout.Lines()) > 1
		lastLabel, ok := v.disp.srcToDisp(strings.Index(src, "link]") + len("lin"))
		if !ok {
			t.Fatal("link label has no source mapping")
		}
		labelRects := v.layout.SelectionRects(ui.TextInputRange{Start: lastLabel, End: lastLabel + 1})
		icons := 0
		for _, p := range v.disp.pieces {
			if !strings.Contains(p.text, "↗") {
				continue
			}
			icons++
			iconRects := v.layout.SelectionRects(ui.TextInputRange{Start: p.d1 - 1, End: p.d1})
			if len(labelRects) != 1 || len(iconRects) != 1 || math.Abs(float64(labelRects[0].Y-iconRects[0].Y)) > 1 {
				t.Fatalf("external icon orphaned at width %.0f: label=%+v icon=%+v", width, labelRects, iconRects)
			}
		}
		if icons != 1 {
			t.Fatalf("expected one external icon at width %.0f, got %d", width, icons)
		}
	}
	if !wrapped {
		t.Fatal("icon wrapping fixture never exercised a wrapped line")
	}
}

func TestPolishAcceptanceFoldWithVirtualLayout(t *testing.T) {
	src := "# Fold\n\n" + strings.Repeat("正文 paragraph for virtual layout.\n\n", virtualLayoutMin+10) + "# End\n\nvisible"
	h := newHarness(t, src)
	if !h.e.virtualLayout() {
		t.Fatal("fixture did not activate virtual layout")
	}
	top := headingAtSource(t, h.e, "# Fold")
	before := h.e.docH
	clickFoldChevron(h, top)
	if h.e.docH >= before/2 || h.e.Source() != src {
		t.Fatalf("folded virtual document has wrong height/source: before=%f after=%f folded=%v", before, h.e.docH, h.e.isFoldedHeading(top))
	}
	h.e.SelectAll()
	if h.e.SelectedText() != src || !h.e.isFoldedHeading(top) {
		t.Fatal("folded virtual selection omitted hidden source or opened the fold")
	}
	h.e.ScrollToOffset(strings.Index(src, "正文"))
	h.frame()
	if h.e.isFoldedHeading(top) || h.e.isFoldedSourceOffset(h.e.Caret()) {
		t.Fatal("explicit navigation retained hidden focus in virtual document")
	}
}
