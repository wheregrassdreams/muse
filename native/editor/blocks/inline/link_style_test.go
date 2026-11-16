package editor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	markdownui "muse/nativeui/components/markdown"
)

const linkStyleFixture = "[网页](https://example.test) [[found|有效]] [[missing|失效]] [[pending|未知]]"

func TestExternalLinkIconStaysOutsideSourceMapping(t *testing.T) {
	const src = "前文 [网页](https://example.test) [文件](note.md) [标题](#section) [邮件](mailto:a@example.test) 后文 [[note|内链]]"
	h := newHarness(t, src)
	h.caretAt(0)
	l := h.e.leaves[0]
	if !strings.Contains(l.view.disp.text, "↗") {
		t.Fatalf("external link has no inline marker: %q", l.view.disp.text)
	}
	from, to := strings.Index(src, "[网页]"), strings.Index(src, ")")+1
	iconCount, iconAt := 0, -1
	for _, p := range l.view.disp.pieces {
		if strings.Contains(p.text, "↗") {
			iconCount++
			if p.kind != pDeco || p.s0 != p.s1 {
				t.Fatalf("external icon occupies source bytes: %+v", p)
			}
			iconAt = p.d0
			rects := l.view.layout.SelectionRects(ui.TextInputRange{Start: p.d0, End: p.d1})
			if len(rects) == 0 || rects[0].W <= 0 {
				t.Fatal("external icon has no inline layout geometry")
			}
			img := h.tt.Image()
			inked := 0
			for py := int(l.y + l.view.ty + rects[0].Y); py < int(l.y+l.view.ty+rects[0].Y+rects[0].H); py++ {
				for px := int(l.x + l.view.tx + rects[0].X); px < int(l.x+l.view.tx+rects[0].X+rects[0].W); px++ {
					if pixel := img.RGBAAt(px, py); !sameRGB(pixel, ui.LightTheme().Background) {
						inked++
					}
				}
			}
			if inked == 0 {
				t.Fatal("external icon has no rendered pixels in its inline geometry")
			}
			if p.span.Underline || p.span.DottedUnderline || p.span.Size >= h.e.lk.size {
				t.Fatalf("external icon lost its compact, undecorated style: %+v", p.span)
			}
		}
	}
	if iconCount != 1 {
		t.Fatalf("external icon count=%d, want one", iconCount)
	}
	if got := l.view.disp.dispToSrc(iconAt, false) + l.lineStart; got != to {
		t.Fatalf("icon maps to byte %d, want adjacent source boundary %d", got, to)
	}
	if got := strings.Count(l.view.disp.text, "↗"); got != 1 {
		t.Fatalf("relative, heading, mailto or wiki links received an external icon: %q", l.view.disp.text)
	}
	h.shot("link-external-icon")
	h.e.anchor, h.e.caret = from, to
	h.frame()
	if strings.Contains(h.e.leaves[0].view.disp.text, "↗") {
		t.Fatalf("source-expanded link retained its decoration: %q", h.e.leaves[0].view.disp.text)
	}
	if got, want := h.e.SelectedText(), src[from:to]; got != want {
		t.Fatalf("selection=%q, want source bytes %q", got, want)
	}
	if !h.e.Copy() {
		t.Fatal("copy failed")
	}
	if got, _ := h.clip.Read(transfer.Text); string(got) != src[from:to] {
		t.Fatalf("copied=%q, want only source bytes %q", got, src[from:to])
	}
	if h.e.Source() != src {
		t.Fatalf("decoration changed source: %q", h.e.Source())
	}
}

