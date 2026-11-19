package editor

import (
	"sort"

	textvim "github.com/wheregrassdreams/muse-editor/text/vim"
	"github.com/wheregrassdreams/muse-editor/markdown/paragraph"
	"github.com/wheregrassdreams/muse-editor/text/input"
	helix "github.com/wheregrassdreams/muse-editor/text/helix"
	"github.com/wheregrassdreams/muse-editor/text/layout"
)

// ApplyOperation executes a keymap-neutral editor intent through the shared
// session contract used by both keymaps and native UI adapters.
func (e *Editor) ApplyOperation(op input.Operation) bool {
	return e.Session.ApplyOperation(op)
}

// applyOperation owns the editor-specific effects of a semantic operation.
// Source, selection and undo state remain canonical in Session.
func (e *Editor) applyOperation(op input.Operation) bool {
	count := max(1, op.Count)
	switch op.Kind {
	case input.OperationBeginCharacterFind:
		e.vimController().SetAwaitingCharacter(op.Direction, op.Till, count)
		return true
	case input.OperationFindCharacter:
		return e.findCharacter(op.Character, op.Direction, op.Till, count)
	case input.OperationRepeatCharacterFind:
		return e.repeatCharacterFind(op.Reverse, count)
	case input.OperationBeginReplace:
		e.vimController().SetAwaitingReplacement(count)
		return true
	case input.OperationReplaceCharacter:
		e.modalReplace(count, op.Character)
		return true
	case input.OperationMoveHorizontal:
		e.moveHorizontalOperation(op.Direction, op.Extend, op.WithinLine)
		return true
	case input.OperationPutRegister:
		return e.putRegister(op.Before, count)
	case input.OperationBeginSearch:
		e.modalStartSearch(op.Direction)
		return true
	case input.OperationSubmitSearch:
		e.modalSubmitSearch()
		return true
	case input.OperationCancelSearch:
		e.modalCancelSearch()
		return true
	case input.OperationRepeatSearch:
		e.modalRepeatSearch(op.Reverse, count)
		return true
	case input.OperationScrollCaretLine:
		return e.scrollCaretLine(op.Placement)
	case input.OperationFoldAtCaret:
		return e.foldAtCaret(op.Fold)
	case input.OperationFoldAll:
		return e.foldAll(op.Fold)
	case input.OperationRevealCaret:
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
	start, end := e.sourceLineRange(e.Caret())
	if direction > 0 {
		return e.Caret() >= end && !e.vimFoldedSoftBreakAt(end)
	}
	if e.Caret() <= start {
		return start == 0 || !e.vimFoldedSoftBreakAt(start-1)
	}
	return false
}

// NavigateToOffset moves the caret to an in-document target and schedules a
// page jump with the requested alignment. Long jumps may animate when the
// SmoothPageJumps preference is enabled; ordinary caret reveal stays instant.
func (e *Editor) NavigateToOffset(off int, align layout.PageJumpAlignment) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	e.clearTableSelectionSession()
	e.dropPhantom()
	off = max(0, min(off, len(e.Source())))
	// Expand against the requested source offset before normalizing. A folded
	// virtual layout can normalize hidden text to its visible heading edge.
	e.expandFoldsAt(off)
	off = e.normalize(off, 1)
	e.setSessionSelection(off, off)
	e.scrollTo, e.hasScrollTo = off, true
	e.pageJump, e.pageJumpAlign = true, align
	e.scrollCenter = align == layout.PageJumpCenter
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
	e.scrollTo, e.hasScrollTo = max(0, min(start, len(e.Source()))), true
	e.pageJump, e.pageJumpAlign = true, layout.PageJumpNearest
	e.scrollCenter, e.scrollPlacement = false, 0
	e.wantReveal = false
	e.paintMu.Unlock()
	e.invalidate()
}

func (e *Editor) scrollCaretLine(placement input.CaretLinePlacement) bool {
	if placement < input.CaretLineTop || placement > input.CaretLineBottom {
		return false
	}
	e.cancelPageJump()
	e.pageJump = false
	e.scrollTo, e.hasScrollTo = e.Caret(), true
	e.scrollPlacement = placement
	e.wantReveal = false
	e.invalidate()
	return true
}

func (e *Editor) foldModels() ([]paragraph.FoldSection, []int) {
	sections := e.foldSections()
	models := make([]paragraph.FoldSection, len(sections))
	folded := make([]int, 0, len(e.folded))
	for i, section := range sections {
		models[i] = section.model
		if e.folded[section.model.HeadingStart] {
			folded = append(folded, section.model.HeadingStart)
		}
	}
	sort.Ints(folded)
	return models, folded
}

