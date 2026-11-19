package media

import "testing"

func TestVirtualImageRowsEnterMoveAndLeave(t *testing.T) {
	var rows VirtualImageRows
	if rows.Enter(42, 1, 1) {
		t.Fatal("one visual row should not create a virtual caret")
	}
	if !rows.Enter(42, 4, -1) {
		t.Fatal("movement entering from below should activate the image")
	}
	state, ok := rows.State()
	if !ok || state != (VirtualImageRow{LineStart: 42, Row: 3}) {
		t.Fatalf("entered state = %+v, active=%v", state, ok)
	}
	if !rows.Move(-1, 4) {
		t.Fatal("movement within the image should be consumed")
	}
	state, _ = rows.State()
	if state.Row != 2 {
		t.Fatalf("moved state = %+v, want row 2", state)
	}
	for i := 0; i < 3; i++ {
		rows.Move(-1, 4)
	}
	if rows.Move(-1, 4) {
		t.Fatal("movement past the image edge should fall through")
	}
	if _, ok := rows.State(); ok {
		t.Fatal("crossing the edge should clear the virtual caret")
	}
}

func TestVirtualImageRowsCaretYClampsState(t *testing.T) {
	rows := VirtualImageRows{state: &VirtualImageRow{LineStart: 9, Row: 10}}
	row, offset, height, ok := rows.CaretY(3, 18)
	if !ok || row != 2 || offset != 36 || height != 18 {
		t.Fatalf("caret projection = (%d, %v, %v, %v)", row, offset, height, ok)
	}
}