func TestWikiLinkResolverDistinguishesValidInvalidAndUnknown(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name, theme := "light", ui.LightTheme()
		if dark {
			name, theme = "dark", ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, linkStyleFixture)
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
			}, 1000, 700)
			opts := h.e.Options()
			opts.LinkRevision = 1
			opts.ResolveWikiLink = func(_, target string) LinkTargetState {
				switch target {
				case "found":
					return LinkTargetValid
				case "missing":
					return LinkTargetInvalid
				default:
					return LinkTargetUnknown
				}
			}
			h.e.SetOptions(opts)
			h.frame()
			valid := linkStyleAt(t, h.e, strings.Index(h.e.src, "有效"))
			invalid := linkStyleAt(t, h.e, strings.Index(h.e.src, "失效"))
			unknown := linkStyleAt(t, h.e, strings.Index(h.e.src, "未知"))
			want := markdownui.InlineLinkColorFor(h.e.lk.accent, dark)
			if valid.Color != want || !valid.Underline || valid.DottedUnderline {
				t.Fatalf("valid wiki style=%+v, want regular linked underline %v", valid, want)
			}
			if !invalid.DottedUnderline || !invalid.Underline || invalid.Color == want {
				t.Fatalf("invalid wiki style=%+v, want subdued dotted link", invalid)
			}
			if unknown.Color != want || !unknown.Underline || unknown.DottedUnderline {
				t.Fatalf("unknown wiki state was treated as invalid: %+v", unknown)
			}
			h.shot("link-states-" + name)

			// A changed index revision must rebuild cached link styling even
			// when the source and target text are unchanged.
			state := LinkTargetInvalid
			opts = h.e.Options()
			opts.LinkRevision++
			opts.ResolveWikiLink = func(_, _ string) LinkTargetState { return state }
			h.e.SetOptions(opts)
			h.frame()
			if got := linkStyleAt(t, h.e, strings.Index(h.e.src, "有效")); !got.DottedUnderline {
				t.Fatalf("updated resolver state stayed cached: %+v", got)
			}
			state = LinkTargetValid
			opts = h.e.Options()
			opts.LinkContext = "another-note.md"
			opts.ResolveWikiLink = func(_, _ string) LinkTargetState { return state }
			h.e.SetOptions(opts)
			h.frame()
			if got := linkStyleAt(t, h.e, strings.Index(h.e.src, "有效")); got.DottedUnderline {
				t.Fatalf("resolver context change left stale link style: %+v", got)
			}
			opts = h.e.Options()
			opts.LinkRevision++
			opts.ResolveWikiLink = nil
			h.e.SetOptions(opts)
			h.frame()
			if got := linkStyleAt(t, h.e, strings.Index(h.e.src, "失效")); got.DottedUnderline {
				t.Fatalf("nil resolver was treated as invalid: %+v", got)
			}
		})
	}
}

func TestLinkHoverHighlightsOnlyCurrentLink(t *testing.T) {
	const src = "[first](https://same.test) and [second](https://same.test)   tail"
	h := newHarness(t, src)
	first := strings.Index(src, "first")
	second := strings.Index(src, "second")
	if got := linkStyleAt(t, h.e, first); got.Color == (ui.Color{}) {
		t.Fatal("first link has no style")
	}
	x, y := pointForSource(t, h.e, first+2)
	beforeSource, beforeAnchor, beforeCaret := h.e.Source(), h.e.anchor, h.e.caret
	beforeLines := h.e.leaves[0].view.layout.Lines()
	h.e.hover(x, y)
	h.frame()
	if h.e.Source() != beforeSource || h.e.anchor != beforeAnchor || h.e.caret != beforeCaret {
		t.Fatalf("hover changed source or selection: source=%q anchor=%d caret=%d", h.e.Source(), h.e.anchor, h.e.caret)
	}
	if after := h.e.leaves[0].view.layout.Lines(); !reflect.DeepEqual(after, beforeLines) {
		t.Fatalf("hover changed link geometry: before=%v after=%v", beforeLines, after)
	}
	firstStyle := linkStyleAt(t, h.e, first)
	secondStyle := linkStyleAt(t, h.e, second)
	if firstStyle.Color == secondStyle.Color {
		t.Fatalf("hover did not distinguish the pointed link: first=%+v second=%+v", firstStyle, secondStyle)
	}
	baseColor := markdownui.InlineLinkColorFor(h.e.lk.accent, h.e.lk.dark)
	x, y = pointForSource(t, h.e, second+2)
	h.e.hover(x, y)
	h.frame()
	if h.e.Source() != beforeSource || h.e.anchor != beforeAnchor || h.e.caret != beforeCaret {
		t.Fatalf("moving hover changed source or selection: source=%q anchor=%d caret=%d", h.e.Source(), h.e.anchor, h.e.caret)
	}
	if after := h.e.leaves[0].view.layout.Lines(); !reflect.DeepEqual(after, beforeLines) {
		t.Fatalf("moving hover changed link geometry: before=%v after=%v", beforeLines, after)
	}
	if a, b := linkStyleAt(t, h.e, first), linkStyleAt(t, h.e, second); a.Color != baseColor || b.Color == baseColor || a.Color == b.Color {
		t.Fatalf("moving hover between equal-URL links did not move the highlight: first=%+v second=%+v", a, b)
	}
	h.shot("link-hover-first")
	h.e.hover(980, 680)
	h.frame()
	if a, b := linkStyleAt(t, h.e, first), linkStyleAt(t, h.e, second); a.Color != b.Color {
		t.Fatalf("link hover did not clear after leaving: first=%+v second=%+v", a, b)
	}
	x, y = pointForSource(t, h.e, strings.Index(src, "   tail")+1)
	h.e.hover(x, y)
	h.frame()
	if a, b := linkStyleAt(t, h.e, first), linkStyleAt(t, h.e, second); a.Color != b.Color {
		t.Fatalf("link hover remained active in trailing paragraph whitespace: first=%+v second=%+v", a, b)
	}
}

