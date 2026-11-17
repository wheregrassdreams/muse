package editor

import (
	"time"

	"muse/editor/input/vim"
	tableui "muse/nativeui/workbench/features/editor/blocks/table"
)

// modalText is the UI host for the keymap controller. Printable-key grammar
// lives in vim.Controller; this method resolves returned intents against the
// current editor, document structure, and selection.
func (e *Editor) modalText(text string) bool {
	previousNavigation := e.smoothCursorNavigation
	controller := e.vimController()
	mode := controller.Mode()
	searching := controller.View().Searching
	intent, handled := e.vimController().InterpretText(text, time.Now(), e)
	e.smoothCursorNavigation = e.opts.SmoothCursor && mode == vim.ModeNormal && !searching &&
		(intent.Motion != 0 || intent.Kind == vim.IntentFindCharacter || intent.Kind == vim.IntentRepeatCharacterFind ||
			intent.Kind == vim.IntentRepeatSearch || intent.Kind == vim.IntentCommand &&
			(intent.Command.Kind == vim.CommandVertical || intent.Command.Kind == vim.CommandFirstLine))
	defer func() { e.smoothCursorNavigation = previousNavigation }()
	if !handled {
		return false
	}
	defer e.invalidate()
	return true
}

// DispatchVimIntent implements the Vim host boundary. Keymap parsing remains
// in editor/input/vim; document and selection effects resolve here.
func (e *Editor) DispatchVimIntent(intent vim.Intent) {
	if operation, ok := vim.OperationForIntent(intent); ok {
		e.ApplyOperation(operation)
		return
	}
	switch intent.Kind {
	case vim.IntentPrefix, vim.IntentConsumed:
		return
	case vim.IntentCommand:
		e.DispatchVimCommand(intent.Command)
		return
	case vim.IntentSearchText:
		e.modalRefreshSearchPreview()
		return
	case vim.IntentUndo:
		e.Undo()
		e.modalSetMode(vim.ModeNormal)
		e.vimController().SetLinewise(false)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		e.setSessionAnchor(e.Caret())
		return
	case vim.IntentEnterInsert:
		focus := e.nativePrepareInsert()
		switch intent.InsertAt {
		case "a":
			if !e.modalEmptyLineAt(focus) {
				e.setSessionHead(e.step(focus, 1, false))
			}
		case "I":
			e.setSessionHead(e.nativeLineStart(focus, true))
		case "A":
			e.setSessionHead(e.nativeLineEndAt(focus))
		}
		e.setSessionAnchor(e.Caret())
		e.modalSetMode(vim.ModeInsert)
		e.breakCoalescing()
		return
	case vim.IntentOpenLine:
		e.nativePrepareInsert()
		e.nativeOpenLine(intent.Before)
		e.modalSetMode(vim.ModeInsert)
		return
	case vim.IntentEnterVisual, vim.IntentEnterLineVisual:
		e.enterModalVisual(intent)
		return
	case vim.IntentBlockInsert:
		points := make([]int, 0)
		for _, r := range e.SelectionRanges() {
			if intent.InsertAt == "I" {
				points = append(points, r.Start)
			} else {
				points = append(points, r.End)
			}
		}
		e.blockInput = &blockInput{points: points}
		e.block = nil
		e.modalSetMode(vim.ModeInsert)
		e.setSessionAnchor(e.Caret())
		return
	case vim.IntentDeleteCharacter:
		if a, z, ok := e.vimVirtualImageLineRange(); ok {
			e.vimController().SetLinewise(true)
			e.modalApply("d", a, z)
		} else if a, z, ok := e.nativeSubstituteRange(intent.Count, intent.Before); ok {
			e.vimController().SetLinewise(false)
			e.modalApply("d", a, z)
		}
		return
	case vim.IntentDeleteLineRemainder:
		if a, z, ok := e.vimVirtualImageLineRange(); ok {
			e.vimController().SetLinewise(true)
			e.modalApply(intent.Operator, a, z)
		} else if a, z, ok := e.nativeLineRemainder(); ok {
			e.vimController().SetLinewise(false)
			e.modalApply(intent.Operator, a, z)
		}
		return
	}

	if intent.Motion != 0 {
		if intent.Motion != vim.MotionLeft && intent.Motion != vim.MotionRight && intent.Motion != vim.MotionDown && intent.Motion != vim.MotionUp {
			_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.ResetNavigation})
		}
		if intent.Motion != vim.MotionDown && intent.Motion != vim.MotionUp {
			e.hasGoal = false
		}
		start := e.Caret()
		mode := e.vimController().Mode()
		extend := intent.Kind == vim.IntentOperator || mode == vim.ModeVisual || mode == vim.ModeSelect || mode == vim.ModeBlockVisual
		for i := 0; i < max(1, intent.Count); i++ {
			e.applyVimMotion(intent, i, extend)
		}
		if extend {
			e.promoteVisualTextToCellRegion()
		}
		if intent.Kind == vim.IntentOperator {
			if intent.Linewise {
				e.vimController().SetLinewise(true)
			}
			e.modalApply(intent.Operator, start, e.Caret())
		} else if e.Keymap() == "helix" && !extend {
			e.setSessionAnchor(start)
		}
		return
	}
	if intent.Kind == vim.IntentOperator {
		e.applyModalOperator(intent)
		return
	}
}

