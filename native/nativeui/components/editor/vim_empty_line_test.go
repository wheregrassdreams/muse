package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestVimHorizontalMotionStaysOnEmptyLine(t *testing.T) {
	h := newHarness(t, "alpha\n\nbeta")
	h.e.SetKeymap("vim")
	h.caretAt(6)

	for _, key := range []string{"l", "h"} {
		sendVim(h, key)
		if h.e.Caret() != 6 {
			t.Fatalf("%s moved out of the empty line to offset %d", key, h.e.Caret())
		}
	}
	for _, key := range []ui.Key{ui.KeyRight, ui.KeyLeft} {
		h.key(0, key)
		if h.e.Caret() != 6 {
			t.Fatalf("arrow key %v moved out of the empty line to offset %d", key, h.e.Caret())
		}
	}
	if got := h.e.Source(); got != "alpha\n\nbeta" {
		t.Fatalf("horizontal motion changed source to %q", got)
	}
}

func TestTypingSpaceOnEmptySourceLineKeepsCaretOnThatLine(t *testing.T) {
	for _, tc := range []struct {
		name string
		vim  bool
	}{
		{name: "regular typing"},
		{name: "Vim insert mode", vim: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, "alpha\n\nbeta")
			if tc.vim {
				h.e.SetKeymap("vim")
				sendVim(h, "a")
			}
			h.caretAt(6)
			h.typ(" ")
			if got := h.e.Source(); got != "alpha\n \nbeta" {
				t.Fatalf("space on blank line produced %q", got)
			}
			if h.e.Caret() != 7 {
				t.Fatalf("space advanced caret to %d, want 7 on the same line", h.e.Caret())
			}
			h.key(0, ui.KeyBackspace)
			if got := h.e.Source(); got != "alpha\n\nbeta" {
				t.Fatalf("Backspace after typing on blank line produced %q, want the blank line preserved", got)
			}
			if h.e.Caret() != 6 {
				t.Fatalf("Backspace left caret at %d, want 6", h.e.Caret())
			}
		})
	}
}

func TestDeletingWhitespaceInsideBlankLinePreservesLineUntilCharactersAreGone(t *testing.T) {
	t.Run("backspace removes source characters first", func(t *testing.T) {
		h := newHarness(t, "alpha\n\nbeta")
		h.caretAt(6)
		h.typ("  ")
		h.key(0, ui.KeyBackspace)
		h.want("alpha\n \nbeta")
		if h.e.Caret() != 7 {
			t.Fatalf("caret after removing one space = %d, want 7", h.e.Caret())
		}
		h.key(0, ui.KeyBackspace)
		h.want("alpha\n\nbeta")
	})
	t.Run("delete removes source characters first", func(t *testing.T) {
		h := newHarness(t, "alpha\n \nbeta")
		h.caretAt(6)
		h.key(0, ui.KeyDelete)
		h.want("alpha\n\nbeta")
		if h.e.Caret() != 6 {
			t.Fatalf("caret after Delete = %d, want 6", h.e.Caret())
		}
	})
}

func TestVimLinewiseVisualOnEmptyLineDoesNotWrapHorizontally(t *testing.T) {
	h := newHarness(t, "alpha\n\nbeta")
	h.e.SetKeymap("vim")
	h.caretAt(6)
	sendVim(h, "V")
	if got := h.e.SelectedText(); got != "\n" {
		t.Fatalf("V selected %q, want the empty source line", got)
	}
	sendVim(h, "l")
	if got := h.e.SelectedText(); got != "\n" || h.e.Caret() != 6 {
		t.Fatalf("Vl changed empty-line selection to %q with caret %d", got, h.e.Caret())
	}
}

func TestVimHorizontalOperatorDoesNotCrossEmptyLine(t *testing.T) {
	h := newHarness(t, "alpha\n\nbeta")
	h.e.SetKeymap("vim")
	h.caretAt(6)
	sendVim(h, "dl")
	if got := h.e.Source(); got != "alpha\n\nbeta" {
		t.Fatalf("dl on an empty line changed source to %q", got)
	}
	if h.e.Caret() != 6 {
		t.Fatalf("dl moved caret out of the empty line to %d", h.e.Caret())
	}
}

