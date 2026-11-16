// Package paragraph owns paragraph-specific source anchors and editing
// policies that survive ordinary editor layout and source navigation.
package paragraph

import (
	"encoding/binary"
	"sort"
	"strings"
)

// BlankInput describes the editor state relevant to preserving paragraph
// boundaries while text is entered into an empty source line.
type BlankInput struct {
	Enabled            bool
	CollapseSoftBreaks bool
	CollapsedSelection bool
	PhantomParagraph   bool
	BlankSourceLine    bool
	Source             string
	Caret              int
	Text               string
}

// BlankInputPlan describes paragraph metadata to capture before a source edit.
// It does not duplicate the editor's text transaction: Session owns the
// inserted text and resulting selection, while this plan preserves the
// existing source newlines in the view-local Breaks set.
type BlankInputPlan struct {
	BeginHistory   bool
	RememberBreaks []int
}

// PlanBlankInput computes the typed-break metadata transition for an input
// intent. The host still commits input.Text through the editor Session.
func PlanBlankInput(in BlankInput) BlankInputPlan {
	if !ShouldTrackBlankInput(in) {
		return BlankInputPlan{}
	}
	plan := BlankInputPlan{BeginHistory: true}
	if in.PhantomParagraph || in.Caret < 0 || in.Caret > len(in.Source) {
		return plan
	}
	lineStart := strings.LastIndexByte(in.Source[:in.Caret], '\n') + 1
	lineEnd := len(in.Source)
	if next := strings.IndexByte(in.Source[in.Caret:], '\n'); next >= 0 {
		lineEnd = in.Caret + next
	}
	if lineStart > 0 && in.Source[lineStart-1] == '\n' {
		plan.RememberBreaks = append(plan.RememberBreaks, lineStart-1)
	}
	if lineEnd < len(in.Source) && in.Source[lineEnd] == '\n' {
		plan.RememberBreaks = append(plan.RememberBreaks, lineEnd)
	}
	return plan
}

// ShouldTrackBlankInput reports whether typing should pin the surrounding
// source newlines as visible paragraph boundaries for this editing session.
func ShouldTrackBlankInput(in BlankInput) bool {
	return ShouldProcessInput(in) &&
		(in.PhantomParagraph || in.BlankSourceLine) && strings.TrimSpace(in.Text) != ""
}

// ShouldProcessInput reports whether an edit is eligible for paragraph
// boundary preservation before the editor resolves the source line kind.
func ShouldProcessInput(in BlankInput) bool {
	return in.Enabled && in.CollapseSoftBreaks && in.CollapsedSelection && strings.TrimSpace(in.Text) != ""
}

// BreakProjection contains the view policy for a parsed Markdown break.
type BreakProjection struct {
	SoftBreak          bool
	CollapseSoftBreaks bool
	PreserveLine       bool
	EditingLine        bool
	StructuralBreak    bool
	RevealSource       bool
}

// Newline reports whether a parsed source break must remain a visual newline.
func (p BreakProjection) Newline() bool {
	return !(p.SoftBreak && p.CollapseSoftBreaks && !p.PreserveLine &&
		!p.EditingLine && !p.StructuralBreak && !p.RevealSource)
}

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
