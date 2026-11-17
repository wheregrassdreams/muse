package editor

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestModalNativeInputGuard(t *testing.T) {
	h := newHarness(t, "abc")
	h.e.SetKeymap("vim")
	h.e.Select(0, 0)
	for _, k := range []ui.Key{ui.KeyBackspace, ui.KeyDelete, ui.KeyEnter, ui.KeyTab} {
		h.key(0, k)
	}
	h.want("abc")
	h.e.imeClient.SetMarkedText(nil, "你好", ui.TextInputRange{Start: 2, End: 2})
	h.e.imeClient.UnmarkText()
	h.want("abc")
	h.e.imeClient.ReplaceText(nil, "i")
	h.e.imeClient.ReplaceText(nil, "你好")
	h.want("你好abc")
	h.key(0, ui.KeyEscape)
	h.e.imeClient.ReplaceText(nil, "d")
	h.e.imeClient.ReplaceText(nil, "w")
	if h.e.ModalAcceptsText() {
		t.Fatal("delete left insert active")
	}
}
func TestHelixSelectThenDelete(t *testing.T) {
	h := newHarness(t, "one two")
	h.e.SetKeymap("helix")
	h.e.Select(0, 0)
	h.e.imeClient.ReplaceText(nil, "w")
	if h.e.SelectedText() != "one " {
		t.Fatalf("selection=%q", h.e.SelectedText())
	}
	h.e.imeClient.ReplaceText(nil, "d")
	h.want("two")
	h.e.imeClient.ReplaceText(nil, "u")
	h.want("one two")
}
