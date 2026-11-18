// Package core owns the backend-neutral document buffer, selection, transaction,
// history, and system-patch primitives used by an editor session.
package core

import "fmt"

// TextRange is a half-open byte range in a UTF-8 Markdown source buffer.
type TextRange struct {
	Start int
	End   int
}

// Selection is the durable text-selection component of an editing session.
// Block-specific selections are kept by their block controllers and are
// composed with this value at the editor boundary.
type Selection struct {
	Anchor    int
	Head      int
	Inclusive bool
	Linewise  bool
}

// TextEdit replaces Range with Text. Range offsets are measured against the
// transaction's original source snapshot, not against earlier edits.
type TextEdit struct {
	Range TextRange
	Text  string
}

// Transaction is an atomic set of source edits and the text selection to
// restore before and after them. The editor facade is responsible for applying
// edits in a history step and restoring these selections during undo/redo.
type Transaction struct {
	Edits  []TextEdit
	Before Selection
	After  Selection
}

// MapSelection applies this transaction's offset changes to a source-relative
// selection while preserving its Vim selection metadata.
func (tx Transaction) MapSelection(selection Selection) Selection {
	mapOffset := func(offset int) int {
		original, deltaTotal := offset, 0
		for _, edit := range tx.Edits {
			delta := len(edit.Text) - (edit.Range.End - edit.Range.Start)
			switch {
			case original >= edit.Range.End:
				deltaTotal += delta
			case original > edit.Range.Start:
				return edit.Range.Start + deltaTotal + len(edit.Text)
			default:
				return original + deltaTotal
			}
		}
		return original + deltaTotal
	}
	selection.Anchor = mapOffset(selection.Anchor)
	selection.Head = mapOffset(selection.Head)
	return selection
}

// Validate checks that edits address the source snapshot and do not overlap.
// Adjacent ranges are allowed. Edits must be ordered by ascending source byte
// offset so callers can reason about their transaction deterministically.
func (t Transaction) Validate(sourceLen int) error {
	lastEnd := 0
	resultLen := sourceLen
	for i, edit := range t.Edits {
		if edit.Range.Start < 0 || edit.Range.End < edit.Range.Start || edit.Range.End > sourceLen {
			return fmt.Errorf("edit %d has invalid source range [%d,%d) for length %d", i, edit.Range.Start, edit.Range.End, sourceLen)
		}
		if i > 0 && edit.Range.Start < lastEnd {
			return fmt.Errorf("edit %d overlaps or precedes the previous edit", i)
		}
		lastEnd = edit.Range.End
		resultLen += len(edit.Text) - (edit.Range.End - edit.Range.Start)
	}
	if t.Before.Anchor < 0 || t.Before.Anchor > sourceLen || t.Before.Head < 0 || t.Before.Head > sourceLen {
		return fmt.Errorf("before selection is outside source length %d", sourceLen)
	}
	if t.After.Anchor < 0 || t.After.Anchor > resultLen || t.After.Head < 0 || t.After.Head > resultLen {
		return fmt.Errorf("after selection is outside result length %d", resultLen)
	}
	return nil
}
