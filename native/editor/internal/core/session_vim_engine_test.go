package core

import (
	"testing"

	"github.com/wheregrassdreams/muse-editor/text"
)

func TestSessionVimKeymapAndCommandsUseAuthoritativeTextEngine(t *testing.T) {
	session := NewSession("a\nb\nc")
	if err := session.SetSelection(Selection{Anchor: 2, Head: 2}); err != nil {
		t.Fatal(err)
	}
	var final text.Intent
	for _, key := range []string{"d", "d"} {
		intent, handled := session.HandleVimKey(text.KeyEvent{Text: key})
		if !handled {
			t.Fatalf("Vim key %q was not handled", key)
		}
		if intent.Name != "" {
			final = intent
		}
	}
	if final.Name != "vim.delete-range" || !final.HasRange {
		t.Fatalf("dd intent = %+v", final)
	}
	handled, err := session.ExecuteVimIntent(final)
	if err != nil || !handled {
		t.Fatalf("execute dd = handled %t, err %v", handled, err)
	}
	if got := session.Source(); got != "a\nc" {
		t.Fatalf("dd source = %q, want %q", got, "a\nc")
	}
	if !session.CanUndo() {
		t.Fatal("dd did not create an engine-owned history entry")
	}
	if _, err := session.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := session.Source(); got != "a\nb\nc" {
		t.Fatalf("undo dd source = %q", got)
	}
}
