package editor

import (
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func sendVim(h *harness, keys string) {
	h.t.Helper()
	for _, r := range keys {
		h.e.ReplaceText(nil, string(r))
	}
	h.frame()
}

func TestVimLinewiseSelectionIsNativeAndMoves(t *testing.T) {
	h := newHarness(t, "alpha\nbeta\ngamma")
	h.e.SetKeymap("vim")
	h.caretAt(1)
	before, ok := h.e.caretRect(h.e.Caret())
	if !ok {
		t.Fatal("missing caret geometry before V")
	}
	sendVim(h, "V")
	current, ok := h.e.caretRect(h.e.Caret())
	if !ok || current.Y != before.Y {
		t.Fatalf("V moved caret off its source line: before=%+v after=%+v", before, current)
	}
	if got := h.e.SelectedText(); got != "alpha\n" {
		t.Fatalf("V selection = %q, want entire first source line", got)
	}
	if os.Getenv("MDEDIT_SHOTS") != "" {
		if err := os.MkdirAll(os.Getenv("MDEDIT_SHOTS"), 0o755); err != nil {
			t.Fatal(err)
		}
		h.shot("vim-linewise")
	}
	sendVim(h, "j")
	current, ok = h.e.caretRect(h.e.Caret())
	if !ok || current.Y <= before.Y {
		t.Fatalf("Vj did not put caret on the second selected line: before=%+v after=%+v", before, current)
	}
	if got := h.e.SelectedText(); got != "alpha\nbeta\n" {
		t.Fatalf("Vj selection = %q, want first two source lines", got)
	}
	sendVim(h, "k")
	if got := h.e.SelectedText(); got != "alpha\n" {
		t.Fatalf("Vk selection = %q, want first source line", got)
	}
}

func TestVimNewNativeCommandsAndUndo(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"^x", "bc\ndef"},
		{"I!", "!abc\ndef"},
		{"A!", "abc!\ndef"},
		{"O!", "!\nabc\ndef"},
		{"o!", "abc\n!\ndef"},
		{"x", "ac\ndef"},
		{"X", "bc\ndef"},
		{"D", "a\ndef"},
		{"C!", "a!\ndef"},
	} {
		h := newHarness(t, "abc\ndef")
		h.e.SetKeymap("vim")
		h.caretAt(1)
		sendVim(h, tc.keys)
		if got := h.e.Source(); got != tc.want {
			t.Errorf("%s source = %q, want %q", tc.keys, got, tc.want)
		}
	}
}

func TestVimVerticalGoalResetsAfterHorizontalMotion(t *testing.T) {
	src := "0123456789 abcdefghijklmnopqrstuvwxyz 0123456789 abcdefghijklmnopqrstuvwxyz 中文混排。" +
		"0123456789 abcdefghijklmnopqrstuvwxyz 0123456789 abcdefghijklmnopqrstuvwxyz 中文混排。" +
		"0123456789 abcdefghijklmnopqrstuvwxyz 0123456789 abcdefghijklmnopqrstuvwxyz 中文混排。"
	h := newHarness(t, src)
	h.e.opts.MaxWidth = 450
	h.e.invalidate()
	h.frame()
	h.e.SetKeymap("vim")
	h.caretAt(38)
	h.e.layoutIfNeeded()
	initial, ok := h.e.caretRect(h.e.Caret())
	if !ok || initial.Y < 0 {
		t.Fatal("caret has no visible layout geometry")
	}
	sendVim(h, "j")
	h.e.layoutIfNeeded()
	afterJ, _ := h.e.caretRect(h.e.Caret())
	if afterJ.Y <= initial.Y {
		t.Fatalf("j did not move to a lower display row: before=%v after=%v", initial.Y, afterJ.Y)
	}
	sendVim(h, "h")
	h.e.layoutIfNeeded()
	current, _ := h.e.caretRect(h.e.Caret())
	if current.X >= afterJ.X {
		t.Fatalf("h did not move caret left: before=%v current=%v", afterJ.X, current.X)
	}
	wantX := current.X
	sendVim(h, "j")
	h.e.layoutIfNeeded()
	got, _ := h.e.caretRect(h.e.Caret())
	if diff := got.X - wantX; diff < -10 || diff > 10 {
		t.Fatalf("after h then j x=%v, want preserved current x=%v", got.X, wantX)
	}
	if os.Getenv("MDEDIT_SHOTS") != "" {
		if err := os.MkdirAll(os.Getenv("MDEDIT_SHOTS"), 0o755); err != nil {
			t.Fatal(err)
		}
		h.shot("vim-geometric-jk")
	}
}

func TestVimLinewiseCountReverseAndLastLine(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		h := newHarness(t, "alpha\nbeta\ngamma\ndelta")
		h.e.SetKeymap("vim")
		h.caretAt(1)
		sendVim(h, "V2j")
		if got := h.e.SelectedText(); got != "alpha\nbeta\ngamma\n" {
			t.Fatalf("V2j selection = %q", got)
		}
	})
	t.Run("reverse across anchor", func(t *testing.T) {
		h := newHarness(t, "alpha\nbeta\ngamma\ndelta")
		h.e.SetKeymap("vim")
		h.caretAt(7)
		sendVim(h, "V2k")
		if got := h.e.SelectedText(); got != "alpha\nbeta\n" {
			t.Fatalf("V2k selection = %q", got)
		}
	})
	t.Run("last line without newline", func(t *testing.T) {
		h := newHarness(t, "alpha\nbeta\ngamma")
		h.e.SetKeymap("vim")
		h.caretAt(12)
		sendVim(h, "V")
		if got := h.e.SelectedText(); got != "gamma" {
			t.Fatalf("V selection = %q", got)
		}
	})
}

