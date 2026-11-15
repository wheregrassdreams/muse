package mdedit

import (
	"math/rand/v2"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestU16IndexFollowsEdits: the input method's offsets are read from an
// index the edits move, so it has to say what a fresh reading of the source
// says, at every boundary of it.
func TestU16IndexFollowsEdits(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	src := "一段文字，with ascii, 还有 emoji 😀 和另一个字 😀😀 end。\n"
	e := New(src, Options{FontSize: 16})
	texts := []string{"x", "中文", "😀", "\n", "ab", "", "é", "🎉🎉", "a longer piece of text inserted"}
	for step := 0; step < 300; step++ {
		from := e.runeStart(rng.IntN(len(e.src) + 1))
		to := min(from+rng.IntN(7), len(e.src))
		to = e.runeStart(to)
		if rng.IntN(2) == 0 {
			to = from
		}
		text := texts[rng.IntN(len(texts))]
		e.begin(stepEdit)
		e.replace(from, to, text)
		e.commit(from, from)
		// The index the edit moved has to hold the source it moved with.
		ix := e.u16()
		if len(ix.bytes) == 0 || ix.bytes[0] != 0 || ix.bytes[len(ix.bytes)-1] != len(e.src) {
			t.Fatalf("step %d: edit %d..%d %q leaves the index from %v to %d of %d",
				step, from, to, text, ix.bytes, ix.bytes[len(ix.bytes)-1], len(e.src))
		}
		for k := 1; k < len(ix.bytes); k++ {
			if ix.bytes[k] <= ix.bytes[k-1] {
				t.Fatalf("step %d: edit %d..%d %q: entries %d and %d are out of order: %v",
					step, from, to, text, ix.bytes[k-1], ix.bytes[k], ix.bytes)
			}
		}

		for _, b := range e.runeStarts() {
			if got, want := e.unitsOf(b), ui.UTF16Len(e.src[:b]); got != want {
				t.Fatalf("step %d: units of byte %d is %d, want %d (source %q)", step, b, got, want, e.src)
			}
		}
		// And back: a unit offset reads the byte it starts at.
		u := 0
		for i, r := range e.src {
			if got := e.byteOf(u); got != i {
				t.Fatalf("step %d: byte of unit %d is %d, want %d", step, u, got, i)
			}
			if r >= 0x10000 {
				u += 2
			} else {
				u++
			}
		}
	}
}

// runeStart moves b back to the byte its rune starts at.
func (e *Editor) runeStart(b int) int {
	b = max(0, min(b, len(e.src)))
	for b > 0 && b < len(e.src) && !utf8Start(e.src[b]) {
		b--
	}
	return b
}

// runeStarts returns the byte offsets the source's runes start at, and its
// length.
func (e *Editor) runeStarts() []int {
	out := make([]int, 0, len(e.src)+1)
	for i := range e.src {
		if utf8Start(e.src[i]) {
			out = append(out, i)
		}
	}
	return append(out, len(e.src))
}

// utf8Start reports whether b is the first byte of a rune.
func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
