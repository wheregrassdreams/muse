package editor

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestQuoteContinuationPrefixRevealsOnItsOwnLine(t *testing.T) {
	const src = "> 第一行。\n> 第二行。\n> 第三行。"
	base := newHarness(t, src)
	base.caretAt(len(src))
	base.frame()
	firstText := strings.Index(src, "第一行")
	secondText := strings.Index(src, "第二行")
	thirdText := strings.Index(src, "第三行")
	baseFirst, ok := quoteViewCaret(base.e, base.e.leaves[0], firstText)
	if !ok {
		t.Fatal("normal first line has no caret geometry")
	}
	baseSecond, ok := quoteViewCaret(base.e, base.e.leaves[0], secondText)
	if !ok {
		t.Fatal("normal second line has no caret geometry")
	}
	baseThird, ok := quoteViewCaret(base.e, base.e.leaves[0], thirdText)
	if !ok {
		t.Fatal("normal third line has no caret geometry")
	}

	h := newHarness(t, src)
	secondMarker := lineStartAt(src, secondText) + 1
	h.caretAt(secondMarker)
	h.frame()
	l := h.e.leaves[0]
	if !strings.Contains(l.view.disp.text, "\n> 第二行") {
		t.Fatalf("caret in second-line quote prefix did not reveal it: %q", l.view.disp.text)
	}
	if strings.Contains(l.view.disp.text, "\n> 第三行") {
		t.Fatalf("unrelated third-line prefix was revealed: %q", l.view.disp.text)
	}
	var mapped bool
	for _, p := range l.view.disp.pieces {
		if p.kind == pText && p.marker && p.s0+l.lineStart <= secondMarker && p.s1+l.lineStart > secondMarker {
			mapped = true
		}
	}
	if !mapped {
		t.Fatalf("revealed `>` has no source-backed text piece: %+v", l.view.disp.pieces)
	}
	if _, ok := h.e.caretRect(secondMarker); !ok || !h.e.stopOK(l, secondMarker) {
		t.Fatal("caret in continuation marker has no visible stop")
	}
	first, ok := quoteViewCaret(h.e, l, firstText)
	if !ok {
		t.Fatal("revealing the second line hid first-line geometry")
	}
	second, ok := quoteViewCaret(h.e, l, secondText)
	if !ok {
		t.Fatal("revealed line content has no caret geometry")
	}
	third, ok := quoteViewCaret(h.e, l, thirdText)
	if !ok {
		t.Fatal("unrevealed third line has no caret geometry")
	}
	if math.Abs(float64(first.X-baseFirst.X)) > 1.5 {
		t.Fatalf("unrevealed first line shifted from x=%v to %v (leaf x=%v text=%q pieces=%+v)", baseFirst.X, first.X, l.x, l.view.disp.text, l.view.disp.pieces)
	}
	if math.Abs(float64(second.X-baseSecond.X)) > 8 {
		t.Fatalf("revealed second line moved too far from its content column: base=%v revealed=%v", baseSecond.X, second.X)
	}
	if math.Abs(float64(third.X-baseThird.X)) > 1.5 || third.Y != baseThird.Y {
		t.Fatalf("unrevealed third line shifted from (%v,%v) to (%v,%v)", baseThird.X, baseThird.Y, third.X, third.Y)
	}
	if first.Y != baseFirst.Y || second.Y != baseSecond.Y {
		t.Fatalf("source reveal changed line rows: first %v/%v second %v/%v", baseFirst.Y, first.Y, baseSecond.Y, second.Y)
	}
	h.shot("quote-continuation-prefix-reveal")
}

func TestNestedQuoteContinuationRevealsAllSourceMarkers(t *testing.T) {
	const src = "> 外层第一段。\n> > 内层第一段。\n> > 内层第二段。"
	h := newHarness(t, src)
	second := strings.Index(src, "内层第二段")
	line := lineStartAt(src, second)
	h.caretAt(line + 1)
	h.frame()
	l := h.e.leafAt(line + 1)
	if l == nil || l.view == nil {
		t.Fatal("nested quote marker did not resolve to a rendered leaf")
	}
	if quoteDepth(l) < 2 {
		t.Fatalf("nested quote lost its container depth: %d (%+v)", quoteDepth(l), l.conts)
	}
	if !strings.Contains(l.view.disp.text, "> > 内层第二段") {
		t.Fatalf("nested continuation prefixes were not exposed: %q", l.view.disp.text)
	}
	for _, at := range []int{line, line + 2} {
		if _, ok := h.e.caretRect(at); !ok || !h.e.stopOK(l, at) {
			t.Fatalf("nested marker at %d has no source-backed caret stop", at)
		}
	}
	h.shot("nested-quote-continuation-prefix-reveal")
}

func quoteViewCaret(e *Editor, l *leaf, source int) (ui.Rect, bool) {
	if l == nil || l.view == nil || l.view.layout == nil {
		return ui.Rect{}, false
	}
	d, ok := l.view.disp.srcToDisp(source - l.lineStart)
	if !ok {
		return ui.Rect{}, false
	}
	r := l.view.layout.Caret(d)
	r.X += unitTextX(l, l.view)
	r.Y += l.y + l.view.ty
	return r, true
}

