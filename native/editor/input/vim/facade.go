// Package vim preserves Muse's historical import path for the backend-neutral
// Helix keymap controller. The implementation lives in muse-editor/text/helix.
package vim

import model "github.com/wheregrassdreams/muse-editor/text/helix"

type Mode = model.Mode
type RegisterKind = model.RegisterKind
type ViewState = model.ViewState
type Controller = model.Controller
type CommandKind = model.CommandKind
type Command = model.Command
type IntentKind = model.IntentKind
type Intent = model.Intent
type MotionKind = model.MotionKind
type Sequence = model.Sequence

const (
	ModeNormal      = model.ModeNormal
	ModeInsert      = model.ModeInsert
	ModeVisual      = model.ModeVisual
	ModeSelect      = model.ModeSelect
	ModeBlockVisual = model.ModeBlockVisual

	RegisterNone  = model.RegisterNone
	RegisterText  = model.RegisterText
	RegisterBlock = model.RegisterBlock
	RegisterTable = model.RegisterTable

	CommandNone          = model.CommandNone
	CommandCountPrefix   = model.CommandCountPrefix
	CommandPendingPrefix = model.CommandPendingPrefix
	CommandFirstLine     = model.CommandFirstLine
	CommandVertical      = model.CommandVertical

	IntentConsumed            = model.IntentConsumed
	IntentPrefix              = model.IntentPrefix
	IntentCommand             = model.IntentCommand
	IntentSearchText          = model.IntentSearchText
	IntentMotion              = model.IntentMotion
	IntentOperator            = model.IntentOperator
	IntentEnterInsert         = model.IntentEnterInsert
	IntentEnterVisual         = model.IntentEnterVisual
	IntentEnterLineVisual     = model.IntentEnterLineVisual
	IntentFindCharacter       = model.IntentFindCharacter
	IntentReplaceCharacter    = model.IntentReplaceCharacter
	IntentPutRegister         = model.IntentPutRegister
	IntentUndo                = model.IntentUndo
	IntentDeleteCharacter     = model.IntentDeleteCharacter
	IntentDeleteLineRemainder = model.IntentDeleteLineRemainder
	IntentOpenLine            = model.IntentOpenLine
	IntentBlockInsert         = model.IntentBlockInsert

	MotionLeft          = model.MotionLeft
	MotionRight         = model.MotionRight
	MotionDown          = model.MotionDown
	MotionUp            = model.MotionUp
	MotionWordForward   = model.MotionWordForward
	MotionWordBackward  = model.MotionWordBackward
	MotionWordEnd       = model.MotionWordEnd
	MotionLineStart     = model.MotionLineStart
	MotionFirstNonBlank = model.MotionFirstNonBlank
	MotionLineEnd       = model.MotionLineEnd
	MotionDocumentEnd   = model.MotionDocumentEnd
)

var New = model.New