func (e *Editor) modalSetMode(mode vim.Mode) {
	e.vimController().SetMode(mode)
}

func (e *Editor) enterModalVisual(intent vim.Intent) {
	e.vimController().SetTextVisualActive(e.Keymap() == "vim" && intent.Kind == vim.IntentEnterVisual)
	e.setSessionHead(e.selectionFocus())
	e.tableSelection, e.tableBridge = nil, nil
	e.setSessionAnchor(e.Caret())
	if intent.Kind == vim.IntentEnterLineVisual {
		e.modalSetMode(vim.ModeVisual)
		e.vimController().SetLinewise(true)
		e.setSessionInclusive(false)
		e.setSessionLinewise(true)
		e.nativeSelectLine(e.Caret())
		if intent.Count > 1 {
			e.nativeMoveLineSelection(1, intent.Count-1)
		}
		return
	}
	e.vimController().SetLinewise(false)
	if e.Keymap() == "helix" {
		e.modalSetMode(vim.ModeSelect)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
	} else {
		e.modalSetMode(vim.ModeVisual)
		e.setSessionInclusive(e.vimImageRow == nil)
		e.setSessionLinewise(e.vimImageRow != nil)
	}
}

func (e *Editor) applyVimMotion(intent vim.Intent, iteration int, extend bool) {
	switch intent.Motion {
	case vim.MotionLeft:
		e.modalHorizontal(-1, extend)
	case vim.MotionRight:
		e.modalHorizontal(1, extend)
	case vim.MotionDown:
		if e.moveTableCellRegion(1, 0) {
			return
		}
		if e.vimController().Sequence().Linewise {
			e.nativeMoveLineSelection(1, 1)
			return
		}
		if e.vimController().Mode() == vim.ModeBlockVisual && e.moveTableVisualColumn(1, 1) {
			return
		}
		if !e.moveStructuralRow(1, extend) {
			e.nativeVertical(1, extend)
		}
	case vim.MotionUp:
		if e.moveTableCellRegion(-1, 0) {
			return
		}
		if e.vimController().Sequence().Linewise {
			e.nativeMoveLineSelection(-1, 1)
			return
		}
		if e.vimController().Mode() == vim.ModeBlockVisual && e.moveTableVisualColumn(-1, 1) {
			return
		}
		if !e.moveStructuralRow(-1, extend) {
			e.nativeVertical(-1, extend)
		}
	case vim.MotionWordForward:
		e.nativeSetCaret(e.stepWithWordMode(e.Caret(), 1, true, intent.BigWord), extend)
	case vim.MotionWordBackward:
		e.nativeSetCaret(e.stepWithWordMode(e.Caret(), -1, true, intent.BigWord), extend)
	case vim.MotionWordEnd:
		z := e.wordEndAtMode(e.Caret(), intent.BigWord, false)
		if iteration > 0 {
			if next := e.nextWordEndAcrossUnitsMode(e.Caret(), intent.BigWord); next > e.Caret() {
				z = next
			}
		} else if z <= e.Caret() || intent.Kind != vim.IntentOperator && e.previousGraphemeBoundary(z) <= e.Caret() {
			if next := e.nextWordEndAcrossUnitsMode(e.Caret(), intent.BigWord); next > e.Caret() {
				z = next
			}
		}
		if intent.Kind != vim.IntentOperator && z > e.Caret() {
			z = e.previousGraphemeBoundary(z)
		}
		e.nativeSetCaret(z, extend)
	case vim.MotionLineStart:
		e.nativeSetCaret(e.nativeLineStart(e.Caret(), false), extend)
	case vim.MotionFirstNonBlank:
		e.nativeSetCaret(e.nativeLineStart(e.Caret(), true), extend)
	case vim.MotionLineEnd:
		_, z := e.modalLine(e.Caret())
		if intent.Kind != vim.IntentOperator {
			start, _ := e.modalLine(e.Caret())
			if z > start {
				z = e.previousGraphemeBoundary(z)
			}
		}
		e.nativeSetCaret(z, extend)
	case vim.MotionDocumentEnd:
		e.nativeSetCaret(len(e.Source()), extend)
	}
}

