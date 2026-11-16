// Package editor exposes the backend-neutral editing session contract.
// Source parsing, keymap input, navigation, and block policies live in focused
// child modules; this package keeps the shared session and transaction API.
package editor

import "muse/editor/internal/core"

// Session is the authoritative editing buffer and history for one document.
// Its implementation is backend-neutral and lives in editor/internal/core.
type Session = core.Session
type EditBatch = core.EditBatch
type Selection = core.Selection
type Transaction = core.Transaction
type TextEdit = core.TextEdit
type TextRange = core.TextRange
type DispatchResult = core.DispatchResult
type ChangeKind = core.ChangeKind
type Change = core.Change
type PresentationAdapter = core.PresentationAdapter
type HistoryGroup = core.HistoryGroup
type TransactionOptions = core.TransactionOptions
type HistorySnapshot = core.HistorySnapshot
type HistoryParticipant = core.HistoryParticipant
type Frame = core.Frame
type SystemPatch = core.SystemPatch

const (
	ChangeSelection     = core.ChangeSelection
	ChangeEdit          = core.ChangeEdit
	ChangeUndo          = core.ChangeUndo
	ChangeRedo          = core.ChangeRedo
	ChangeReplaceSource = core.ChangeReplaceSource

	HistoryGroupNone     = core.HistoryGroupNone
	HistoryGroupTyping   = core.HistoryGroupTyping
	HistoryGroupDelete   = core.HistoryGroupDelete
	HistoryGroupExplicit = core.HistoryGroupExplicit
)

// NewSession creates a backend-neutral editing session.
func NewSession(source string) *Session { return core.NewSession(source) }
