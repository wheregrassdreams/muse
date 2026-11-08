package mdedit

import (
	"fmt"
	"testing"
)

func TestZZMermaid2(t *testing.T) {
	src := "前文\n\n```mermaid\ngraph TD\n  A --> B\n```\n\n后文\n"
	h := newHarness(t, src)
	h.caretAt(len(src))
	h.frame()
	m := blockOfKind(h, kCode)
	x, y := m.x+m.w-h.e.lk.size*0.6-1, m.y+h.e.lk.size*0.7
	fmt.Printf("leaf %d..%d y=%.1f h=%.1f click=(%.1f,%.1f) toggleAt=%v\n", m.start, m.end, m.y, m.h, x, y, h.e.diagramToggleAt(x, y) != nil)
	h.tt.ClickAt(x, y)
	h.frame()
	fmt.Printf("after 1: sourceOK=%v caret=%d diagram=%v\n", h.e.sourceOK, h.e.caret, blockOfKind(h, kCode).view.diagram != nil)
	m = blockOfKind(h, kCode)
	x, y = m.x+m.w-h.e.lk.size*0.6-1, m.y+h.e.lk.size*0.7
	fmt.Printf("  toggleAt now=%v (reveal=%v y=%.1f h=%.1f)\n", h.e.diagramToggleAt(x, y) != nil, m.view.reveal, m.y, m.h)
	h.tt.ClickAt(x, y)
	h.frame()
	fmt.Printf("after 2: sourceOK=%v caret=%d diagram=%v\n", h.e.sourceOK, h.e.caret, blockOfKind(h, kCode).view.diagram != nil)
}