func (e *Editor) applyModalOperator(intent vim.Intent) {
	if intent.Linewise {
		range_ := vim.LinewiseOperatorRange(e.Source(), e.Caret(), intent.Count, intent.Operator)
		e.modalApply(intent.Operator, range_.Start, range_.End)
		return
	}
	if e.vimController().Mode() == vim.ModeBlockVisual && e.block != nil {
		ranges := e.SelectionRanges()
		var rows []string
		for _, r := range ranges {
			rows = append(rows, e.Source()[r.Start:r.End])
		}
		e.setVimBlockRegister(rows)
		if intent.Operator == "y" {
			e.Copy()
			e.startVimYankFlash(ranges)
			e.modalSetMode(vim.ModeNormal)
			e.block = nil
			e.setSessionAnchor(e.Caret())
			return
		}
		at := e.Caret()
		if intent.Operator == "c" {
			e.begin(stepEdit)
			points := make([]int, 0, len(ranges))
			shift := 0
			for _, r := range ranges {
				points = append(points, r.Start-shift)
				shift += r.End - r.Start
			}
			for i := len(ranges) - 1; i >= 0; i-- {
				e.replace(ranges[i].Start, ranges[i].End, "")
			}
			e.blockInput = &blockInput{points: points, started: true}
			if len(points) > 0 {
				at = points[0]
			}
			e.setSessionSelection(at, at)
			e.commit(at, at)
		} else {
			at = e.deleteBlock()
		}
		e.block = nil
		e.setSessionSelection(at, at)
		if intent.Operator == "c" {
			e.modalSetMode(vim.ModeInsert)
		} else {
			e.modalSetMode(vim.ModeNormal)
		}
		return
	}
	if e.tableSelection != nil && e.tableBridge == nil && e.applyTableSelection(intent.Operator) {
		if intent.Operator != "c" {
			e.modalSetMode(vim.ModeNormal)
		}
		return
	}
	mode := e.vimController().Mode()
	if mode == vim.ModeVisual || mode == vim.ModeSelect || mode == vim.ModeBlockVisual {
		a, z := e.selection()
		if a == z {
			z = e.step(z, 1, false)
		}
		e.modalApply(intent.Operator, a, z)
		if intent.Operator != "c" {
			e.modalSetMode(vim.ModeNormal)
		}
	}
}
