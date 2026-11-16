package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestVimBigWordMotions(t *testing.T) {
	const src = "foo,bar baz"
	for _, tc := range []struct {
		keys string
		at   int
		want int
	}{
		{"W", 0, 8},
		{"E", 0, 6},
		{"B", 10, 8},
		{"B", 8, 0},
		{"W", 4, 8},
	} {
		h := newHarness(t, src)
		h.e.SetKeymap("vim")
		h.caretAt(tc.at)
		sendVim(h, tc.keys)
		if h.e.caret != tc.want {
			t.Errorf("%s from %d landed at %d, want %d", tc.keys, tc.at, h.e.caret, tc.want)
		}
	}

	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.caretAt(0)
	sendVim(h, "w")
	if h.e.caret != 3 {
		t.Fatalf("lowercase w should stop at punctuation's word, got %d want 3", h.e.caret)
	}
}

func TestVimBigWordEndSupportsOperatorAndVisualModes(t *testing.T) {
	const src = "foo,bar baz"
	for _, tc := range []struct {
		keys string
		want string
	}{
		{"dW", "baz"},
		{"dE", " baz"},
	} {
		h := newHarness(t, src)
		h.e.SetKeymap("vim")
		h.caretAt(0)
		sendVim(h, tc.keys)
		if got := h.e.Source(); got != tc.want {
			t.Errorf("%s produced %q, want %q", tc.keys, got, tc.want)
		}
	}

	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.caretAt(0)
	sendVim(h, "vE")
	if got := h.e.SelectedText(); got != "foo,bar" {
		t.Fatalf("vE selected %q, want the full WORD", got)
	}
	if !h.e.modalKey(ui.KeyEscape, 0) {
		t.Fatal("could not leave Vim visual mode")
	}
}