func TestLinkHoverPreservesUpstreamSoftWrapCaret(t *testing.T) {
	src := strings.Repeat("Long prose keeps a stable visual column across wraps. ", 5) + "[target](https://example.test)"
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.MaxWidth = 280
	h.e.SetOptions(opts)
	h.frame()
	l := h.e.leafAt(0)
	lines := l.view.layout.Lines()
	if len(lines) < 2 {
		t.Fatal("fixture did not wrap")
	}
	off := l.view.disp.dispToSrc(lines[0].End, false) + l.lineStart
	h.e.anchor, h.e.caret, h.e.upstream = off, off, true
	h.frame()
	before, ok := h.e.caretRect(off)
	if !ok {
		t.Fatal("upstream caret has no geometry")
	}
	beforeLines := l.view.layout.Lines()
	x, y := pointForSource(t, h.e, strings.Index(src, "target")+2)
	anchor, caret := h.e.anchor, h.e.caret
	h.e.hover(x, y)
	h.frame()
	after, ok := h.e.caretRect(off)
	if !ok || after != before {
		t.Fatalf("hover moved soft-wrap caret geometry: off=%d upstream=%v before=%+v after=%+v lines=%v/%v ok=%v", off, h.e.upstream, before, after, beforeLines, h.e.leafAt(0).view.layout.Lines(), ok)
	}
	if !h.e.upstream || h.e.anchor != anchor || h.e.caret != caret || h.e.Source() != src {
		t.Fatalf("hover changed editing state: upstream=%v anchor=%d caret=%d", h.e.upstream, h.e.anchor, h.e.caret)
	}
}

func TestLinkHoverRemainsReadableInDarkTheme(t *testing.T) {
	const src = "[link](https://example.test)"
	h := newHarness(t, src)
	theme := ui.DarkTheme()
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
	}, 1000, 700)
	h.frame()
	x, y := pointForSource(t, h.e, strings.Index(src, "link")+1)
	h.e.hover(x, y)
	h.frame()
	sp := linkStyleAt(t, h.e, strings.Index(src, "link"))
	if sp.Color == markdownui.InlineLinkColorFor(h.e.lk.accent, true) {
		t.Fatal("dark-theme link hover did not change the current link")
	}
	if contrast := contrastOn(sp.Color, ui.Transparent, theme.Background); contrast < 4.5 {
		t.Fatalf("dark-theme hovered link contrast %.2f is too low", contrast)
	}
}

func linkStyleAt(t *testing.T, e *Editor, off int) ui.Span {
	t.Helper()
	l := e.leafAt(off)
	for _, p := range l.view.disp.pieces {
		if p.kind == pText && p.s0+l.lineStart <= off && off < p.s1+l.lineStart {
			return p.span
		}
	}
	t.Fatalf("no text style at source byte %d", off)
	return ui.Span{}
}

func pointForSource(t *testing.T, e *Editor, off int) (float32, float32) {
	t.Helper()
	l := e.leafAt(off)
	d, ok := l.view.disp.srcToDisp(off - l.lineStart)
	if !ok {
		t.Fatalf("source offset %d has no display position", off)
	}
	rects := l.view.layout.SelectionRects(ui.TextInputRange{Start: d, End: d + 1})
	if len(rects) == 0 {
		t.Fatalf("source offset %d has no hit rectangle", off)
	}
	r := rects[0]
	return l.x + l.view.tx + r.X + r.W/2, l.y + l.view.ty + r.Y + r.H/2
}
