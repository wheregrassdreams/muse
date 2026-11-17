package table

import (
	"testing"

	"muse/editor"
)

func TestSnapshotCloneOwnsSelectionValues(t *testing.T) {
	snapshot := Snapshot{
		Mode:          "Visual",
		TextSelection: editor.Selection{Anchor: 3, Head: 9},
		Selection:     &Selection{LineStart: 2, Anchor: Cell{Row: 0, Col: 1}, Focus: Cell{Row: 2, Col: 3}, Axis: Cells},
		Bridge:        &ContentBridge{LineStart: 2, Axis: Columns, Col: 1, Direction: 1, Resume: "Visual"},
	}
	clone := Clone(snapshot)
	if !Equal(snapshot, clone) {
		t.Fatal("clone changed snapshot value")
	}
	clone.Selection.Focus.Col++
	clone.Bridge.Col++
	if Equal(snapshot, clone) {
		t.Fatal("mutating cloned snapshot changed equality with original")
	}
	if snapshot.Selection.Focus.Col != 3 || snapshot.Bridge.Col != 1 {
		t.Fatal("clone shares mutable pointers with original")
	}
}
