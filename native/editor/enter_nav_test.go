package mdedit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEnterOnABlankLineAddsOne(t *testing.T) {
	for _, tc := range []struct {
		name      string
		src       string
		at        int
		want      string
		wantCaret int
	}{
		{"between blocks", "a\n\nb", 2, "a\n\n\nb", 3},
		{"at the end", "a\n", 2, "a\n\n", 3},
		{"empty note", "", 0, "\n", 1},
	} {
		h := newHarness(t, tc.src)
		h.caretAt(tc.at)
		h.key(0, ui.KeyEnter)
		if got := h.e.Source(); got != tc.want {
			t.Errorf("%s: Enter gives %q, want %q", tc.name, got, tc.want)
		}
		if h.e.caret != tc.wantCaret {
			t.Errorf("%s: caret at %d, want %d", tc.name, h.e.caret, tc.wantCaret)
		}
	}
}
