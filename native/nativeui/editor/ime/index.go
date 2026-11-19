package ime

import (
	"sort"
	"strings"
	"unicode/utf8"
)

const indexStride = 2048

// Index maps UTF-8 byte offsets to UTF-16 code-unit offsets. Its sparse index
// is incrementally moved across edits so text-input queries do not rescan the
// whole document on every key event.
type Index struct {
	revision uint64
	bytes    []int
	units    []int
	midB     []int
	midU     []int
	nextB    []int
	nextU    []int
}

func (ix *Index) Ensure(source string, revision uint64) {
	if ix == nil || ix.revision == revision && ix.bytes != nil {
		return
	}
	ix.Rebuild(source, revision)
}

func (ix *Index) Rebuild(source string, revision uint64) {
	if ix == nil {
		return
	}
	ix.revision = revision
	ix.bytes = ix.bytes[:0]
	ix.units = ix.units[:0]
	units := 0
	next := 0
	for i, r := range source {
		if i >= next {
			ix.bytes = append(ix.bytes, i)
			ix.units = append(ix.units, units)
			next = i + indexStride
		}
		units += utf16RuneLen(r)
	}
	ix.bytes = append(ix.bytes, len(source))
	ix.units = append(ix.units, units)
}

// NoteEdit moves the cached index after replacing [from,to) in the old source
// with new. src is the new source and revision is its revision number.
func (ix *Index) NoteEdit(from, to int, old, replacement, src string, revision uint64) {
	if ix == nil || ix.bytes == nil || ix.revision+1 != revision || from < 0 || to < from || to > len(src) {
		if ix != nil {
			ix.bytes = nil
		}
		return
	}
	ix.revision = revision
	deltaB, deltaU := len(replacement)-(to-from), UTF16Len(replacement)-UTF16Len(old)
	lo := sort.SearchInts(ix.bytes, from)
	hi := sort.SearchInts(ix.bytes, to)
	end := to + deltaB
	last, units := 0, 0
	if lo > 0 {
		last = ix.bytes[lo-1]
		units = ix.units[lo-1] + UTF16Len(src[ix.bytes[lo-1]:from])
	}
	ix.midB, ix.midU = ix.midB[:0], ix.midU[:0]
	for at := from; at < end; {
		if last < 0 || at-last >= indexStride {
			ix.midB = append(ix.midB, at)
			ix.midU = append(ix.midU, units)
			last = at
		}
		n := min(at+indexStride, end)
		units += UTF16Len(src[at:n])
		at = n
	}
	if lo == 0 {
		ix.nextB = append(ix.nextB[:0], 0)
		ix.nextU = append(ix.nextU[:0], 0)
	} else {
		ix.nextB = append(ix.nextB[:0], ix.bytes[:lo]...)
		ix.nextU = append(ix.nextU[:0], ix.units[:lo]...)
	}
	ix.nextB = append(ix.nextB, ix.midB...)
	ix.nextU = append(ix.nextU, ix.midU...)
	for k := hi; k < len(ix.bytes); k++ {
		byteOffset := ix.bytes[k] + deltaB
		unitOffset := ix.units[k] + deltaU
		last := len(ix.nextB) - 1
		if last >= 0 && ix.nextB[last] == byteOffset {
			// Deleting through a sparse checkpoint can collapse it onto the
			// preceding boundary. Keep one canonical checkpoint at that byte.
			ix.nextU[last] = unitOffset
			continue
		}
		ix.nextB = append(ix.nextB, byteOffset)
		ix.nextU = append(ix.nextU, unitOffset)
	}
	ix.bytes, ix.nextB = ix.nextB, ix.bytes[:0]
	ix.units, ix.nextU = ix.nextU, ix.units[:0]
}