func TestVimCountBeforeLinewiseVisualSelectsRows(t *testing.T) {
	h := newHarness(t, "one\ntwo\nthree\nfour")
	h.e.SetKeymap("vim")
	h.caretAt(1)
	sendVim(h, "3V")
	if got := h.e.SelectedText(); got != "one\ntwo\nthree\n" {
		t.Fatalf("3V selection = %q, want three complete source lines", got)
	}
}

func TestVimInsertCommandsUseTableSelectionFocus(t *testing.T) {
	for _, command := range []string{"I", "A", "o", "O"} {
		t.Run(command, func(t *testing.T) {
			src := "before\n| a | b |\n| - | - |\n| c | d |\nafter"
			h := newHarness(t, src)
			h.e.SetKeymap("vim")
			h.e.layoutIfNeeded()
			var table *leaf
			for _, l := range h.e.leaves {
				if l.kind == kTable {
					table = l
					break
				}
			}
			if table == nil {
				t.Fatal("table did not parse")
			}
			h.e.selectTableAxis(table, tableRows, 1, 1, false)
			focus := h.e.selectionFocus()
			lineStart, lineEnd := h.e.sourceLineRange(focus)
			want := lineStart
			switch command {
			case "I":
				want = h.e.nativeLineStart(focus, true)
			case "A":
				want = lineEnd
			case "o":
				want = lineEnd + 1
			case "O":
				want = lineStart
			}
			h.e.setSessionSelection(len(src), len(src))
			h.e.modalText(command)
			if h.e.Caret() != want {
				t.Fatalf("caret=%d, want structural selection focus-derived offset %d", h.e.Caret(), want)
			}
			if h.e.tableSelection != nil || h.e.tableBridge != nil {
				t.Fatalf("%s left structural table state active", command)
			}
			if h.e.vimController().Mode() != "Insert" {
				t.Fatalf("%s mode=%q, want Insert", command, h.e.vimController().Mode())
			}
		})
	}
}

func TestVimLinewiseEditClipboardAndHistory(t *testing.T) {
	h := newHarness(t, "alpha\nbeta\ngamma")
	h.e.SetKeymap("vim")
	h.caretAt(7)
	sendVim(h, "Vd")
	h.want("alpha\ngamma")
	sendVim(h, "u")
	h.want("alpha\nbeta\ngamma")
	// Copy uses the harness-local clipboard; no system clipboard is read or written.
	h.caretAt(1)
	sendVim(h, "Vy")
	sendVim(h, "p")
	h.want("alpha\nalpha\nbeta\ngamma")
	sendVim(h, "u")
	h.want("alpha\nbeta\ngamma")

	h.caretAt(1)
	sendVim(h, "Vc")
	h.want("beta\ngamma")
	sendVim(h, "x")
	h.key(0, ui.KeyEscape)
	h.want("xbeta\ngamma")
	sendVim(h, "u")
	h.want("beta\ngamma")
	sendVim(h, "u")
	h.want("alpha\nbeta\ngamma")
}

func TestVimEditingCommandsUndoAndRedo(t *testing.T) {
	h := newHarness(t, "abc\ndef")
	h.e.SetKeymap("vim")
	h.caretAt(0)
	sendVim(h, "x")
	h.want("bc\ndef")
	sendVim(h, "u")
	h.want("abc\ndef")
	h.key(ui.Ctrl, ui.KeyR)
	h.want("bc\ndef")
}

func TestVimLinewiseModeToggleEscapeAndEmptyLine(t *testing.T) {
	h := newHarness(t, "alpha\n\nbeta")
	h.e.SetKeymap("vim")
	h.caretAt(6)
	sendVim(h, "V")
	if got := h.e.SelectedText(); got != "\n" {
		t.Fatalf("empty-line selection = %q", got)
	}
	sendVim(h, "v")
	if h.e.vimController().Sequence().Linewise || h.e.vimController().Mode() != "Visual" {
		t.Fatalf("V to v did not leave linewise mode: %#v", h.e.modal)
	}
	h.key(0, ui.KeyEscape)
	if h.e.vimController().Sequence().Linewise || h.e.vimController().Mode() != "Normal" {
		t.Fatalf("Escape left visual state: %#v", h.e.modal)
	}
	sendVim(h, "V")
	h.key(0, ui.KeyEscape)
	if h.e.vimController().Sequence().Linewise || h.e.vimController().Mode() != "Normal" {
		t.Fatalf("Escape left linewise state: %#v", h.e.modal)
	}
}

func TestVimCommandsDoNotConsumeIMECommit(t *testing.T) {
	h := newHarness(t, "中文")
	h.e.SetKeymap("vim")
	h.caretAt(0)
	h.e.ReplaceText(nil, "你好")
	h.want("中文")
}
