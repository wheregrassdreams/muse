package editor

import "testing"

func TestImageEmbedDrawsThePicture(t *testing.T) {
	// A picture embed becomes an object; a note embed stays a label.
	if got := shown(t, "![[图.png]]\n"); len(got) != 1 || got[0] != "[obj]" {
		t.Errorf("an image embed shows %q", got)
	}
	if got := shown(t, "![[笔记]]\n"); len(got) != 1 || got[0] != "«a:笔记»" {
		t.Errorf("a note embed shows %q", got)
	}
	// The caret at it shows the source, so it can be edited.
	h := newHarness(t, "![[图.png]]")
	// The caret is measured in UTF-8 byte offsets and must land on a rune
	// boundary inside the embed's source.
	h.caretAt(6)
	if h.e.Caret() != 6 {
		t.Fatalf("caret offset = %d, want a valid in-embed rune boundary", h.e.Caret())
	}
	if !h.shows("![[图.png]]") {
		t.Fatalf("the embed is hidden at the caret: %q", h.e.leaves[0].view.disp.text)
	}
}
