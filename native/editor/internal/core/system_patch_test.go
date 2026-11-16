package core

import (
	"testing"

	"muse/editor/input"
)

func TestSystemPatchFoldsIntoPreviousUserEdit(t *testing.T) {
	initial := "---\nupdated_at: ''\n---\nbody"
	session := NewSession(initial)
	if err := session.SetSelection(Selection{Anchor: len(initial), Head: len(initial)}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Dispatch(input.Event{Kind: input.InsertText, Text: " changed"}); err != nil {
		t.Fatal(err)
	}
	patch, ok := session.PlanSystemPatch("---\nupdated_at: 'now'\n---\nbody changed")
	if !ok {
		t.Fatal("system patch was not planned")
	}
	if _, err := session.ApplySystemPatch(patch, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !session.CanUndo() {
		t.Fatal("user edit should remain undoable")
	}
	if _, err := session.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := session.Source(); got != initial {
		t.Fatalf("undo source = %q, want %q", got, initial)
	}
	if session.CanUndo() {
		t.Fatal("system patch became a separate undo step")
	}
}

func TestSystemPatchWithoutUserHistoryBecomesBaseline(t *testing.T) {
	session := NewSession("abc")
	patch, ok := session.PlanSystemPatch("aXbc")
	if !ok {
		t.Fatal("system patch was not planned")
	}
	if _, err := session.ApplySystemPatch(patch, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := session.Source(); got != "aXbc" {
		t.Fatalf("source = %q, want %q", got, "aXbc")
	}
	if session.CanUndo() {
		t.Fatal("system-only patch created user-visible undo history")
	}
}
