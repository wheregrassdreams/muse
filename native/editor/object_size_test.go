package mdedit

import (
	"strings"
	"testing"
)

// TestASizelessObjectDrawsNothing: an object with no box — a picture that
// loaded with no pixels, a formula with no bitmap — draws nothing. Its
// replacement character must not reach the text: the shaper draws it as a
// glyph when it has no box, and a font that has none draws the empty square
// that once stood where the picture was.
func TestASizelessObjectDrawsNothing(t *testing.T) {
	for _, c := range []struct {
		name string
		obj  *object
	}{
		{"no width", &object{w: 0, h: 20, alt: "图"}},
		{"no height", &object{w: 20, h: 0, alt: "图"}},
		{"nothing at all", &object{}},
	} {
		b := &builder{e: New("x", Options{FontSize: 16})}
		b.atom(0, 1, "", istyle{}, c.obj)
		if len(b.pieces) != 1 {
			t.Fatalf("%s: %d pieces", c.name, len(b.pieces))
		}
		pc := b.pieces[0]
		if pc.obj != nil || b.hasObjects {
			t.Fatalf("%s: the object stayed on the piece", c.name)
		}
		if strings.ContainsRune(pc.text, 0xFFFC) {
			t.Fatalf("%s: the text carries the replacement character", c.name)
		}
	}
	// An object with a box is drawn, as before.
	b := &builder{e: New("x", Options{FontSize: 16})}
	b.atom(0, 1, "", istyle{}, &object{w: 20, h: 10})
	if len(b.pieces) != 1 || b.pieces[0].obj == nil || !b.hasObjects {
		t.Fatalf("an object with a box was dropped: %+v", b.pieces)
	}
	if b.pieces[0].span.ObjectWidth != 20 {
		t.Fatalf("the span has no box: %+v", b.pieces[0].span)
	}
}
