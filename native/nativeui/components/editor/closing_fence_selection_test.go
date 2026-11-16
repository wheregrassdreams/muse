package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestVisualLineSelectionPaintsClosingFence(t *testing.T) {
	const src = "```go\nfunc main() {\n    println(\"hello, muse\")\n}\n```\n\n后续正文。"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	code := h.e.leaves[0]
	closeStart := strings.LastIndex(src, "```")
	closeEnd := closeStart + len("```")
	h.e.sourceOK, h.e.sourceAt = true, code.lineStart
	h.e.setSessionSelection(0, closeStart+2)
	h.e.setSessionLinewise(true)
	h.e.setSessionInclusive(true)
	h.e.wantReveal = true
	h.e.invalidate()
	h.frame()

	v := h.e.unitView(code, h.e.Caret())
	closeDisplay, ok := v.disp.srcToDisp(closeStart - code.lineStart)
	if !ok {
		t.Fatal("closing fence has no source-backed display position")
	}
	closeDisplayEnd := v.disp.looseDisp(closeEnd - code.lineStart)
	rects := v.layout.SelectionRects(ui.TextInputRange{Start: closeDisplay, End: closeDisplayEnd})
	if len(rects) == 0 {
		t.Fatalf("closing fence has no selection geometry: source=%q display=%q", src[closeStart:closeEnd], v.disp.text)
	}
	img := h.tt.Image()
	baseline := newHarness(t, src)
	baselineCode := baseline.e.leaves[0]
	baseline.e.sourceOK, baseline.e.sourceAt = true, baselineCode.lineStart
	baseline.e.setSessionSelection(len(src), len(src))
	baseline.e.wantReveal = true
	baseline.e.invalidate()
	baseline.frame()
	baselineView := baseline.e.unitView(baselineCode, baseline.e.Caret())
	// Probe the top of the first selected glyph cell, above the glyph ink.
	// The selected frame must differ from the otherwise identical code panel.
	r := rects[0]
	x, y := int(code.x+v.tx+r.X+1), int(code.y+v.ty+r.Y+1)
	baseX, baseY := int(baselineCode.x+baselineView.tx+r.X+1), int(baselineCode.y+baselineView.ty+r.Y+1)
	if img.RGBAAt(x, y) == baseline.tt.Image().RGBAAt(baseX, baseY) {
		t.Fatalf("closing fence selection background is missing at (%d,%d): selected=%v baseline=%v rect=%+v selection=%+v color=%+v panel=%+v source=%q display=%q", x, y, img.RGBAAt(x, y), baseline.tt.Image().RGBAAt(baseX, baseY), r, h.e.SelectionRange(), h.e.lk.selection, h.e.lk.panel, src[closeStart:closeEnd], v.disp.text)
	}
	h.shot("visual-line-selection-closing-fence")
}
