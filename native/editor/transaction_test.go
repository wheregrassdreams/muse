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

func TestProductionApplyTransactionRejectsInvalidUTF8Atomically(t *testing.T) {
	e := New("🙂x", Options{})
	before := e.Source()
	tests := []struct {
		name string
		tx   core.Transaction
	}{
		{
			name: "range splits source rune",
			tx: core.Transaction{
				Before: core.Selection{}, After: core.Selection{},
				Edits: []core.TextEdit{{Range: core.TextRange{Start: 1, End: 2}, Text: "x"}},
			},
		},
		{
			name: "replacement is invalid utf8",
			tx: core.Transaction{
				Before: core.Selection{}, After: core.Selection{},
				Edits: []core.TextEdit{{Range: core.TextRange{Start: 0, End: 0}, Text: string([]byte{0xff})}},
			},
		},
		{
			name: "result selection splits inserted rune",
			tx: core.Transaction{
				Before: core.Selection{}, After: core.Selection{Anchor: 1, Head: 1},
				Edits: []core.TextEdit{{Range: core.TextRange{Start: 0, End: 0}, Text: "🙂"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := e.ApplyTransaction(tt.tx); err == nil {
				t.Fatal("expected transaction to be rejected")
			}
			if got := e.Source(); got != before {
				t.Fatalf("rejected transaction changed source: %q", got)
			}
		})
	}
}
