package mdedit

import (
	"math/rand/v2"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// TestRandomKeysKeepTheDocumentSound presses keys at random and checks what
// must hold after every one of them: the caret is a byte of the source, the
// leaves are whole, they come in order, no two lines share an offset, and
// what the view shows is text.
//
// It is the shape of the model, not the behaviour of a key, so it holds for
// every edit — which is what a bug in the model breaks.
func TestRandomKeysKeepTheDocumentSound(t *testing.T) {
	seeds := []string{
		"",
		"\n",
		"\n\n\n",
		"   \n",
		"a",
		"a\n\nb",
		"%%注释%% 正文",
		"正文前 %%这是注释%% 正文后",
		"> quote\n>\n> more",
		"- item\n\n- two",
		"# head\n\npara",
		"```\ncode\n```",
	}
	rng := rand.New(rand.NewPCG(11, 12))
	for _, src := range seeds {
		h := newHarness(t, src)
		for step := 0; step < 120; step++ {
			switch rng.IntN(10) {
			case 0:
				h.key(0, ui.KeyEnter)
			case 1:
				h.key(0, ui.KeyBackspace)
			case 2:
				h.key(0, ui.KeyDelete)
			case 3:
				h.key(0, ui.KeyLeft)
			case 4:
				h.key(0, ui.KeyRight)
			case 5:
				h.key(0, ui.KeyUp)
			case 6:
				h.key(0, ui.KeyDown)
			case 7:
				h.key(ui.Cmd, ui.KeyZ)
			case 8:
				h.typ("中")
			default:
				h.typ("x")
			}
			checkSound(t, seedName(src), step, h.e)
		}
	}
}

// seedName names a seed by its first line, for the failure message.
func seedName(src string) string {
	if src == "" {
		return "(empty)"
	}
	if len(src) > 16 {
		return src[:16] + "…"
	}
	return src
}

// checkSound checks the invariants of the document as it stands.
func checkSound(t *testing.T, name string, step int, e *Editor) {
	t.Helper()
	if e.caret < 0 || e.caret > len(e.src) || e.anchor < 0 || e.anchor > len(e.src) {
		t.Fatalf("%s step %d: caret %d .. anchor %d of %d bytes", name, step, e.caret, e.anchor, len(e.src))
	}
	prev := -1
	seen := map[int]bool{}
	for i, l := range e.leaves {
		if l.start < 0 || l.end < l.start || l.end > len(e.src) {
			t.Fatalf("%s step %d: leaf %d is %d..%d of %d", name, step, i, l.start, l.end, len(e.src))
		}
		if !onRuneStart(e.src, l.start) || !onRuneStart(e.src, l.end) {
			t.Fatalf("%s step %d: leaf %d %d..%d splits a character", name, step, i, l.start, l.end)
		}
		if l.start < prev {
			t.Fatalf("%s step %d: leaf %d starts at %d, before the leaf above it (%d)", name, step, i, l.start, prev)
		}
		prev = l.start
		if l.isLine() {
			if seen[l.start] {
				t.Fatalf("%s step %d: two lines at %d", name, step, l.start)
			}
			seen[l.start] = true
		}
		if l.view != nil && !utf8.ValidString(l.view.disp.text) {
			t.Fatalf("%s step %d: leaf %d shows broken text: %q", name, step, i, l.view.disp.text)
		}
	}
}

// onRuneStart reports whether b is the start of a character, or the end of
// the source.
func onRuneStart(src string, b int) bool {
	return b == 0 || b == len(src) || b > 0 && b < len(src) && utf8.RuneStart(src[b])
}
