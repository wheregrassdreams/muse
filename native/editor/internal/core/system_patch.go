package core

import (
	"fmt"
	"strings"
	"time"
)

// SystemPatch is one minimal source replacement and its selection transition.
// It can be planned before a UI adapter adjusts feature-owned ranges, then
// committed atomically to the Session.
type SystemPatch struct {
	BeforeSource string
	Edit         TextEdit
	Before       Selection
	After        Selection
}

// PlanSystemPatch computes a rune-aligned replacement from source to target.
// The selection follows the same offset mapping as an ordinary source edit.
func PlanSystemPatch(source, target string, selection Selection) (SystemPatch, bool) {
	if source == target || validateSelection(source, selection) != nil {
		return SystemPatch{}, false
	}
	from := 0
	for from < len(source) && from < len(target) && source[from] == target[from] {
		from++
	}
	for from > 0 && (!runeBoundary(source, from) || !runeBoundary(target, from)) {
		from--
	}
	oldEnd, newEnd := len(source), len(target)
	for oldEnd > from && newEnd > from && source[oldEnd-1] == target[newEnd-1] {
		oldEnd--
		newEnd--
	}
	for oldEnd < len(source) && !runeBoundary(source, oldEnd) {
		oldEnd++
	}
	for newEnd < len(target) && !runeBoundary(target, newEnd) {
		newEnd++
	}
	text := target[from:newEnd]
	delta := len(text) - (oldEnd - from)
	mapOffset := func(offset int) int {
		switch {
		case offset >= oldEnd:
			return offset + delta
		case offset > from:
			return from + len(text)
		default:
			return offset
		}
	}
	after := selection
	after.Anchor = mapOffset(selection.Anchor)
	after.Head = mapOffset(selection.Head)
	return SystemPatch{
		BeforeSource: source,
		Edit:         TextEdit{Range: TextRange{Start: from, End: oldEnd}, Text: text},
		Before:       selection,
		After:        after,
	}, true
}

// PlanSystemPatch builds a patch against the Session's current source and
// selection. ApplySystemPatch rejects it if either has changed in the meantime.
func (s *Session) PlanSystemPatch(target string) (SystemPatch, bool) {
	if s == nil {
		return SystemPatch{}, false
	}
	s.mu.RLock()
	patch, ok := PlanSystemPatch(s.src, target, s.selectionLocked())
	s.mu.RUnlock()
	return patch, ok
}

// ApplySystemPatch commits a system-owned source change without adding a
// separate user-visible undo item. The patch is folded into the previous user
// history step; if no such step exists, the system change remains the baseline.
func (s *Session) ApplySystemPatch(patch SystemPatch, beforeMetadata, afterMetadata HistorySnapshot) (DispatchResult, error) {
	if s == nil {
		return DispatchResult{}, fmt.Errorf("nil editor session")
	}
	s.editMu.Lock()
	s.mu.Lock()
	if s.src != patch.BeforeSource || s.selectionLocked() != patch.Before {
		s.mu.Unlock()
		s.editMu.Unlock()
		return DispatchResult{}, fmt.Errorf("system patch is stale")
	}
	tx := Transaction{Edits: []TextEdit{patch.Edit}, Before: patch.Before, After: patch.After}
	if err := validateTransactionSource(s.src, tx); err != nil {
		s.mu.Unlock()
		s.editMu.Unlock()
		return DispatchResult{}, err
	}
	if patch.Edit.Range.Start == patch.Edit.Range.End && patch.Edit.Text == "" {
		revision := s.rev
		s.mu.Unlock()
		s.editMu.Unlock()
		return DispatchResult{Handled: true, Revision: revision}, nil
	}
	beforeSource := s.src
	old := s.src[patch.Edit.Range.Start:patch.Edit.Range.End]
	s.src = strings.Clone(s.src[:patch.Edit.Range.Start] + patch.Edit.Text + s.src[patch.Edit.Range.End:])
	s.setSelection(patch.After)
	s.rev++
	s.redo = nil
	step := sessionStep{
		edits:          []historyEdit{{at: patch.Edit.Range.Start, old: old, new: patch.Edit.Text}},
		before:         patch.Before,
		after:          patch.After,
		group:          HistoryGroupExplicit,
		at:             time.Now(),
		closed:         true,
		beforeMetadata: cloneHistorySnapshot(beforeMetadata),
		afterMetadata:  cloneHistorySnapshot(afterMetadata),
	}
	if n := len(s.undo); n > 0 {
		mergeHistoryStep(&s.undo[n-1], step)
		s.undo[n-1].group = HistoryGroupExplicit
		s.undo[n-1].closed = true
	}
	change := Change{
		Kind:         ChangeEdit,
		BeforeSource: beforeSource,
		AfterSource:  s.src,
		Before:       patch.Before,
		After:        patch.After,
		Revision:     s.rev,
		Edits:        []TextEdit{patch.Edit},
	}
	revision := s.rev
	s.mu.Unlock()
	s.editMu.Unlock()
	s.notifyChange(change)
	return DispatchResult{Handled: true, Changed: true, SelectionChanged: patch.Before != patch.After, Revision: revision}, nil
}
