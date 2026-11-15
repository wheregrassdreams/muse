package editor

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestBlockSelectionRetainsShortAndBlankDisplayRows(t *testing.T) {
	h := newHarness(t, "abcde\nx\n\nabcde")
	h.e.SetKeymap("vim")
	h.e.Select(1, 1)
	h.key(ui.Ctrl, ui.KeyV)
	sendVim(h, "l3j")
	if got := h.e.SelectedText(); got != "bc\n\n\nbc" {
		t.Fatalf("block rows = %q", got)
	}
	rs := h.e.SelectionRanges()
	if len(rs) != 4 || rs[1].Start != rs[1].End || rs[2].Start != rs[2].End {
		t.Fatalf("short/blank rows lost from ranges: %#v", rs)
	}
	if os.Getenv("MDEDIT_SHOTS") != "" {
		h.shot("rectangle-short-and-empty-rows")
	}
}

func TestBlockChangeTypesAtEverySelectedRowAndUndoesTogether(t *testing.T) {
	const src = "abcde\nx\nabcde"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.e.Select(1, 1)
	h.key(ui.Ctrl, ui.KeyV)
	sendVim(h, "l2jcZ")
	h.key(0, ui.KeyEscape)
	if got, want := h.e.Source(), "aZde\nxZ\naZde"; got != want {
		t.Fatalf("block change = %q, want %q", got, want)
	}
	h.e.Undo()
	if got := h.e.Source(); got != src {
		t.Fatalf("one undo did not restore block change: %q", got)
	}
}

func TestBlockUndoRestoresRangesAndRedoClearsThem(t *testing.T) {
	const src = "abcde\nabcde"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.e.Select(1, 1)
	h.key(ui.Ctrl, ui.KeyV)
	sendVim(h, "ljd")
	h.e.Undo()
	if got := h.e.SelectedText(); got != "bc\nbc" {
		t.Fatalf("undo selection = %q", got)
	}
	if !h.e.Copy() {
		t.Fatal("copy after undo failed")
	}
	if b, _ := h.clip.Read(transfer.Text); string(b) != "bc\nbc" {
		t.Fatalf("copy after undo = %q", b)
	}
	h.key(ui.Ctrl, ui.KeyR)
	if h.e.Source() != "ade\nade" || len(h.e.SelectionRanges()) != 0 {
		t.Fatalf("redo source/ranges = %q / %#v", h.e.Source(), h.e.SelectionRanges())
	}
}

func TestBlockClipboardCarriesShapeWithoutChangingTextAndPasteModes(t *testing.T) {
	src := "abcde\nabcde"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.e.Select(1, 1)
	h.key(ui.Ctrl, ui.KeyV)
	sendVim(h, "lj")
	if !h.e.Copy() {
		t.Fatal("block copy failed")
	}
	b, err := h.clip.Read(rectangleClipboardFormat)
	if err != nil {
		t.Fatal("block metadata missing from native clipboard")
	}
	if rows, ok := decodeRectangleClipboard(b); !ok || len(rows) != 2 || rows[0] != "bc" || rows[1] != "bc" {
		t.Fatalf("clipboard block payload = %q / %v", b, ok)
	}
	plain := newHarness(t, "abcde\nabcde")
	plain.clip = h.clip
	plain.e.Select(1, 1)
	plain.e.Paste(true)
	if plain.e.Source() != "abc\nbcbcde\nabcde" {
		t.Fatalf("plain paste did not use text representation only: %q", plain.e.Source())
	}
	rich := newHarness(t, "abcde\nabcde")
	rich.clip = h.clip
	rich.e.Select(1, 1)
	rich.e.Paste(false)
	if rich.e.Source() != "abcbcde\nabcbcde" {
		t.Fatalf("native paste did not restore block shape: %q", rich.e.Source())
	}
}

func TestBlockInputSessionCommitsOnEscapeAndUndoesAsOneStep(t *testing.T) {
	const src = "abcde\nabcde"
	for _, tc := range []struct{ name, keys, text, want string }{{"change cancel", "ljc", "", "ade\nade"}, {"insert multikey", "l2jI", "你好", "a你好bcde\na你好bcde"}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, src)
			h.e.SetKeymap("vim")
			h.e.Select(1, 1)
			h.key(ui.Ctrl, ui.KeyV)
			sendVim(h, tc.keys)
			if tc.text != "" {
				h.typ("你")
				h.typ("好")
			}
			h.key(0, ui.KeyEscape)
			if got := h.e.Source(); got != tc.want {
				t.Fatalf("source=%q want %q", got, tc.want)
			}
			h.e.Undo()
			if got := h.e.Source(); got != src {
				t.Fatalf("session undo=%q", got)
			}
		})
	}
}

