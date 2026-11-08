package mdedit

import (
	"fmt"
	"strings"
	"testing"
)

func controlDoc() string {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "第 %d 段，用来把控件推到下面去。\n\n", i)
	}
	b.WriteString("```mermaid\ngraph TD\n  A --> B\n```\n\n")
	b.WriteString("- [ ] 一件事\n\n| 甲 | 乙 |\n| --- | --- |\n| a | b |\n\n![图](a.png)\n\n")
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&b, "尾段 %d，给控件下面留出余地：块缩小不该是移动视图的那个原因。\n\n", i)
	}
	return b.String()
}

// TestBlockControlsLeaveTheCaretAndTheViewAlone: the controls a block
// carries — a task's box, a diagram's source, a table's or a picture's tag —
// change what the block shows, and nothing else. They do not move the caret,
// they do not scroll the view back to it, and pressing one twice leaves the
// block as it was found.
//
// This is the mistake this package made three times: a control that acts on
// a block ends up moving the text's caret, because the caret is the one
// piece of state every path reaches for (see docs/editor-model.md §6).
func TestBlockControlsLeaveTheCaretAndTheViewAlone(t *testing.T) {
	src := controlDoc()
	for _, c := range []struct {
		name    string
		press   func(h *harness) bool
		on, off func(h *harness) bool
	}{
		{"task-box", pressTaskBox, hasChecked, hasUnchecked},
		{"code-label", pressCodeLabel, codeShowsSource, codeHidesSource},
		{"diagram-source", pressDiagram, diagramShowsSource, diagramDrawn},
		{"table-tag", pressTableTag, tableShowsSource, tableDraws},
		{"picture-tag", pressPictureTag, pictureShowsSource, pictureDraws},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, src)
			h.caretAt(0)
			h.frame()
			// The view goes where the controls are, with room below them
			// (a block that shrinks must not be what moves the view), and
			// the caret stays at the top, off the screen: that is where a
			// person's eyes are when they click one.
			h.e.scroll.Y = blockOfKind(h, kCode).y - 25
			h.frame()
			caret, scroll := h.e.caret, h.e.scroll.Y
			for i, want := range []func(*harness) bool{c.on, c.off} {
				if !c.press(h) {
					t.Fatal("no control to press")
				}
				h.frame()
				h.frame()
				if h.e.caret != caret {
					t.Fatalf("press %d moved the caret: %d -> %d", i+1, caret, h.e.caret)
				}
				if h.e.scroll.Y != scroll {
					t.Fatalf("press %d scrolled the view: %.1f -> %.1f", i+1, scroll, h.e.scroll.Y)
				}
				if !want(h) {
					t.Fatalf("press %d did not change what the block shows", i+1)
				}
			}
		})
	}
}

func pressTaskBox(h *harness) bool {
	for _, l := range h.e.leaves {
		if it, ok := l.innerItem(); ok && it.task != 0 {
			x := h.e.colX + h.e.indentOf(l.conts) - h.e.lk.size*0.9
			h.tt.ClickAt(x, l.y+h.e.lk.size*0.8-h.e.scroll.Y)
			return true
		}
	}
	return false
}

func pressCodeLabel(h *harness) bool {
	l := blockOfKind(h, kCode)
	if l == nil {
		return false
	}
	h.tt.ClickAt(l.x+l.w-h.e.lk.size*1.2, l.y+4-h.e.scroll.Y)
	return true
}

// codeShowsSource: the block shows its fence and its lines.
func codeShowsSource(h *harness) bool {
	l := blockOfKind(h, kCode)
	return l != nil && l.view != nil && strings.Contains(l.view.disp.text, "```")
}

func codeHidesSource(h *harness) bool {
	l := blockOfKind(h, kCode)
	return l != nil && l.view != nil && !strings.Contains(l.view.disp.text, "```")
}

func pressDiagram(h *harness) bool {
	l := blockOfKind(h, kCode)
	if l == nil {
		return false
	}
	h.tt.ClickAt(l.x+l.w-h.e.lk.size*0.6-1, l.y+h.e.lk.size*0.7-h.e.scroll.Y)
	return true
}

func pressTableTag(h *harness) bool {
	l := blockOfKind(h, kTable)
	if l == nil {
		// Open already: the block is a block of lines, found by its line.
		if i := h.e.blockAt(h.e.sourceAt); h.e.sourceOK && i >= 0 {
			l = h.e.leaves[i]
		}
	}
	if l == nil {
		return false
	}
	chip := h.e.sourceChip(l)
	h.tt.Move(l.x+40, l.y+20-h.e.scroll.Y)
	h.frame()
	h.tt.ClickAt(chip.X+6, chip.Y+4-h.e.scroll.Y)
	return true
}

func pressPictureTag(h *harness) bool {
	var p *leaf
	for _, l := range h.e.leaves {
		if h.e.pictureBlock(l) {
			p = l
		}
	}
	if p == nil {
		return false
	}
	chip := h.e.sourceChip(p)
	h.tt.Move(p.x+40, p.y+20-h.e.scroll.Y)
	h.frame()
	h.tt.ClickAt(chip.X+6, chip.Y+4-h.e.scroll.Y)
	return true
}

func hasChecked(h *harness) bool   { return strings.Contains(h.e.Source(), "[x] 一件事") }
func hasUnchecked(h *harness) bool { return strings.Contains(h.e.Source(), "[ ] 一件事") }

func diagramShowsSource(h *harness) bool {
	l := blockOfKind(h, kCode)
	return l != nil && l.view != nil && l.view.diagram == nil && strings.Contains(l.view.disp.text, "```")
}

func diagramDrawn(h *harness) bool {
	l := blockOfKind(h, kCode)
	return l != nil && l.view != nil && l.view.diagram != nil
}

func tableShowsSource(h *harness) bool {
	for _, l := range h.e.leaves {
		if l.kind == kRaw && strings.HasPrefix(strings.TrimSpace(h.e.Source()[l.start:l.end]), "|") {
			return true
		}
	}
	return false
}

func tableDraws(h *harness) bool {
	l := blockOfKind(h, kTable)
	return l != nil && l.tab != nil
}

func pictureShowsSource(h *harness) bool {
	for _, l := range h.e.leaves {
		if h.e.pictureBlock(l) && l.view != nil && strings.Contains(l.view.disp.text, "![") {
			return true
		}
	}
	return false
}

func pictureDraws(h *harness) bool {
	n := 0
	for _, l := range h.e.leaves {
		if h.e.pictureBlock(l) && l.view != nil && l.view.disp.text == objectRune {
			n++
		}
	}
	return n == 1
}
