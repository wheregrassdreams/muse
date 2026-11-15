package paragraph

import "testing"

func TestBreaksRememberAndDeduplicateSourceAnchors(t *testing.T) {
	source := "one\ntwo\n"
	var breaks Breaks
	breaks.Remember(source, 3)
	breaks.Remember(source, 3)
	breaks.Remember(source, 0)
	if breaks.Len() != 1 || !breaks.Contains(3, 4) || breaks.Contains(0, 3) {
		t.Fatalf("remembered breaks = %v", breaks.Snapshot())
	}
}

func TestBreaksMoveSurvivingAndInsertedOffsets(t *testing.T) {
	var breaks Breaks
	breaks.Restore([]int{2, 5, 9})
	breaks.ApplyEdit(4, 6, "x\ny", true)
	got := breaks.Snapshot()
	want := []int{2, 5, 10}
	if len(got) != len(want) {
		t.Fatalf("break offsets = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("break offsets = %v, want %v", got, want)
		}
	}
}

func TestBreaksRestoreSortsAndKeyUsesRelativeOffsets(t *testing.T) {
	var breaks Breaks
	breaks.Restore([]int{18, 11, 14})
	if got, want := breaks.Key(10, 19), string([]byte{1, 4, 8}); got != want {
		t.Fatalf("Key() bytes = %v, want %v", []byte(got), []byte(want))
	}
	if got := breaks.Snapshot(); got[0] != 11 || got[1] != 14 || got[2] != 18 {
		t.Fatalf("Restore() order = %v", got)
	}
}
