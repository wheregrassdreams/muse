package core

import (
	"fmt"

	"github.com/wheregrassdreams/muse-editor/text"
)

// SystemPatch is one minimal source replacement and its selection transition.
// It can be planned before a UI adapter adjusts feature-owned ranges, then
// committed atomically to the text engine.
type SystemPatch struct {
	BeforeSource string
	Edit         TextEdit
	Before       Selection
	After        Selection
	Policy       text.SystemPatchPolicy
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
	insert := target[from:newEnd]
	delta := len(insert) - (oldEnd - from)
	mapOffset := func(offset int) int {
		switch {
		case offset >= oldEnd:
			return offset + delta
		case offset > from:
			return from + len(insert)
		default:
			return offset
		}
	}
	after := selection
	after.Anchor = mapOffset(selection.Anchor)
	after.Head = mapOffset(selection.Head)
	return SystemPatch{
		BeforeSource: source,
		Edit:         TextEdit{Range: TextRange{Start: from, End: oldEnd}, Text: insert},
		Before:       selection,
		After:        after,
		Policy:       text.SystemPatchFoldIntoPrevious,
	}, true
}

// PlanSystemPatch builds a patch against the Session's current source and
// selection. ApplySystemPatch rejects it if either has changed in the meantime.
func (s *Session) PlanSystemPatch(target string) (SystemPatch, bool) {
	if s == nil {
		return SystemPatch{}, false
	}
	return PlanSystemPatch(s.Source(), target, s.Selection())
}

// ApplySystemPatch delegates source correction and history policy to the text
// engine. The adapter only validates the plan against the current revision and
// publishes the resulting delta to the parser/projection layer.
func (s *Session) ApplySystemPatch(patch SystemPatch, beforeMetadata, afterMetadata HistorySnapshot) (DispatchResult, error) {
	if s == nil {
		return DispatchResult{}, fmt.Errorf("nil editor session")
	}
	s.editMu.Lock()
	state := s.engine.Snapshot()
	before := s.Selection()
	if state.Text() != patch.BeforeSource || before != patch.Before {
		s.editMu.Unlock()
		return DispatchResult{}, fmt.Errorf("system patch is stale")
	}
	if err := validateTransactionSource(patch.BeforeSource, Transaction{
		Before: patch.Before,
		After:  patch.After,
		Edits:  []TextEdit{patch.Edit},
	}); err != nil {
		s.editMu.Unlock()
		return DispatchResult{}, err
	}
	metadataBefore := historyMetadataWithSelection(s.historyMetadata(beforeMetadata), patch.Before)
	metadataAfter := historyMetadataWithSelection(s.historyMetadata(afterMetadata), patch.After)
	selection := text.NewSelection(text.Offset(patch.After.Anchor), text.Offset(patch.After.Head))
	policy := patch.Policy
	if policy == 0 {
		policy = text.SystemPatchFoldIntoPrevious
	}
	result, err := s.engine.ApplySystemPatch(text.SystemPatch{
		Edits: []text.Edit{{
			Range:  text.Range{From: text.Offset(patch.Edit.Range.Start), To: text.Offset(patch.Edit.Range.End)},
			Insert: patch.Edit.Text,
		}},
		Selection:     &selection,
		Policy:        policy,
		HistoryBefore: metadataBefore,
		HistoryAfter:  metadataAfter,
	})
	if err != nil {
		s.editMu.Unlock()
		return DispatchResult{}, err
	}
	s.mu.Lock()
	s.selectionInclusive, s.selectionLinewise = patch.After.Inclusive, patch.After.Linewise
	s.cacheValid = false
	s.mu.Unlock()
	after := s.Selection()
	s.editMu.Unlock()
	change := Change{
		Kind:           ChangeEdit,
		BeforeRevision: state.Revision(),
		Revision:       result.State.Revision(),
		Before:         before,
		After:          after,
		Edits:          []TextEdit{patch.Edit},
	}
	s.notifyChange(change)
	return DispatchResult{
		Handled:          true,
		Changed:          true,
		SelectionChanged: before != after,
		Revision:         result.State.Revision(),
	}, nil
}
