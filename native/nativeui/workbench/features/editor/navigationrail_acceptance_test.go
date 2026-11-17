package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestNavigationAcceptanceBlockCopyDelete(t *testing.T) {
	const src = "abcde\nabcde\nabcde"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.e.Select(1, 1)
	h.key(ui.Ctrl, ui.KeyV)
	h.e.imeClient.ReplaceText(nil, "l")
	h.e.imeClient.ReplaceText(nil, "j")
	h.frame()
	if got := h.e.SelectedText(); got != "bc\nbc" {
		t.Fatalf("Ctrl+V selected a source envelope instead of two rows: %q", got)
	}
	if !h.e.Copy() {
		t.Fatal("native block copy failed")
	}
	if b, _ := h.clip.Read(transfer.Text); string(b) != "bc\nbc" {
		t.Fatalf("native Copy differs from painted block: %q", b)
	}
	h.e.imeClient.ReplaceText(nil, "d")
	if got := h.e.Source(); got != "ade\nade\nabcde" {
		t.Fatalf("block delete crossed unselected text/newlines: %q", got)
	}
	h.e.imeClient.ReplaceText(nil, "u")
	if h.e.Source() != src {
		t.Fatalf("block deletion was not one undo step: %q", h.e.Source())
	}
}

func TestNavigationAcceptanceChineseWordConsumers(t *testing.T) {
	const src = "今天我们学习中文分词。"
	for _, tc := range []struct {
		keys     string
		at, want int
	}{
		{"w", 0, len("今天")},
		{"e", 0, len("今")},
		{"2e", 0, len("今天我")},
		{"b", len("今天"), 0},
	} {
		h := newHarness(t, src)
		h.e.SetKeymap("vim")
		h.e.Select(tc.at, tc.at)
		for _, r := range tc.keys {
			h.e.imeClient.ReplaceText(nil, string(r))
		}
		if h.e.Caret() != tc.want {
			t.Fatalf("Chinese %s from byte %d landed at %d, want token boundary %d", tc.keys, tc.at, h.e.Caret(), tc.want)
		}
	}
	h := newHarness(t, src)
	a, z := h.e.wordRangeAt(strings.Index(src, "天"))
	if src[a:z] != "今天" {
		t.Fatalf("native word selection disagrees with Chinese navigation: %q", src[a:z])
	}
	h.e.SetKeymap("vim")
	h.e.Select(0, 0)
	h.e.imeClient.ReplaceText(nil, "d")
	h.e.imeClient.ReplaceText(nil, "e")
	if h.e.Source() != strings.TrimPrefix(src, "今天") {
		t.Fatalf("Chinese de used a different word endpoint: %q", h.e.Source())
	}
}

func TestNavigationAcceptanceWordMotionKeepsPunctuationAfterLinkDecoration(t *testing.T) {
	const src = "before [label](https://example.com),after"
	e := New(src, Options{})
	e.layoutIfNeeded()
	if got, want := e.step(strings.Index(src, "label"), 1, true), strings.Index(src, ","); got != want {
		t.Fatalf("skipping a zero-source link icon also skipped real punctuation: got=%d want=%d", got, want)
	}
	if got, want := e.nextWordEndAt(strings.Index(src, "label")), strings.Index(src, ",")+1; got != want {
		t.Fatalf("word-end motion skipped punctuation after a zero-source icon: got=%d want=%d", got, want)
	}
}

func TestChineseWordOperatorsShareDictionaryBoundaries(t *testing.T) {
	const src = "今天我们学习中文分词。"
	for _, tc := range []struct {
		keys string
		want string
	}{
		{"dw", strings.TrimPrefix(src, "今天")},
		{"de", strings.TrimPrefix(src, "今天")},
		{"ce", strings.TrimPrefix(src, "今天")},
		{"2dw", strings.TrimPrefix(src, "今天我们")},
	} {
		e := New(src, Options{})
		e.SetKeymap("vim")
		e.Select(0, 0)
		for _, r := range tc.keys {
			e.modalText(string(r))
		}
		if e.Source() != tc.want {
			t.Errorf("%s source = %q, want %q", tc.keys, e.Source(), tc.want)
		}
		if tc.keys == "ce" && e.vimController().Mode() != "Insert" {
			t.Errorf("ce should enter Insert mode, got %s", e.ModeLabel())
		}
	}
}