func TestBlockAInsertsAtTheRightEdgeOnEveryRow(t *testing.T) {
	h := newHarness(t, "abcde\nabcde")
	h.e.SetKeymap("vim")
	h.e.Select(1, 1)
	h.key(ui.Ctrl, ui.KeyV)
	sendVim(h, "ljA")
	h.typ("X")
	h.key(0, ui.KeyEscape)
	if got, want := h.e.Source(), "abcXde\nabcXde"; got != want {
		t.Fatalf("block A = %q want %q", got, want)
	}
	h.e.Undo()
	if h.e.Source() != "abcde\nabcde" {
		t.Fatalf("block A undo=%q", h.e.Source())
	}
}

func TestBlockPAndPUseDistinctGeometricColumns(t *testing.T) {
	for _, tc := range []struct{ key, want string }{{"P", "Z中文\nZ中文"}, {"p", "中Z文\n中Z文"}} {
		h := newHarness(t, "中文\n中文")
		h.e.SetKeymap("vim")
		h.e.modal.registerBlock = []string{"Z", "Z"}
		h.e.Select(0, 0)
		sendVim(h, tc.key)
		if got := h.e.Source(); got != tc.want {
			t.Fatalf("%s result=%q want %q", tc.key, got, tc.want)
		}
	}
}

func TestBlockRangeRevealAndViewCacheUseActualRangeRevision(t *testing.T) {
	h := newHarness(t, "*bold* and `code`")
	e := h.e
	e.block = &blockSelection{x0: 0, y0: 0, revision: 1, ranges: []Range{{Start: 1, End: 2}}}
	if !selectionReveals(e, 0, 6, false) {
		t.Fatal("selected emphasis source did not reveal")
	}
	if selectionReveals(e, 11, 17, false) {
		t.Fatal("unselected inline code revealed through an envelope")
	}
	e.layoutIfNeeded()
	u := e.units()[0]
	first := e.unitView(u, e.caret)
	e.block.revision++
	e.block.ranges = []Range{{Start: 12, End: 13}}
	e.leavesLaid = false
	e.layoutIfNeeded()
	u = e.units()[0]
	second := e.unitView(u, e.caret)
	if first == second {
		t.Fatal("block range revision reused an inline source view")
	}
}

func TestBlockSelectionUsesShapedCJKAndSoftWrappedDisplayRows(t *testing.T) {
	h := newHarness(t, "甲乙丙丁\n甲乙丙丁")
	h.e.SetKeymap("vim")
	h.e.Select(len("甲"), len("甲"))
	h.key(ui.Ctrl, ui.KeyV)
	sendVim(h, "lj")
	if got := h.e.SelectedText(); got != "乙丙\n乙丙" {
		t.Fatalf("CJK geometric selection=%q", got)
	}
	if os.Getenv("MDEDIT_SHOTS") != "" {
		h.shot("rectangle-cjk")
	}
	w := newHarness(t, "abcdefghijklmnoabcdefghijklmno")
	opt := w.e.Options()
	opt.MaxWidth = 120
	w.e.SetOptions(opt)
	w.frame()
	w.e.SetKeymap("vim")
	w.e.Select(0, 0)
	w.key(ui.Ctrl, ui.KeyV)
	sendVim(w, "l4j")
	if len(w.e.SelectionRanges()) < 2 {
		t.Fatalf("soft-wrapped display rows collapsed to source envelope: %#v", w.e.SelectionRanges())
	}
	if got := w.e.SelectedText(); !strings.Contains(got, "\n") {
		t.Fatalf("soft-wrapped block copy did not preserve display rows: %q", got)
	}
	if os.Getenv("MDEDIT_SHOTS") != "" {
		w.shot("rectangle-soft-wrap")
	}
}

func TestBlockSelectionPaintsInlineCodeRows(t *testing.T) {
	h := newHarness(t, "a `code` z\na `code` z")
	h.e.SetKeymap("vim")
	h.e.Select(3, 3)
	h.key(ui.Ctrl, ui.KeyV)
	sendVim(h, "lj")
	if os.Getenv("MDEDIT_SHOTS") != "" {
		h.shot("rectangle-inline-code")
	}
}
