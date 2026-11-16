package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func pointerCell(t *testing.T, e *Editor, off int, fraction float32) (float32, float32) {
	t.Helper()
	a, ok := e.caretRect(off)
	if !ok {
		t.Fatalf("no caret geometry at %d", off)
	}
	b, ok := e.caretRect(e.nextGraphemeBoundary(off))
	if !ok || b.X <= a.X {
		t.Fatalf("no forward character geometry from %d: before=%+v after=%+v", off, a, b)
	}
	return a.X + (b.X-a.X)*fraction, a.Y + a.H/2
}

func TestVimMouseDragUsesVisualSelectionModel(t *testing.T) {
	const src = "abcd"
	for _, tc := range []struct {
		name          string
		start, target int
		motion        string
	}{
		{"forward", 1, 2, "l"},
		{"reverse", 2, 1, "h"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mouse := newHarness(t, src)
			mouse.e.SetKeymap("vim")
			x, y := pointerCell(t, mouse.e, tc.start, .75)
			mouse.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x, Y: y})
			if mouse.e.caret != tc.start {
				t.Fatalf("Vim click landed at %d, want the start of character %d", mouse.e.caret, tc.start)
			}
			targetX, targetY := pointerCell(t, mouse.e, tc.target, .75)
			mouse.e.dragTo(targetX, targetY)
			if mouse.e.modal.mode != "Visual" || !mouse.e.selectionInclusive {
				t.Fatalf("mouse drag mode=%q inclusive=%v, want Vim Visual inclusive selection", mouse.e.modal.mode, mouse.e.selectionInclusive)
			}

			keys := newHarness(t, src)
			keys.e.SetKeymap("vim")
			keys.e.setCaret(tc.start, false)
			sendVim(keys, "v"+tc.motion)
			if got, want := mouse.e.SelectedText(), keys.e.SelectedText(); got != want || got != "bc" {
				t.Fatalf("mouse selection=%q, Vim v-%s selection=%q; want shared selection %q", got, tc.motion, want, "bc")
			}
			if mouse.e.anchor != keys.e.anchor || mouse.e.caret != keys.e.caret {
				t.Fatalf("mouse endpoints=(%d,%d), Vim endpoints=(%d,%d)", mouse.e.anchor, mouse.e.caret, keys.e.anchor, keys.e.caret)
			}
			mouse.shot("vim-mouse-visual-selection-" + tc.name)
		})
	}
}

func TestVimNormalClickAndCaretStayOnCharacter(t *testing.T) {
	const src = "abcd"
	leftHalf := newHarness(t, src)
	leftHalf.e.SetKeymap("vim")
	x, y := pointerCell(t, leftHalf.e, 2, .25) // Left half of “c”.
	leftHalf.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x, Y: y})
	if leftHalf.e.caret != 2 {
		t.Fatalf("left-half click selected character at %d, want c at 2", leftHalf.e.caret)
	}

	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	x, y = pointerCell(t, h.e, 3, .9) // Empty space after the last glyph.
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x + 20, Y: y})
	if h.e.caret != 3 {
		t.Fatalf("click after last glyph left Vim caret at %d, want last character at 3", h.e.caret)
	}
	if got := h.e.vimNormalOffset(len(src)); got != 3 {
		t.Fatalf("normal-mode end offset=%d, want last character at 3", got)
	}
	h.e.setCaret(len(src), false)
	if h.e.caret != 3 {
		t.Fatalf("normal-mode caret reached empty end position %d, want 3", h.e.caret)
	}
	h.shot("vim-normal-caret-at-end")
}

func TestVimLinewiseSelectsOneBlankLine(t *testing.T) {
	h := newHarness(t, "before\n\n\nnext")
	h.e.SetKeymap("vim")
	var blanks []*leaf
	for _, l := range h.e.leaves {
		if l.kind == kBlank {
			blanks = append(blanks, l)
		}
	}
	if len(blanks) != 2 {
		t.Fatalf("parsed %d blank lines, want two", len(blanks))
	}
	for i, blank := range blanks {
		h.e.anchor, h.e.caret = blank.start, blank.start
		sendVim(h, "V")
		if got := h.e.SelectedText(); got != "\n" {
			t.Fatalf("V on blank line %d selected %q, want one line break", i, got)
		}
		if h.e.modal.mode != "Visual" || !h.e.selectionLinewise {
			t.Fatalf("blank-line V mode=%q linewise=%v", h.e.modal.mode, h.e.selectionLinewise)
		}
		a, z := h.e.selection()
		for j, candidate := range blanks {
			if selected := h.e.selectionCoversBlankLine(candidate, a, z); selected != (i == j) {
				t.Fatalf("V on blank line %d visually selects blank line %d: range=[%d,%d)", i, j, a, z)
			}
		}
		if i == 0 {
			h.shot("vim-one-blank-line")
		}
		h.e.modalKey(ui.KeyEscape, 0)
	}
}

func TestVimMotionAcrossInlineMathKeepsItsDestination(t *testing.T) {
	const src = "before $x^2$ after\nnext"
	for _, mode := range []string{"Normal", "Visual", "Linewise Visual"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t, src)
			h.e.SetKeymap("vim")
			h.e.setCaret(9, false) // Inside the collapsed math source.
			if h.e.caret != 9 {
				t.Fatalf("normal-mode caret normalized an interior offset to %d", h.e.caret)
			}
			switch mode {
			case "Normal":
				sendVim(h, "l")
			case "Visual":
				sendVim(h, "vl")
			case "Linewise Visual":
				sendVim(h, "Vj")
			}
			want := 10
			if mode == "Linewise Visual" {
				want = len("before $x^2$ after\nnext") - len("next")
			}
			if h.e.caret != want {
				t.Fatalf("%s ended at %d, want %d", mode, h.e.caret, want)
			}
		})
	}
}
