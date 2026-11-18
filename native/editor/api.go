// Package editor re-exports the standalone source-editor engine for Muse's
// model and feature packages. Rendering and MyGo adaptation live in nativeui.
package editor

import (
	"sort"

	mdedit "github.com/wheregrassdreams/muse-editor/markdown/edit"
	"github.com/wheregrassdreams/muse-editor/text/session"
)

// Session is the authoritative editing buffer and history for one document.
// Its implementation lives in the independent muse-editor module.
type Session = session.Session
type EditBatch = session.EditBatch
type Selection = session.Selection
type Transaction = session.Transaction
type TextEdit = session.TextEdit
type TextRange = session.TextRange
type DispatchResult = session.DispatchResult
type ChangeKind = session.ChangeKind
type Change = session.Change
type PresentationAdapter = session.PresentationAdapter
type HistoryGroup = session.HistoryGroup
type TransactionOptions = session.TransactionOptions
type HistorySnapshot = session.HistorySnapshot
type HistoryParticipant = session.HistoryParticipant
type Frame = session.Frame
type SystemPatch = session.SystemPatch

const (
	ChangeSelection     = session.ChangeSelection
	ChangeEdit          = session.ChangeEdit
	ChangeUndo          = session.ChangeUndo
	ChangeRedo          = session.ChangeRedo
	ChangeReplaceSource = session.ChangeReplaceSource

	HistoryGroupNone     = session.HistoryGroupNone
	HistoryGroupTyping   = session.HistoryGroupTyping
	HistoryGroupDelete   = session.HistoryGroupDelete
	HistoryGroupExplicit = session.HistoryGroupExplicit
)

// NewSession creates a backend-neutral editing session.
func NewSession(source string) *Session { return session.NewSession(source) }

// TransactionFromPlan converts a backend-neutral Markdown source plan into
// Muse's atomic editor transaction. The plan selection wins when present;
// otherwise the prior selection is mapped through the edits.
func TransactionFromPlan(plan mdedit.Plan, before Selection) Transaction {
	tx := Transaction{Before: before}
	tx.Edits = make([]TextEdit, 0, len(plan.Edits))
	for _, edit := range plan.Edits {
		tx.Edits = append(tx.Edits, TextEdit{
			Range: TextRange{Start: int(edit.Range.From), End: int(edit.Range.To)},
			Text:  edit.Insert,
		})
	}
	sort.SliceStable(tx.Edits, func(i, j int) bool {
		if tx.Edits[i].Range.Start == tx.Edits[j].Range.Start {
			return tx.Edits[i].Range.End < tx.Edits[j].Range.End
		}
		return tx.Edits[i].Range.Start < tx.Edits[j].Range.Start
	})
	tx.After = tx.MapSelection(before)
	if plan.Selection != nil {
		tx.After = Selection{Anchor: int(plan.Selection.Anchor), Head: int(plan.Selection.Head)}
	}
	return tx
}
