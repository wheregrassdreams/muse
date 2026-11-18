package input

import text "github.com/wheregrassdreams/muse-editor/text/input"

type OperationKind = text.OperationKind

const (
	OperationBeginCharacterFind  = text.OperationBeginCharacterFind
	OperationFindCharacter       = text.OperationFindCharacter
	OperationRepeatCharacterFind = text.OperationRepeatCharacterFind
	OperationBeginReplace        = text.OperationBeginReplace
	OperationMoveHorizontal      = text.OperationMoveHorizontal
	OperationPutRegister         = text.OperationPutRegister
	OperationReplaceCharacter    = text.OperationReplaceCharacter
	OperationBeginSearch         = text.OperationBeginSearch
	OperationSubmitSearch        = text.OperationSubmitSearch
	OperationCancelSearch        = text.OperationCancelSearch
	OperationRepeatSearch        = text.OperationRepeatSearch
	OperationScrollCaretLine     = text.OperationScrollCaretLine
	OperationFoldAtCaret         = text.OperationFoldAtCaret
	OperationFoldAll             = text.OperationFoldAll
	OperationRevealCaret         = text.OperationRevealCaret
)

type CaretLinePlacement = text.CaretLinePlacement

const (
	CaretLineTop    = text.CaretLineTop
	CaretLineCenter = text.CaretLineCenter
	CaretLineBottom = text.CaretLineBottom
)

type FoldOperation = text.FoldOperation

const (
	FoldToggle          = text.FoldToggle
	FoldClose           = text.FoldClose
	FoldOpen            = text.FoldOpen
	FoldToggleRecursive = text.FoldToggleRecursive
	FoldCloseRecursive  = text.FoldCloseRecursive
	FoldOpenRecursive   = text.FoldOpenRecursive
	FoldReveal          = text.FoldReveal
	FoldCloseAll        = text.FoldCloseAll
	FoldOpenAll         = text.FoldOpenAll
)

type Operation = text.Operation
