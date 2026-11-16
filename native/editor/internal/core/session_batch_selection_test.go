package core

import "testing"

func TestEditBatchOwnsIntermediateSelectionUntilCommit(t *testing.T) {
	session := NewSession("abc")
	batch, err := session.BeginTransaction(TransactionOptions{Group: HistoryGroupExplicit})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.ApplyEdit(batch, TextEdit{Range: TextRange{Start: 1, End: 2}, Text: "XYZ"}); err != nil {
		t.Fatal(err)
	}
	selection := Selection{Anchor: 3, Head: 3}
	if err := session.UpdateTransactionSelection(batch, selection); err != nil {
		t.Fatal(err)
	}
	if got := session.Selection(); got != selection {
		t.Fatalf("intermediate selection=%+v, want %+v", got, selection)
	}
	if _, err := session.CommitTransaction(batch, selection); err != nil {
		t.Fatal(err)
	}
	if got := session.Source(); got != "aXYZc" {
		t.Fatalf("committed source=%q, want %q", got, "aXYZc")
	}
	if !session.CanUndo() {
		t.Fatal("committed batch was not undoable")
	}
}
