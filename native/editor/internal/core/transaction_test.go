package core

import "testing"

func TestTransactionValidatesOriginalSourceRanges(t *testing.T) {
	tx := Transaction{
		Edits: []TextEdit{
			{Range: TextRange{Start: 1, End: 2}, Text: "x"},
			{Range: TextRange{Start: 4, End: 4}, Text: "y"},
		},
		Before: Selection{Anchor: 1, Head: 2},
		After:  Selection{Anchor: 5, Head: 5},
	}
	if err := tx.Validate(6); err != nil {
		t.Fatalf("valid transaction rejected: %v", err)
	}
}

func TestTransactionRejectsOverlappingOrUnorderedEdits(t *testing.T) {
	tests := []struct {
		name  string
		edits []TextEdit
	}{
		{name: "overlap", edits: []TextEdit{{Range: TextRange{Start: 1, End: 4}}, {Range: TextRange{Start: 3, End: 5}}}},
		{name: "unordered", edits: []TextEdit{{Range: TextRange{Start: 4, End: 5}}, {Range: TextRange{Start: 1, End: 2}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := (Transaction{Edits: tt.edits}).Validate(8); err == nil {
				t.Fatal("invalid edit ordering was accepted")
			}
		})
	}
}

func TestTransactionRejectsSelectionOutsideSource(t *testing.T) {
	tx := Transaction{Before: Selection{Anchor: 4, Head: 4}, After: Selection{Anchor: 9, Head: 9}}
	if err := tx.Validate(8); err == nil {
		t.Fatal("out-of-source before selection was accepted")
	}
	tx.Before = Selection{}
	if err := tx.Validate(8); err == nil {
		t.Fatal("negative after selection was accepted")
	}
}
