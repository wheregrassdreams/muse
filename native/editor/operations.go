package editor

// EditorOperationKind names editor semantics independently of any keymap.
// Keymaps and UI controls translate input into these operations; source,
// selection, folding, and viewport behavior remain owned by Editor.
type EditorOperationKind string

const (
	OperationBeginCharacterFind  EditorOperationKind = "begin-character-find"
	OperationFindCharacter       EditorOperationKind = "find-character"
	OperationRepeatCharacterFind EditorOperationKind = "repeat-character-find"
	OperationBeginReplace        EditorOperationKind = "begin-replace"
	OperationMoveHorizontal      EditorOperationKind = "move-horizontal"
	OperationPutRegister         EditorOperationKind = "put-register"
	OperationReplaceCharacter    EditorOperationKind = "replace-character"
	OperationBeginSearch         EditorOperationKind = "begin-search"
	OperationSubmitSearch        EditorOperationKind = "submit-search"
	OperationCancelSearch        EditorOperationKind = "cancel-search"
	OperationRepeatSearch        EditorOperationKind = "repeat-search"
	OperationScrollCaretLine     EditorOperationKind = "scroll-caret-line"
	OperationFoldAtCaret         EditorOperationKind = "fold-at-caret"
	OperationFoldAll             EditorOperationKind = "fold-all"
	OperationRevealCaret         EditorOperationKind = "reveal-caret"
)

type CaretLinePlacement uint8

const (
	CaretLineTop CaretLinePlacement = iota + 1
	CaretLineCenter
	CaretLineBottom
)

// PageJumpAlignment describes how a semantic in-document navigation target
// should land in the viewport. It is shared by outline, footnote, anchor and
// search navigation, independently of the input keymap that initiated it.
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

// EditorOperation is a keymap-neutral editor intent. Unused fields are
// ignored by the selected Kind.
type EditorOperation struct {
	Kind       EditorOperationKind
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

// ApplyOperation executes a keymap-neutral editor intent through the shared
// session contract used by both keymaps and native UI adapters.
func (e *Editor) ApplyOperation(op EditorOperation) bool {
	return e.Session.ApplyOperation(op)
}

// applyOperation owns the editor-specific effects of a semantic operation.
// Source, selection and undo state remain canonical in Session.
func (e *Editor) applyOperation(op EditorOperation) bool {
	count := max(1, op.Count)
	switch op.Kind {
	case OperationBeginCharacterFind:
		e.modal.awaitingCharDirection = op.Direction
		e.modal.awaitingCharTill = op.Till
		e.modal.charCount = count
		return true
	case OperationFindCharacter:
		return e.findCharacter(op.Character, op.Direction, op.Till, count)
	case OperationRepeatCharacterFind:
		return e.repeatCharacterFind(op.Reverse, count)
	case OperationBeginReplace:
		e.modal.awaitingReplacement = true
		e.modal.charCount = count
		return true
	case OperationReplaceCharacter:
		e.modalReplace(count, op.Character)
		return true
	case OperationMoveHorizontal:
		e.moveHorizontalOperation(op.Direction, op.Extend, op.WithinLine)
		return true
	case OperationPutRegister:
		return e.putRegister(op.Before, count)
	case OperationBeginSearch:
		e.modalStartSearch(op.Direction)
		return true
	case OperationSubmitSearch:
		e.modalSubmitSearch()
		return true
	case OperationCancelSearch:
		e.modalCancelSearch()
		return true
	case OperationRepeatSearch:
		e.modalRepeatSearch(op.Reverse, count)
		return true
	case OperationScrollCaretLine:
		return e.scrollCaretLine(op.Placement)
	case OperationFoldAtCaret:
		return e.foldAtCaret(op.Fold)
	case OperationFoldAll:
		return e.foldAll(op.Fold)
	case OperationRevealCaret:
		e.revealFoldsAtCaret()
		return true
	default:
		return false
	}
}

// moveHorizontalOperation applies a horizontal caret movement. WithinLine
// gives consumers such as Vim the usual h/l boundary behavior while keeping
// cross-block movement available to other editor consumers.
func (e *Editor) moveHorizontalOperation(direction int, extend, withinLine bool) {
	if direction == 0 {
		return
	}
	if withinLine && e.horizontalLineBoundary(direction) {
		return
	}
	e.nativeHorizontal(direction, extend)
}

func (e *Editor) horizontalLineBoundary(direction int) bool {
	if e.tableSelection != nil || e.tableBridge != nil {
		return false
	}
	start, end := e.sourceLineRange(e.caret)
	if direction > 0 {
		return e.caret >= end && !e.vimFoldedSoftBreakAt(end)
	}
	if e.caret <= start {
		return start == 0 || !e.vimFoldedSoftBreakAt(start-1)
	}
	return false
}

// NavigateToOffset moves the caret to an in-document target and schedules a
// page jump with the requested alignment. Long jumps may animate when the
// SmoothPageJumps preference is enabled; ordinary caret reveal stays instant.
func (e *Editor) NavigateToOffset(off int, align PageJumpAlignment) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	e.clearTableSelectionSession()
	e.dropPhantom()
	off = max(0, min(off, len(e.src)))
	// Expand against the requested source offset before normalizing. A folded
	// virtual layout can normalize hidden text to its visible heading edge.
	e.expandFoldsAt(off)
	off = e.normalize(off, 1)
	e.anchor, e.caret = off, off
	e.scrollTo, e.hasScrollTo = off, true
	e.pageJump, e.pageJumpAlign = true, align
	e.scrollCenter = align == PageJumpCenter
	e.scrollPlacement = 0
	e.wantReveal = false
	e.hasGoal = false
	e.invalidate()
}

