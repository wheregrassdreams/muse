package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// This follows the screenshot-shaped path: put the caret with a pointer in a
// wrapped header cell, then deliver Vim's physical key and text events.
func TestPointerPlacedWrappedHeaderVJKeepsHeaderRowSelected(t *testing.T) {
	src := "| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| `Attachments/landscape.webp` | [File:Webp (1).webp](https://commons.wikimedia.org/wiki/File:Webp_(1).webp) | Churchill Machine Tool Co. Ltd.; source: picryl.com | [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) |\n| another file | [File:A](https://example.test) | Public-domain artist | Public domain |"
	h := &harness{t: t, e: New(src, Options{FontSize: 18, MaxWidth: 820, PadX: 32})}
	h.e.SetKeymap("vim")
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { h.e.View(c).Grow(1) })
	}, 1040, 520)
	h.frame()
	h.frame()
	h.e.Focus()
	h.frame()
	off := strings.Index(src, "Source") + 2
	caret, ok := h.e.caretRect(off)
	if !ok {
		t.Fatal("header caret geometry unavailable")
	}
	x, y := caret.X, caret.Y+caret.H/2-h.e.scroll.Y
	h.tt.Move(x, y)
	h.tt.Press(x, y)
	h.tt.Release(x, y)
	h.frame()
	table := documentTestTable(t, h.e)
	_, row, col := cellAt(table, h.e.Caret())
	if row != 0 || col != 1 {
		t.Fatalf("pointer landed at (%d,%d), expected header cell (0,1)", row, col)
	}
	h.tt.Key(ui.Shift, ui.KeyV)
	h.frame()
	h.tt.Type("V")
	h.frame()
	h.tt.Key(0, ui.KeyJ)
	h.frame()
	h.tt.Type("j")
	h.frame()
	s := h.e.tableSelection
	if s == nil || s.axis != tableRows || s.first != 0 || s.last != 1 {
		t.Fatalf("Vj from wrapped header omitted header row: %+v", s)
	}
	h.tt.Key(0, ui.KeyK)
	h.frame()
	h.tt.Type("k")
	h.frame()
	s = h.e.tableSelection
	if s == nil || s.axis != tableRows || s.first != 0 || s.last != 0 {
		t.Fatalf("Vjk did not shrink back to header row: %+v", s)
	}
}