func TestCalloutHeaderPreviewTracksExactSourceSelection(t *testing.T) {
	const src = "> [!NOTE] 提示\n> callout 正文第一行。\n> callout 正文第二行。"
	h := newHarness(t, src)
	var header *leaf
	for _, l := range h.e.leaves {
		if _, _, ok := h.e.calloutHeader(l); ok {
			header = l
		}
	}
	if header == nil {
		t.Fatal("fixture has no callout header leaf")
	}
	_, headerEnd, _ := h.e.calloutHeader(header)
	bodyText := strings.Index(src, "正文第一")
	bodySelectionEnd := bodyText + len("正文")

	// Selecting only body text leaves the semantic header projected.
	h.e.Select(bodyText, bodySelectionEnd)
	if h.e.Session.Selection().Anchor != bodyText || h.e.Session.Selection().Head != bodySelectionEnd {
		t.Fatalf("body-only source selection was rejected: %+v", h.e.Session.Selection())
	}
	h.frame()
	if h.e.calloutHeaderSourceOpen(header) {
		t.Fatal("body-only character selection opened the callout header")
	}
	if strings.Contains(header.view.disp.text, "[!NOTE]") {
		t.Fatalf("body-only selection revealed the header source: %q", header.view.disp.text)
	}
	h.shot("callout-body-only-selection")

	// A Visual-line range includes the full physical header, including `>`.
	h.e.setSessionSelection(header.lineStart, headerEnd)
	h.e.setSessionLinewise(true)
	h.e.wantReveal = true
	h.frame()
	if !h.e.calloutHeaderSourceOpen(header) {
		t.Fatal("linewise selection of the header did not open its source")
	}
	if !strings.Contains(header.view.disp.text, "> [!NOTE] 提示") {
		t.Fatalf("selected callout header source was not rendered: %q", header.view.disp.text)
	}
	h.shot("callout-header-line-selection-source")

	// Leaving the source selection restores the semantic title/icon view.
	h.e.setSessionLinewise(false)
	h.caretAt(bodyText)
	h.frame()
	if h.e.calloutHeaderSourceOpen(header) || strings.Contains(header.view.disp.text, "[!NOTE]") {
		t.Fatalf("callout header source stayed open after leaving it: %q", header.view.disp.text)
	}
	h.shot("callout-header-preview-restored")
}

func TestCalloutPrefixesRevealAcrossIndependentLeavesAndBlankQuote(t *testing.T) {
	const src = "> [!TIP] 提示\n> callout 第一段。\n>\n> callout 第二段。"
	h := newHarness(t, src)
	lines := []struct {
		name string
		text string
		want string
	}{
		{"header", "[!TIP]", "> [!TIP] 提示"},
		{"first-body", "callout 第一段", "\n> callout 第一段"},
		{"blank-quote", "", ">"},
		{"second-body", "callout 第二段", "> callout 第二段"},
	}
	for _, tc := range lines {
		t.Run(tc.name, func(t *testing.T) {
			var marker, stop int
			var l *leaf
			if tc.name == "blank-quote" {
				bodyEnd := strings.Index(src, "callout 第一段") + len("callout 第一段")
				gap := strings.Index(src[bodyEnd:], "\n>\n")
				if gap < 0 {
					t.Fatal("fixture has no standalone blank quote line after the first body paragraph")
				}
				marker = bodyEnd + gap + 1 // the `>` between the two newline bytes
				for _, candidate := range h.e.leaves {
					if candidate.kind == kBlank && candidate.lineStart == marker {
						l = candidate
						break
					}
				}
				if l == nil {
					t.Fatalf("source marker at %d did not parse as kBlank", marker)
				}
				// A blank quoted line's caret stop is after its `>`; select the
				// physical line so its one-byte source marker is visible too.
				stop = l.start
				h.e.setSessionSelection(l.lineStart, l.start)
				h.e.setSessionLinewise(true)
				h.e.wantReveal = true
				h.frame()
			} else {
				at := strings.Index(src, tc.text)
				if at < 0 {
					t.Fatalf("fixture line %q not found", tc.text)
				}
				at = lineStartAt(src, at)
				marker, stop = at+1, at+1
				h.caretAt(stop)
				h.frame()
				l = h.e.leafAt(stop)
			}
			if l == nil || l.view == nil {
				t.Fatalf("marker at %d did not resolve to a rendered leaf: leaf=%v", marker, l != nil)
			}
			if !strings.Contains(l.view.disp.text, tc.want) {
				t.Fatalf("%s marker did not reveal in its own row: %q want substring %q leaf=%+v ranges=%+v caret=%d pieces=%+v", tc.name, l.view.disp.text, tc.want, l, quoteSourcePrefixes(h.e, l), h.e.Caret(), l.view.disp.pieces)
			}
			foundStop := false
			for _, p := range l.view.disp.pieces {
				if p.kind == pText && p.marker && p.s0+l.lineStart <= marker && p.s1+l.lineStart > marker {
					foundStop = true
				}
			}
			if !foundStop || !h.e.stopOK(l, stop) {
				t.Fatalf("%s marker has no source-backed caret stop", tc.name)
			}
			h.shot("callout-prefix-step-" + tc.name)
		})
	}

	// A full linewise selection must show source prefixes without the
	// projected semantic title being painted over the selected header.
	headerLine := lineStartAt(src, strings.Index(src, "[!TIP]"))
	lastText := strings.Index(src, "callout 第二段")
	h.e.setSessionSelection(headerLine, lastText)
	h.e.setSessionLinewise(true)
	h.e.wantReveal = true
	h.frame()
	var header *leaf
	for _, l := range h.e.leaves {
		if _, _, ok := h.e.calloutHeader(l); ok {
			header = l
			break
		}
	}
	if header == nil || !h.e.calloutHeaderSourceOpen(header) {
		t.Fatal("whole-callout linewise selection did not enter header source mode")
	}
	if strings.Contains(header.view.disp.text, "提示") && strings.Count(header.view.disp.text, "提示") != 1 {
		t.Fatalf("selected header duplicates semantic title/source: %q", header.view.disp.text)
	}
	h.shot("callout-full-visual-source")
}
