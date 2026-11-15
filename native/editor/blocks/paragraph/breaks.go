// Package paragraph owns paragraph-specific source anchors and editing
// policies that survive ordinary editor layout and source navigation.
package paragraph

import (
	"encoding/binary"
	"sort"
)

// Breaks tracks explicit source newlines created or preserved during the
// current editing session. The backing source remains unchanged.
type Breaks struct{ offsets []int }

// Len reports the number of remembered breaks.
func (b *Breaks) Len() int { return len(b.offsets) }

// Clear releases all tracked source anchors.
func (b *Breaks) Clear() { b.offsets = nil }

// Snapshot copies the sorted anchor offsets for undo history.
func (b *Breaks) Snapshot() []int { return append([]int(nil), b.offsets...) }

// Restore replaces the current anchors with a copied, sorted snapshot.
func (b *Breaks) Restore(offsets []int) {
	b.offsets = append(b.offsets[:0], offsets...)
	sort.Ints(b.offsets)
}

// Remember adds at when source contains a newline there.
func (b *Breaks) Remember(source string, at int) {
	if at < 0 || at >= len(source) || source[at] != '\n' {
		return
	}
	b.insert(at)
}

func (b *Breaks) insert(at int) {
	i := sort.SearchInts(b.offsets, at)
	if i < len(b.offsets) && b.offsets[i] == at {
		return
	}
	b.offsets = append(b.offsets, 0)
	copy(b.offsets[i+1:], b.offsets[i:])
	b.offsets[i] = at
}

// ApplyEdit moves surviving anchors through a source replacement. When
// rememberInserted is true, inserted newlines are added to the new snapshot.
func (b *Breaks) ApplyEdit(from, to int, inserted string, rememberInserted bool) {
	out := b.offsets[:0]
	for _, at := range b.offsets {
		switch {
		case at < from:
			out = append(out, at)
		case at >= to:
			out = append(out, at+len(inserted)-(to-from))
		}
	}
	b.offsets = out
	if rememberInserted {
		b.RememberInserted(from, inserted)
	}
}

// RememberInserted adds newline offsets from inserted text without shifting
// existing anchors. This is for callers that already applied anchor movement.
func (b *Breaks) RememberInserted(at int, inserted string) {
	for i := 0; i < len(inserted); i++ {
		if inserted[i] == '\n' {
			b.insert(at + i)
		}
	}
}

// Contains reports whether an anchor falls in [from,to).
func (b *Breaks) Contains(from, to int) bool {
	i := sort.SearchInts(b.offsets, from)
	return i < len(b.offsets) && b.offsets[i] < to
}

// Key encodes anchors in [lineStart,lineEnd) relative to lineStart. The key
// can participate in paragraph layout cache identity without storing source
// positions from before unrelated edits.
func (b *Breaks) Key(lineStart, lineEnd int) string {
	i := sort.SearchInts(b.offsets, lineStart)
	var key []byte
	for ; i < len(b.offsets) && b.offsets[i] < lineEnd; i++ {
		key = binary.AppendUvarint(key, uint64(b.offsets[i]-lineStart))
	}
	return string(key)
}
