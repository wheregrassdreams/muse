package session

import (
	"testing"

	editormodel "github.com/wheregrassdreams/muse-editor/text/session"
)

func TestApplyTransactionUsesSessionHistoryAtomically(t *testing.T) {
	target := editormodel.NewSession("abcdef")
	before := target.Selection()
	err := ApplyTransaction(target, editormodel.Transaction{
		Edits: []editormodel.TextEdit{
			{Range: editormodel.TextRange{Start: 1, End: 2}, Text: "XY"},
			{Range: editormodel.TextRange{Start: 4, End: 6}, Text: "!"},
		},
		Before: before,
		After:  editormodel.Selection{Anchor: 4, Head: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := target.Source(), "aXYcd!"; got != want {
		t.Fatalf("source after transaction = %q, want %q", got, want)
	}
	if _, err := target.Undo(); err != nil {
		t.Fatalf("undo transaction: %v", err)
	}
	if got, want := target.Source(), "abcdef"; got != want {
		t.Fatalf("source after undo = %q, want %q", got, want)
	}
}

func TestApplyTransactionRejectsStaleOrInvalidValues(t *testing.T) {
	target := editormodel.NewSession("🙂x")
	before := target.Selection()
	stale := editormodel.Transaction{
		Edits:  []editormodel.TextEdit{{Range: editormodel.TextRange{Start: 0, End: 1}, Text: "x"}},
		Before: editormodel.Selection{Anchor: 1, Head: 1}, After: before,
	}
	if err := ApplyTransaction(target, stale); err == nil {
		t.Fatal("accepted stale transaction selection")
	}
	invalid := editormodel.Transaction{
		Edits:  []editormodel.TextEdit{{Range: editormodel.TextRange{Start: 1, End: 2}, Text: "x"}},
		Before: before, After: before,
	}
	if err := ApplyTransaction(target, invalid); err == nil {
		t.Fatal("accepted a source range that splits a UTF-8 rune")
	}
	if got, want := target.Source(), "🙂x"; got != want {
		t.Fatalf("rejected transaction changed source: got %q want %q", got, want)
	}
}
