package ime

import (
	"math/rand/v2"
	"testing"
)

func TestIndexFollowsEdits(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	source := "一段文字，with ascii, 还有 emoji 😀 和另一个字 😀😀 end。\n"
	var index Index
	revision := uint64(1)
	index.Rebuild(source, revision)
	texts := []string{"x", "中文", "😀", "\n", "ab", "", "é", "🎉🎉", "a longer piece of text inserted"}
	for step := 0; step < 300; step++ {
		from := runeStart(source, rng.IntN(len(source)+1))
		to := runeStart(source, min(from+rng.IntN(7), len(source)))
		if rng.IntN(2) == 0 {
			to = from
		}
		replacement := texts[rng.IntN(len(texts))]
		old := source[from:to]
		next := source[:from] + replacement + source[to:]
		revision++
		index.NoteEdit(from, to, old, replacement, next, revision)
		source = next
		index.Ensure(source, revision)

		gotRevision, bytes, units := index.Snapshot()
		if gotRevision != revision || len(bytes) == 0 || bytes[0] != 0 || bytes[len(bytes)-1] != len(source) {
			t.Fatalf("step %d: index revision/range = %d/%v, want %d and end %d", step, gotRevision, bytes, revision, len(source))
		}
		for k := 1; k < len(bytes); k++ {
			if bytes[k] <= bytes[k-1] || units[k] < units[k-1] {
				t.Fatalf("step %d: index entries %d and %d are not ordered: bytes=%v units=%v", step, k-1, k, bytes, units)
			}
		}
		for _, byteOffset := range runeStarts(source) {
			if got, want := index.UnitsOf(source, revision, byteOffset), UTF16Len(source[:byteOffset]); got != want {
				t.Fatalf("step %d: UTF-16 offset of byte %d is %d, want %d", step, byteOffset, got, want)
			}
		}
		unitsAt := 0
		for byteOffset, r := range source {
			if got := index.ByteOf(source, revision, unitsAt); got != byteOffset {
				t.Fatalf("step %d: byte of UTF-16 offset %d is %d, want %d", step, unitsAt, got, byteOffset)
			}
			unitsAt += utf16RuneLen(r)
		}
		if got := index.ByteOf(source, revision, unitsAt); got != len(source) {
			t.Fatalf("step %d: byte of final UTF-16 offset %d is %d, want %d", step, unitsAt, got, len(source))
		}
	}
}

func TestStateMapsMarkedTextCoordinates(t *testing.T) {
	source := "一a😀z"
	var state State
	state.SetComposition("中😀", Range{Start: 1, End: 3})
	snapshot := Snapshot{Source: source, Revision: 1, Caret: len("一a"), Selection: Selection{Range: Range{Start: len("一a"), End: len("一a")}}}

	text, span := state.TextForRange(snapshot, Range{Start: 0, End: 8})
	if text != "一a中😀😀z" || span != (Range{Start: 0, End: 8}) {
		t.Fatalf("TextForRange = %q / %+v", text, span)
	}
	selected, ok := state.Selection(snapshot)
	if !ok || selected.Range != (Range{Start: 3, End: 5}) {
		t.Fatalf("composition selection = %+v, %v", selected, ok)
	}
	marked, ok := state.MarkedRange(snapshot)
	if !ok || marked != (Range{Start: 2, End: 5}) {
		t.Fatalf("marked range = %+v, %v", marked, ok)
	}
	if got, inside := state.VirtualByte(snapshot, 3); got != snapshot.Caret || inside != 1 {
		t.Fatalf("virtual composition byte = %d + %d, want caret %d + 1", got, inside, snapshot.Caret)
	}
}

func runeStart(source string, offset int) int {
	offset = max(0, min(offset, len(source)))
	for offset > 0 && offset < len(source) && source[offset]&0xC0 == 0x80 {
		offset--
	}
	return offset
}

func runeStarts(source string) []int {
	starts := make([]int, 0, len(source)+1)
	for offset := range source {
		starts = append(starts, offset)
	}
	return append(starts, len(source))
}
