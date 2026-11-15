package input

// OperationKind names editor semantics independently of any keymap or UI
// toolkit. Adapters translate physical events into these intents.
type OperationKind string

const (
	OperationBeginCharacterFind  OperationKind = "begin-character-find"
	OperationFindCharacter       OperationKind = "find-character"
	OperationRepeatCharacterFind OperationKind = "repeat-character-find"
	OperationBeginReplace        OperationKind = "begin-replace"
	OperationMoveHorizontal      OperationKind = "move-horizontal"
	OperationPutRegister         OperationKind = "put-register"
	OperationReplaceCharacter    OperationKind = "replace-character"
	OperationBeginSearch         OperationKind = "begin-search"
	OperationSubmitSearch        OperationKind = "submit-search"
	OperationCancelSearch        OperationKind = "cancel-search"
	OperationRepeatSearch        OperationKind = "repeat-search"
	OperationScrollCaretLine     OperationKind = "scroll-caret-line"
	OperationFoldAtCaret         OperationKind = "fold-at-caret"
	OperationFoldAll             OperationKind = "fold-all"
	OperationRevealCaret         OperationKind = "reveal-caret"
)

type CaretLinePlacement uint8

const (
	CaretLineTop CaretLinePlacement = iota + 1
	CaretLineCenter
	CaretLineBottom
)

// PageJumpAlignment describes how a semantic navigation target should land in
// the viewport, independently of the command that initiated it.
type PageJumpAlignment uint8

const (
	PageJumpNearest PageJumpAlignment = iota
	PageJumpTopMargin
	PageJumpCenter
	PageJumpTop
	PageJumpBottom
)

type FoldOperation uint8

const (
	FoldToggle FoldOperation = iota + 1
	FoldClose
	FoldOpen
	FoldToggleRecursive
	FoldCloseRecursive
	FoldOpenRecursive
	FoldReveal
	FoldCloseAll
	FoldOpenAll
)

// Operation is a keymap-neutral editor intent. Fields unused by Kind are
// ignored. It contains no input-device or presentation types.
type Operation struct {
	Kind       OperationKind
	Count      int
	Direction  int
	Character  string
	Till       bool
	Reverse    bool
	WithinLine bool
	Extend     bool
	Before     bool
	Placement  CaretLinePlacement
	Fold       FoldOperation
}