func TestNavigationAcceptanceTableVisualModes(t *testing.T) {
	const src = "| A | B | C |\n| --- | --- | --- |\n| one | two | three |\n| four | five | six |\n| seven | eight | nine |"
	for _, axis := range []tableAxis{tableRows, tableColumns} {
		h := newHarness(t, src)
		table := documentTestTable(t, h.e)
		h.e.SetKeymap("vim")
		at := table.tab.rows[1][1].start
		h.e.Select(at, at)
		if axis == tableRows {
			h.e.imeClient.ReplaceText(nil, "V")
		} else {
			h.key(ui.Ctrl, ui.KeyV)
		}
		if s := h.e.tableSelection; s == nil || axis == tableRows && (s.axis != tableRows || s.first != 1 || s.last != 1) || axis == tableColumns && (s.axis != tableCells || s.anchorRow != 1 || s.anchorCol != 1 || s.focusRow != 1 || s.focusCol != 1) || h.e.block != nil {
			t.Fatalf("table visual mode has wrong active shape axis=%d: %s", axis, tableSelectionState(h.e))
		}
		h.e.imeClient.ReplaceText(nil, "j")
		if s := h.e.tableSelection; s == nil || axis == tableRows && (s.axis != tableRows || s.first != 1 || s.last != 2) || axis == tableColumns && (s.axis != tableCells || s.anchorRow != 1 || s.anchorCol != 1 || s.focusRow != 2 || s.focusCol != 1) {
			t.Fatalf("vertical table motion lost semantic range axis=%d: %s", axis, tableSelectionState(h.e))
		}
		if axis == tableRows {
			for _, step := range []struct {
				key  string
				last int
			}{{"j", 3}, {"k", 2}, {"k", 1}, {"j", 2}} {
				h.e.imeClient.ReplaceText(nil, step.key)
				if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 1 || s.last != step.last {
					t.Fatalf("table row reversal after %s: %s", step.key, tableSelectionState(h.e))
				}
			}
		} else {
			h.e.imeClient.ReplaceText(nil, "l")
			if s := h.e.tableSelection; s == nil || s.axis != tableCells || s.anchorRow != 1 || s.anchorCol != 1 || s.focusRow != 2 || s.focusCol != 2 {
				t.Fatalf("cell region did not expand in both dimensions: %s", tableSelectionState(h.e))
			}
		}
		selected := h.e.SelectedText()
		if !h.e.Copy() {
			t.Fatal("table visual copy failed")
		}
		if html, _ := h.clip.Read(transfer.HTML); !strings.Contains(string(html), "<table>") {
			t.Fatalf("table visual copy produced a fragment axis=%d: %q", axis, selected)
		}
		h.e.imeClient.ReplaceText(nil, "d")
		table = documentTestTable(t, h.e)
		if axis == tableRows && len(table.tab.rows) != 2 || axis == tableColumns && (table.tab.cols() != 3 || len(table.tab.rows) != 4) {
			t.Fatalf("table visual delete damaged structure axis=%d: %q", axis, h.e.Source())
		}
		if !h.e.Undo() || h.e.Source() != src || h.e.tableSelection == nil || h.e.SelectedText() != selected {
			t.Fatalf("table visual undo lost shape/source axis=%d: %s", axis, tableSelectionState(h.e))
		}
		if axis == tableColumns && h.e.vimController().Mode() != "BlockVisual" || axis == tableRows && h.e.vimController().Mode() != "Visual" {
			t.Fatalf("table Undo restored a competing selection mode axis=%d: %s", axis, h.e.ModeLabel())
		}
	}
}

func TestNavigationAcceptanceBlockCodeDoesNotColorUnselectedCharacter(t *testing.T) {
	const src = "a `code` z\na `code` z"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.e.Select(3, 3) // c
	h.key(ui.Ctrl, ui.KeyV)
	h.e.imeClient.ReplaceText(nil, "l")
	h.e.imeClient.ReplaceText(nil, "j")
	h.frame()
	if got := h.e.SelectedText(); got != "co\nco" {
		t.Fatalf("unexpected code rectangle source range: %q", got)
	}
	if want := len("a `code` z\n") + 4; h.e.Caret() != want {
		t.Fatalf("code reveal moved the active block caret beyond o: caret=%d want=%d", h.e.Caret(), want)
	}
	baseline := newHarness(t, src)
	baseline.e.Select(3, 3)
	baseline.frame()
	r, ok := h.e.caretRect(5) // d in the first row, outside the block
	if !ok {
		t.Fatal("unselected character has no geometry")
	}
	for y := int(r.Y + 2); y < int(r.Y+r.H-2); y++ {
		for x := int(r.X + 1); x < int(r.X+3); x++ {
			p := h.tt.Image().RGBAAt(x, y)
			if want := baseline.tt.Image().RGBAAt(x, y); p != want {
				t.Fatalf("inline code padding paints the unselected d at %d,%d", x, y)
			}
		}
	}
}

func TestNavigationAcceptanceBlockBandCoversSelectedLineTop(t *testing.T) {
	h := newHarness(t, "a    z\na    z")
	h.e.SetKeymap("vim")
	h.e.Select(1, 1)
	h.key(ui.Ctrl, ui.KeyV)
	h.e.imeClient.ReplaceText(nil, "l")
	h.e.imeClient.ReplaceText(nil, "j")
	h.frame()
	r, ok := h.e.caretRect(1)
	if !ok {
		t.Fatal("selected space has no native geometry")
	}
	p := h.tt.Image().RGBAAt(int(r.X+2), int(r.Y+2))
	baseline := newHarness(t, "a    z\na    z")
	baseline.e.Select(1, 3)
	baseline.frame()
	want := baseline.tt.Image().RGBAAt(int(r.X+2), int(r.Y+2))
	if p != want {
		t.Fatalf("block background differs from native text selection at selected line top: pixel=%v want=%v rect=%+v", p, want, r)
	}
}

func TestNavigationAcceptanceBlockKeepsColumnAcrossSeparateShortLineMotions(t *testing.T) {
	h := newHarness(t, "abcdef\nx\nabcdef")
	h.e.SetKeymap("vim")
	h.e.Select(2, 2)
	h.key(ui.Ctrl, ui.KeyV)
	for _, key := range []string{"l", "j", "j"} {
		h.e.imeClient.ReplaceText(nil, key)
		h.frame()
	}
	if got := h.e.SelectedText(); got != "cd\n\ncd" {
		t.Fatalf("separate vertical motions lost the rectangle's geometric column: %q", got)
	}
	for _, key := range []string{"k", "k"} {
		h.e.imeClient.ReplaceText(nil, key)
		h.frame()
	}
	if got := h.e.SelectedText(); got != "cd" {
		t.Fatalf("reverse vertical motions did not restore the original block: %q", got)
	}
}