func TestVimEContinuesAcrossEmptySourceUnits(t *testing.T) {
	t.Run("normal motion from blank line", func(t *testing.T) {
		h := newHarness(t, "alpha\n\nbeta")
		h.e.SetKeymap("vim")
		h.caretAt(6)
		sendVim(h, "e")
		if h.e.Caret() != 10 {
			t.Fatalf("e from empty line landed at %d, want last grapheme of beta at 10", h.e.Caret())
		}
	})

	t.Run("operator motion from blank line", func(t *testing.T) {
		h := newHarness(t, "alpha\n\nbeta")
		h.e.SetKeymap("vim")
		h.caretAt(6)
		sendVim(h, "de")
		if got := h.e.Source(); got != "alpha\n" {
			t.Fatalf("de from empty line produced %q, want %q", got, "alpha\n")
		}
	})
}

func TestVimAppendOnEmptyLineDoesNotJumpToNextLine(t *testing.T) {
	h := newHarness(t, "alpha\n\nbeta")
	h.e.SetKeymap("vim")
	h.caretAt(6)
	sendVim(h, "a")
	if h.e.vimController().Mode() != "Insert" || h.e.Caret() != 6 {
		t.Fatalf("a on empty line entered insert at mode=%q offset=%d", h.e.vimController().Mode(), h.e.Caret())
	}
	sendVim(h, "X")
	h.key(0, ui.KeyEscape)
	if got := h.e.Source(); got != "alpha\nX\nbeta" {
		t.Fatalf("aX on empty line produced %q", got)
	}
}

func TestVimCharacterPutOnEmptyLineAndAfterGrapheme(t *testing.T) {
	t.Run("empty line", func(t *testing.T) {
		h := newHarness(t, "alpha\n\nbeta")
		h.e.SetKeymap("vim")
		h.caretAt(6)
		h.e.vimController().SetTextRegister("X", false)
		sendVim(h, "p")
		if got := h.e.Source(); got != "alpha\nX\nbeta" {
			t.Fatalf("p on empty line produced %q", got)
		}
	})

	t.Run("extended grapheme", func(t *testing.T) {
		h := newHarness(t, "éx")
		h.e.SetKeymap("vim")
		h.caretAt(0)
		h.e.vimController().SetTextRegister("!", false)
		sendVim(h, "p")
		if got := h.e.Source(); got != "é!x" {
			t.Fatalf("p split an extended grapheme: %q", got)
		}
	})
}

func TestVimLinewiseYankAndPutPreservesEmptyLines(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
	}{
		{name: "trailing empty line", source: "alpha\n", want: "alpha\n\n"},
		{name: "empty document", source: "", want: "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, tc.source)
			h.e.SetKeymap("vim")
			h.caretAt(len(tc.source))
			sendVim(h, "yy")
			register := h.e.vimController().View()
			if register.Register != "" || !register.RegisterValid || !register.RegisterLine {
				t.Fatalf("empty line register = %q valid=%v linewise=%v", register.Register, register.RegisterValid, register.RegisterLine)
			}
			sendVim(h, "p")
			if got := h.e.Source(); got != tc.want {
				t.Fatalf("yy p produced %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVimLinewisePutSeparatesUnterminatedRegisterLines(t *testing.T) {
	for _, tc := range []struct {
		name, key, want string
	}{
		{name: "before current line", key: "P", want: "beta\nalpha\nbeta"},
		{name: "after current line", key: "p", want: "alpha\nbeta\nbeta"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, "alpha\nbeta")
			h.e.SetKeymap("vim")
			h.caretAt(6)
			sendVim(h, "yy")
			h.caretAt(0)
			sendVim(h, tc.key)
			if got := h.e.Source(); got != tc.want {
				t.Fatalf("yy %s produced %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}
