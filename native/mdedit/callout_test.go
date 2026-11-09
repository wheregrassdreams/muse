package mdedit

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	"strings"
	"testing"
)

func TestCalloutCardsSourceAndHeader(t *testing.T) {
	for _, header := range []string{"[!NOTE]", "[!NOTE] 提示", "[!WARNING] 小心"} {
		t.Run(header, func(t *testing.T) {
			src := "> " + header + "\n> first **bold**\n>\n> second\n>\n> > nested quote\n\nafter\n"
			h := newHarness(t, src)
			h.caretAt(len(src) - 2)
			found, blank, nested := false, false, false
			for _, l := range h.e.leaves {
				if len(l.conts) > 1 {
					nested = true
				}
				if l.kind == kBlank && len(l.conts) > 0 {
					blank = true
				}
				if c, end, ok := h.e.calloutHeader(l); ok {
					found = true
					if c.alert == parser.NoAlert {
						t.Fatal("missing alert")
					}
					if l.view == nil {
						t.Fatal("missing header view")
					}
					if strings.Contains(l.view.disp.text, "[!") || strings.Contains(l.view.disp.text, "提示") {
						t.Fatalf("duplicate title: %q", l.view.disp.text)
					}
					for at := l.lineStart; at <= end; at++ {
						// Only UTF-8 boundaries are caret positions.
						if at < len(src) && src[at]&0xc0 == 0x80 {
							continue
						}
						h.caretAt(at)
						v := h.e.view(l, l.w, false, parser.AlignNone)
						if _, ok := v.disp.srcToDisp(at - l.lineStart); !ok {
							t.Fatalf("header byte %d inaccessible", at)
						}
					}
				}
			}
			if !found || !blank || !nested {
				t.Fatalf("header=%v blank=%v nested=%v", found, blank, nested)
			}
			h.want(src)
		})
	}
}

func TestCalloutThemeShots(t *testing.T) {
	src := "> [!NOTE] 提示\n> first **bold** paragraph\n>\n> second paragraph\n>\n> > ordinary nested quote\n\n> [!WARNING] Custom warning\n> Be careful.\n>\n> > 嵌套警告引用\n\n> [!TIP]\n> Helpful advice.\n\nordinary quote:\n\n> Still a bar.\n\nend\n"
	for _, dark := range []bool{false, true} {
		name := "callouts-light"
		theme := ui.LightTheme()
		if dark {
			name = "callouts-dark"
			theme = ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := &harness{t: t, e: New(src, Options{FontSize: 16})}
			h.tt = ui.NewTester(func(c *ui.Context) { ui.Themed(c, theme).Fill().Children(func() { h.e.View(c).Grow(1) }) }, 1000, 900)
			h.frame()
			h.caretAt(len(src) - 2)
			h.frame()
			if h.e.lk.dark != dark {
				t.Fatal("theme not applied")
			}
			h.want(src)
			h.shot(name)
			// Each following card starts beyond the previous container's body.
			lastBottom := float32(-1)
			seen := map[int]bool{}
			for _, l := range h.e.leaves {
				for _, c := range l.conts {
					if !isCalloutContainer(c) {
						continue
					}
					if !seen[c.id] {
						if l.y-h.e.lk.size*.45 < lastBottom {
							t.Fatal("overlapping cards")
						}
						seen[c.id] = true
					}
					lastBottom = l.y + l.h + h.e.lk.size*.6
				}
			}
		})
	}
}
