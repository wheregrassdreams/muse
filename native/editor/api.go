// Package editor re-exports the standalone source-editor engine for Muse's
// model and feature packages. Rendering and MyGo adaptation live in nativeui.
package editor

import (
	mdedit "github.com/wheregrassdreams/muse-editor/markdown/edit"
	mdtable "github.com/wheregrassdreams/muse-editor/markdown/table"
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
	return mdtable.TransactionFromPlan(plan, before)
}