// NavigateToRange selects a search or reference range and brings it into
// view through the same page-jump semantics as NavigateToOffset.
func (e *Editor) NavigateToRange(start, end int) {
	e.Select(start, end)
	e.paintMu.Lock()
	e.scrollTo, e.hasScrollTo = max(0, min(start, len(e.src))), true
	e.pageJump, e.pageJumpAlign = true, PageJumpNearest
	e.scrollCenter, e.scrollPlacement = false, 0
	e.wantReveal = false
	e.paintMu.Unlock()
	e.invalidate()
}

func (e *Editor) scrollCaretLine(placement CaretLinePlacement) bool {
	if placement < CaretLineTop || placement > CaretLineBottom {
		return false
	}
	e.cancelPageJump()
	e.pageJump = false
	e.scrollTo, e.hasScrollTo = e.caret, true
	e.scrollPlacement = placement
	e.wantReveal = false
	e.invalidate()
	return true
}

func (e *Editor) foldHeadingAtCaret() *leaf {
	off := e.selectionFocus()
	var target *leaf
	for _, section := range e.foldSections() {
		h := section.heading
		if off >= h.lineStart && off < section.to && (target == nil || h.lineStart >= target.lineStart) {
			target = h
		}
	}
	return target
}

func (e *Editor) foldAtCaret(operation FoldOperation) bool {
	if operation == FoldReveal {
		e.revealFoldsAtCaret()
		return true
	}
	h := e.foldHeadingAtCaret()
	if h == nil {
		return false
	}
	_, canFold := e.foldSectionAtHeading(h)
	if !canFold {
		return false
	}
	isClosed := e.isFoldedHeading(h)
	switch operation {
	case FoldToggle:
		e.toggleFold(h)
	case FoldClose:
		if !isClosed {
			e.toggleFold(h)
		}
	case FoldOpen:
		if isClosed {
			e.toggleFold(h)
		}
	case FoldCloseRecursive:
		e.setRecursiveFold(h, true)
	case FoldOpenRecursive:
		e.setRecursiveFold(h, false)
	case FoldToggleRecursive:
		e.setRecursiveFold(h, !isClosed)
	default:
		return false
	}
	return true
}

func (e *Editor) setRecursiveFold(h *leaf, close bool) {
	section, ok := e.foldSectionAtHeading(h)
	if !ok {
		return
	}
	if close && e.folded == nil {
		e.folded = make(map[int]bool)
	}
	for _, child := range e.foldSections() {
		if child.heading.lineStart < h.lineStart || child.heading.lineStart >= section.to || !child.content {
			continue
		}
		if close {
			e.folded[child.heading.lineStart] = true
		} else {
			delete(e.folded, child.heading.lineStart)
		}
	}
	if close && e.isFoldedSourceOffset(e.selectionFocus()) {
		e.setCaret(h.end, false)
	}
	e.clearTableSelection()
	e.foldRevision++
	e.unitRev = 0
	e.unitList = nil
	e.leavesLaid = false
	e.invalidate()
}

func (e *Editor) foldAll(operation FoldOperation) bool {
	switch operation {
	case FoldCloseAll:
		closed := make(map[int]bool)
		for _, section := range e.foldSections() {
			if section.content {
				closed[section.heading.lineStart] = true
			}
		}
		e.folded = closed
	case FoldOpenAll:
		e.folded = nil
	default:
		return false
	}
	e.foldRevision++
	e.unitRev = 0
	e.unitList = nil
	e.leavesLaid = false
	e.invalidate()
	return true
}

func (e *Editor) revealFoldsAtCaret() {
	if e.expandFoldsAt(e.selectionFocus()) {
		e.invalidate()
	}
}

func (e *Editor) findCharacter(char string, dir int, till bool, count int) bool {
	m := &e.modal
	m.findChar, m.findDir, m.findTill, m.findValid = modalGrapheme(char), dir, till, true
	at, ok := e.modalFind(m.findChar, dir, till, count)
	if !ok {
		if m.sequence.Pending != "" {
			m.sequence.Pending, m.sequence.OperatorCount = "", 0
		}
		return true
	}
	if m.sequence.Pending == "d" || m.sequence.Pending == "c" || m.sequence.Pending == "y" {
		a, z := e.caret, e.nextGraphemeBoundary(e.caret)
		if at < e.caret {
			a, z = at, e.nextGraphemeBoundary(e.caret)
		} else {
			z = e.nextGraphemeBoundary(at)
		}
		if till && at > e.caret {
			z = at
		}
		op := m.sequence.Pending
		m.sequence.Pending, m.sequence.OperatorCount = "", 0
		e.modalApply(op, a, z)
	} else {
		e.nativeSetCaret(at, m.mode == "Visual" || m.mode == "Select")
	}
	return true
}

func (e *Editor) repeatCharacterFind(reverse bool, count int) bool {
	m := &e.modal
	if !m.findValid {
		return true
	}
	dir := m.findDir
	if reverse {
		dir = -dir
	}
	at, ok := e.modalFind(m.findChar, dir, m.findTill, count)
	if !ok {
		if m.sequence.Pending != "" {
			m.sequence.Pending, m.sequence.OperatorCount = "", 0
		}
		return true
	}
	if m.sequence.Pending == "d" || m.sequence.Pending == "c" || m.sequence.Pending == "y" {
		a, z := e.caret, e.nextGraphemeBoundary(e.caret)
		if dir > 0 {
			if m.findTill {
				z = at
			} else {
				z = e.nextGraphemeBoundary(at)
			}
		} else {
			a = at
		}
		op := m.sequence.Pending
		m.sequence.Pending, m.sequence.OperatorCount = "", 0
		e.modalApply(op, a, z)
	} else {
		e.nativeSetCaret(at, m.mode == "Visual" || m.mode == "Select")
	}
	return true
}