func (ix *Index) UnitsOf(source string, revision uint64, byteOffset int) int {
	if ix == nil {
		return UTF16Len(source[:clampByte(source, byteOffset)])
	}
	ix.Ensure(source, revision)
	byteOffset = max(0, min(byteOffset, len(source)))
	k := 0
	for k+1 < len(ix.bytes) && ix.bytes[k+1] <= byteOffset {
		k++
	}
	return ix.units[k] + UTF16Len(source[ix.bytes[k]:byteOffset])
}

func clampByte(source string, offset int) int {
	offset = max(0, min(offset, len(source)))
	for offset > 0 && offset < len(source) && !utf8.RuneStart(source[offset]) {
		offset--
	}
	return offset
}

// ByteOf maps a UTF-16 coordinate to a source byte boundary. Offsets inside a
// surrogate pair advance to the end of that rune, matching the prior client.
func (ix *Index) ByteOf(source string, revision uint64, unitOffset int) int {
	if unitOffset <= 0 {
		return 0
	}
	if ix == nil {
		return utf16ByteOffset(source, unitOffset)
	}
	ix.Ensure(source, revision)
	k := 0
	for k+1 < len(ix.units) && ix.units[k+1] <= unitOffset {
		k++
	}
	return ix.bytes[k] + utf16ByteOffset(source[ix.bytes[k]:], unitOffset-ix.units[k])
}

func (ix *Index) TextForRange(snapshot Snapshot, composition Composition, r Range) (string, Range) {
	r = normalizeRange(r)
	caretUnits := ix.UnitsOf(snapshot.Source, snapshot.Revision, snapshot.Caret)
	compositionUnits := UTF16Len(composition.Text)
	full := ix.UnitsOf(snapshot.Source, snapshot.Revision, len(snapshot.Source)) + compositionUnits
	r.Start = max(0, min(r.Start, full))
	r.End = max(0, min(r.End, full))
	var out strings.Builder
	if r.Start < caretUnits {
		start := ix.ByteOf(snapshot.Source, snapshot.Revision, r.Start)
		end := ix.ByteOf(snapshot.Source, snapshot.Revision, min(r.End, caretUnits))
		out.WriteString(snapshot.Source[start:end])
	}
	if compositionUnits > 0 && r.Start < caretUnits+compositionUnits && r.End > caretUnits {
		start := utf16ByteOffset(composition.Text, max(r.Start-caretUnits, 0))
		end := utf16ByteOffset(composition.Text, min(r.End-caretUnits, compositionUnits))
		out.WriteString(composition.Text[start:end])
	}
	if r.End > caretUnits+compositionUnits {
		start := ix.ByteOf(snapshot.Source, snapshot.Revision, max(r.Start, caretUnits+compositionUnits)-compositionUnits)
		end := ix.ByteOf(snapshot.Source, snapshot.Revision, r.End-compositionUnits)
		out.WriteString(snapshot.Source[start:end])
	}
	text := out.String()
	return text, Range{Start: r.Start, End: r.Start + UTF16Len(text)}
}

// Snapshot returns copies of the index arrays for diagnostics and tests.
func (ix *Index) Snapshot() (revision uint64, bytes, units []int) {
	if ix == nil {
		return 0, nil, nil
	}
	return ix.revision, append([]int(nil), ix.bytes...), append([]int(nil), ix.units...)
}

func (ix *Index) Empty() bool {
	return ix == nil || ix.revision == 0 && ix.bytes == nil && ix.units == nil && ix.midB == nil && ix.midU == nil && ix.nextB == nil && ix.nextU == nil
}

func (ix *Index) Reset() {
	if ix == nil {
		return
	}
	*ix = Index{}
}

func UTF16Len(value string) int {
	units := 0
	for _, r := range value {
		units += utf16RuneLen(r)
	}
	return units
}

func utf16RuneLen(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

func utf16ByteOffset(source string, units int) int {
	if units <= 0 {
		return 0
	}
	position := 0
	for offset, r := range source {
		if position >= units {
			return offset
		}
		position += utf16RuneLen(r)
	}
	return len(source)
}
