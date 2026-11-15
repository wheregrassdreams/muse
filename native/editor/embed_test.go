package mdedit

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
	h.caretAt(4)
	if !h.shows("![[图.png]]") {
		t.Fatalf("the embed is hidden at the caret: %q", h.e.leaves[0].view.disp.text)
	}
}