func (e *Editor) applyFoldResult(result paragraph.FoldResult) {
	if !result.Changed {
		return
	}
	e.folded = make(map[int]bool, len(result.Folded))
	for _, start := range result.Folded {
		e.folded[start] = true
	}
	if len(e.folded) == 0 {
		e.folded = nil
	}
	if result.MoveCaret {
		e.setCaret(result.Caret, false)
	}
	e.foldRevision++
	e.unitRev = 0
	e.unitList = nil
	e.leavesLaid = false
	e.invalidate()
}

func (e *Editor) foldAtCaret(operation paragraph.FoldOperation) bool {
	sections, folded := e.foldModels()
	focus := e.selectionFocus()
	target, hasTarget := paragraph.FoldSectionAtOffset(sections, focus)
	result := paragraph.ApplyFoldAtOffset(sections, folded, operation, focus, focus)
	if !result.Handled {
		return false
	}
	e.applyFoldResult(result)
	clearSelection := operation == paragraph.FoldCloseRecursive || operation == paragraph.FoldOpenRecursive || operation == paragraph.FoldToggleRecursive
	if operation == paragraph.FoldClose {
		clearSelection = true
	} else if operation == paragraph.FoldToggle && hasTarget {
		for _, start := range result.Folded {
			if start == target.HeadingStart {
				clearSelection = true
				break
			}
		}
	}
	if result.Changed && clearSelection {
		e.clearTableSelection()
	}
	return true
}

func (e *Editor) foldAll(operation paragraph.FoldOperation) bool {
	sections, folded := e.foldModels()
	result := paragraph.ApplyFoldAll(sections, folded, operation)
	if !result.Handled {
		return false
	}
	e.applyFoldResult(result)
	return true
}

func (e *Editor) revealFoldsAtCaret() {
	if e.expandFoldsAt(e.selectionFocus()) {
		e.invalidate()
	}
}

func (e *Editor) findCharacter(char string, dir int, till bool, count int) bool {
	character := textvim.FirstGrapheme(char)
	e.vimController().SetLastFind(character, dir, till)
	sequence := e.vimController().Sequence()
	at, ok := e.modalFind(character, dir, till, count)
	if !ok {
		if sequence.Pending != "" {
			e.vimController().ClearPending()
			e.vimController().SetOperatorCount(0)
		}
		return true
	}
	if sequence.Pending == "d" || sequence.Pending == "c" || sequence.Pending == "y" {
		a, z := e.Caret(), e.nextGraphemeBoundary(e.Caret())
		if at < e.Caret() {
			a, z = at, e.nextGraphemeBoundary(e.Caret())
		} else {
			z = e.nextGraphemeBoundary(at)
		}
		if till && at > e.Caret() {
			z = at
		}
		op := sequence.Pending
		e.vimController().ClearPending()
		e.vimController().SetOperatorCount(0)
		e.modalApply(op, a, z)
	} else {
		mode := e.vimController().Mode()
		e.nativeSetCaret(at, mode == helix.ModeVisual || mode == helix.ModeSelect)
	}
	return true
}

func (e *Editor) repeatCharacterFind(reverse bool, count int) bool {
	state := e.VimViewState()
	sequence := e.vimController().Sequence()
	if !state.FindValid {
		return true
	}
	dir := state.FindDirection
	if reverse {
		dir = -dir
	}
	at, ok := e.modalFind(state.FindCharacter, dir, state.FindTill, count)
	if !ok {
		if sequence.Pending != "" {
			e.vimController().ClearPending()
			e.vimController().SetOperatorCount(0)
		}
		return true
	}
	if sequence.Pending == "d" || sequence.Pending == "c" || sequence.Pending == "y" {
		a, z := e.Caret(), e.nextGraphemeBoundary(e.Caret())
		if dir > 0 {
			if state.FindTill {
				z = at
			} else {
				z = e.nextGraphemeBoundary(at)
			}
		} else {
			a = at
		}
		op := sequence.Pending
		e.vimController().ClearPending()
		e.vimController().SetOperatorCount(0)
		e.modalApply(op, a, z)
	} else {
		mode := e.vimController().Mode()
		e.nativeSetCaret(at, mode == helix.ModeVisual || mode == helix.ModeSelect)
	}
	return true
}
