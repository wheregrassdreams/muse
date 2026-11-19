package editor

import (
	"testing"

	editormodel "github.com/wheregrassdreams/muse-editor/text/session"
)

func TestApplyTransactionIsAtomicAndUndoable(t *testing.T) {
	h := newHarness(t, "abcdef")
	before := h.e.Session.Selection()
	tx := editormodel.Transaction{
		Edits: []editormodel.TextEdit{
			{Range: editormodel.TextRange{Start: 1, End: 2}, Text: "XY"},
			{Range: editormodel.TextRange{Start: 4, End: 6}, Text: "!"},
		},
		Before: before,
		After:  editormodel.Selection{Anchor: 4, Head: 4},
	}
	if err := (editorTableHost{editor: h.e}).Apply(tx); err != nil {
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
	if got := h.e.Session.Selection(); got != before {
		t.Fatalf("selection after undo = %+v, want %+v", got, before)
	}
}

func TestApplyTransactionRejectsStaleSelection(t *testing.T) {
	h := newHarness(t, "abc")
	selectBefore := h.e.Session.Selection()
	h.e.Select(1, 2)
	tx := editormodel.Transaction{
		Edits:  []editormodel.TextEdit{{Range: editormodel.TextRange{Start: 1, End: 2}, Text: "X"}},
		Before: selectBefore,
		After:  selectBefore,
	}
	if err := (editorTableHost{editor: h.e}).Apply(tx); err == nil {
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
		tx   editormodel.Transaction
	}{
		{
			name: "range splits source rune",
			tx: editormodel.Transaction{
				Before: editormodel.Selection{}, After: editormodel.Selection{},
				Edits: []editormodel.TextEdit{{Range: editormodel.TextRange{Start: 1, End: 2}, Text: "x"}},
			},
		},
		{
			name: "replacement is invalid utf8",
			tx: editormodel.Transaction{
				Before: editormodel.Selection{}, After: editormodel.Selection{},
				Edits: []editormodel.TextEdit{{Range: editormodel.TextRange{Start: 0, End: 0}, Text: string([]byte{0xff})}},
			},
		},
		{
			name: "result selection splits inserted rune",
			tx: editormodel.Transaction{
				Before: editormodel.Selection{}, After: editormodel.Selection{Anchor: 1, Head: 1},
				Edits: []editormodel.TextEdit{{Range: editormodel.TextRange{Start: 0, End: 0}, Text: "🙂"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := (editorTableHost{editor: e}).Apply(tt.tx); err == nil {
				t.Fatal("expected transaction to be rejected")
			}
			if got := e.Source(); got != before {
				t.Fatalf("rejected transaction changed source: %q", got)
			}
		})
	}
}
