package inputadapter

import (
	"testing"

	"github.com/egoist/mygo/ui"
	editorinput "muse/editor/input"
)

func TestKeyEventNormalizesKeyAndModifiers(t *testing.T) {
	got, ok := KeyEvent(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyA, Mods: ui.Ctrl | ui.Shift})
	if !ok {
		t.Fatal("known key was rejected")
	}
	if got.Key != editorinput.KeyA || got.Modifiers != editorinput.ModCtrl|editorinput.ModShift {
		t.Fatalf("normalized event = %+v", got)
	}
}

func TestTextEventAcceptsCommittedTextOnly(t *testing.T) {
	got, ok := TextEvent(ui.InputEvent{Kind: ui.InputText, Text: "你好"})
	if !ok || got.Kind != editorinput.InsertText || got.Text != "你好" {
		t.Fatalf("committed text = %+v, %v", got, ok)
	}
	if _, ok := TextEvent(ui.InputEvent{Kind: ui.InputCompose, Text: "中"}); ok {
		t.Fatal("composition was treated as committed text")
	}
}
