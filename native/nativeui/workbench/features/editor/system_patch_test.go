package editor

import "testing"

func TestReplaceSystemAllFoldsIntoPriorUndoStep(t *testing.T) {
	h := newHarness(t, "---\nupdated_at: ''\n---\nbody")
	h.e.ReplaceAll("---\nupdated_at: ''\n---\nchanged body")
	if !h.e.Session.CanUndo() {
		t.Fatal("user edit should be undoable")
	}
	h.e.ReplaceSystemAll("---\nupdated_at: '2026-11-13T04:00:00Z'\n---\nchanged body")
	if !h.e.Session.CanUndo() {
		t.Fatal("metadata update removed the user edit from history")
	}
	if !h.e.Undo() || h.e.Source() != "---\nupdated_at: ''\n---\nbody" {
		t.Fatalf("Undo did not restore pre-edit source: %q", h.e.Source())
	}
}
