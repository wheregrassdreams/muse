package mdedit

import "testing"

func TestModalEditingSequence(t *testing.T) {
	e := New("alpha beta\n中文", Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	e.modalText("d")
	e.modalText("w")
	if e.Source() != "beta\n中文" {
		t.Fatal(e.Source())
	}
	e.modalText("u")
	if e.Source() != "alpha beta\n中文" {
		t.Fatal(e.Source())
	}
	e.modalText("c")
	e.modalText("c")
	if e.modal.mode != "Insert" {
		t.Fatal(e.ModeLabel())
	}
	if e.modalText("你好") {
		t.Fatal("insert text intercepted")
	}
	e.typeText("你好")
	if e.Source() != "你好\n中文" {
		t.Fatal(e.Source())
	}
}
func TestModalRejectsCompositionPayload(t *testing.T) {
	e := New("中文", Options{})
	e.SetKeymap("helix")
	if e.ModalAcceptsText() {
		t.Fatal("normal accepts IME")
	}
	e.modalText("你好")
	if e.Source() != "中文" {
		t.Fatal(e.Source())
	}
}

func TestModalNativeSequences(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{{"dw", "two\nthree\n"}, {"dd", "three\n"}, {"yyp", "one two\none two\nthree\n"}, {"2dd", ""}, {"vld", "e two\nthree\n"}} {
		e := New("one two\nthree\n", Options{})
		e.SetKeymap("vim")
		e.Select(0, 0)
		for _, r := range tc.keys {
			e.ReplaceText(nil, string(r))
		}
		if e.Source() != tc.want {
			t.Fatalf("%s: %q want %q", tc.keys, e.Source(), tc.want)
		}
	}
}
