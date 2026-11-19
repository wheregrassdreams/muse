package inputadapter

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/wheregrassdreams/muse-editor/text"
)

func TestKeyEventMapsLogicalKeysAndModifiers(t *testing.T) {
	got, ok := KeyEvent(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyLeft, Mods: ui.Shift | ui.Cmd, Repeat: true})
	if !ok || got.Key != "ArrowLeft" || got.Modifiers != text.ModShift|text.ModMeta || !got.Repeat {
		t.Fatalf("KeyEvent() = %+v, %v", got, ok)
	}
}

func TestTextEventKeepsCommittedSpaceAsText(t *testing.T) {
	got, ok := TextEvent(ui.InputEvent{Kind: ui.InputText, Text: " "})
	if !ok || got.Text != " " || got.Key != "" {
		t.Fatalf("TextEvent() = %+v, %v", got, ok)
	}
	if _, ok := TextEvent(ui.InputEvent{Kind: ui.InputCompose, Text: "x"}); ok {
		t.Fatal("composition event escaped the IME boundary")
	}
}

func TestShortcutRespectsPlatformCommandKey(t *testing.T) {
	if got := Shortcut(text.KeyEvent{Key: "Z", Modifiers: text.ModMeta}, true); got != ShortcutUndo {
		t.Fatalf("mac command-z = %v", got)
	}
	if got := Shortcut(text.KeyEvent{Key: "Z", Modifiers: text.ModControl}, false); got != ShortcutUndo {
		t.Fatalf("control-z = %v", got)
	}
	if got := Shortcut(text.KeyEvent{Key: "Z", Modifiers: text.ModControl}, true); got != ShortcutNone {
		t.Fatalf("control-z on mac = %v", got)
	}
}
