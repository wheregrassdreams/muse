package editor

import (
	"testing"

	"muse/editor/internal/core"
)

func TestApplyTransactionIsAtomicAndUndoable(t *testing.T) {
	h := newHarness(t, "abcdef")
	before := h.e.TransactionSelection()
	tx := core.Transaction{
		Edits: []core.TextEdit{
			{Range: core.TextRange{Start: 1, End: 2}, Text: "XY"},
			{Range: core.TextRange{Start: 4, End: 6}, Text: "!"},
		},
		Before: before,
		After:  core.Selection{Anchor: 4, Head: 4},
	}
	if err := h.e.ApplyTransaction(tx); err != nil {
		t.Fatal(err)
	}
	if got, want := h.e.Source(), "aXYcd!"; got != want {
		t.Fatalf("source after transaction = %q, want %q", got, want)
	}
	if !h.e.Undo() {
		t.Fatal("transaction was not undoable")
	}
	if got, want := h.e.Source(), "abcdef"; got != want {
		t.Fatalf("source after undo = %q, want %q", got, want)
	}
	if got := h.e.TransactionSelection(); got != before {
		t.Fatalf("selection after undo = %+v, want %+v", got, before)
	}
}

func TestApplyTransactionRejectsStaleSelection(t *testing.T) {
	h := newHarness(t, "abc")
	selectBefore := h.e.TransactionSelection()
	h.e.Select(1, 2)
	tx := core.Transaction{
		Edits:  []core.TextEdit{{Range: core.TextRange{Start: 1, End: 2}, Text: "X"}},
		Before: selectBefore,
		After:  selectBefore,
	}
	if err := h.e.ApplyTransaction(tx); err == nil {
		t.Fatal("stale transaction selection was accepted")
	}
	if got, want := h.e.Source(), "abc"; got != want {
		t.Fatalf("source changed after rejected transaction: got %q want %q", got, want)
	}
}
