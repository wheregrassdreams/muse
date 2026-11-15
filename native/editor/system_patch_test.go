package editor

import "testing"

func TestReplaceSystemAllFoldsIntoPriorUndoStep(t *testing.T) {
	h := newHarness(t, "---\nupdated_at: ''\n---\nbody")
	h.e.ReplaceAll("---\nupdated_at: ''\n---\nchanged body")
	if len(h.e.hist.undo) != 1 {
		t.Fatalf("user edit history = %d, want 1", len(h.e.hist.undo))
	}
	h.e.ReplaceSystemAll("---\nupdated_at: '2026-11-13T04:00:00Z'\n---\nchanged body")
	if len(h.e.hist.undo) != 1 {
		t.Fatalf("metadata created another undo step: %d", len(h.e.hist.undo))
	}
	if !h.e.Undo() || h.e.Source() != "---\nupdated_at: ''\n---\nbody" {
		t.Fatalf("Undo did not restore pre-edit source: %q", h.e.Source())
	}
}
