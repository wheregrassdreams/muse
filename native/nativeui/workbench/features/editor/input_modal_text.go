package editor

import (
	"strings"
	"time"

	"github.com/wheregrassdreams/muse-editor/markdown/paragraph"
	tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"
	textengine "github.com/wheregrassdreams/muse-editor/text"
	helix "github.com/wheregrassdreams/muse-editor/text/helix"
	"github.com/wheregrassdreams/muse-editor/text/input"
	textvim "github.com/wheregrassdreams/muse-editor/text/vim"
	tableui "muse/nativeui/workbench/features/editor/blocks/table"
)

// modalText is the UI host for the keymap controller. Vim printable-key
// grammar lives in muse-editor/text/vim; Helix remains host-owned.
func (e *Editor) modalText(text string) bool {
	previousNavigation := e.smoothCursorNavigation
	controller := e.vimController()
	mode := controller.Mode()
	view := controller.View()
	searching := view.Searching
	if e.Keymap() == "vim" && controller.Sequence().Pending == "z" &&
		!view.PrefixStartedAt.IsZero() && time.Since(view.PrefixStartedAt) > 900*time.Millisecond {
		controller.ClearPending()
		return true
	}
	// Insert-mode text is delivered through the editor's text-input path, not
	// interpreted a second time as Vim command text. The key path still handles
	// non-text keys through modalKey.
	if e.Keymap() == "vim" && mode == helix.ModeInsert && !searching {
		return false
	}
	if e.Keymap() == "vim" && view.AwaitingReplacement &&
		(mode == helix.ModeVisual || mode == helix.ModeSelect) && text != "" {
		controller.ClearAwaitingReplacement()
		controller.ClearPending()
		e.modalReplace(0, text)
		return true
	}
	// Visual replacement is selection-based, while the Vim text engine's `r`
	// intent is intentionally a caret-range command. Keep the selection
	// semantics at the host boundary, where the actual UI selection is known.
	if e.Keymap() == "vim" && mode == helix.ModeVisual && text == "r" {
		controller.SetAwaitingReplacement(1)
		return true
	}
	if e.Keymap() == "vim" && mode == helix.ModeVisual && text == "v" && controller.Sequence().Linewise {
		controller.SetTextVisualActive(true)
		controller.SetLinewise(false)
		e.setSessionLinewise(false)
		e.setSessionInclusive(true)
		controller.SetMode(helix.ModeVisual)
		e.invalidate()
		return true
	}
	if e.Keymap() == "vim" && controller.Sequence().Linewise && e.tableSelection == nil &&
		(mode == helix.ModeVisual || mode == helix.ModeSelect) && (text == "d" || text == "c" || text == "y") {
		if e.applyVimLinewiseOperator(text) {
			e.invalidate()
			return true
		}
	}
	// Helix keeps a text selection active while the mode label remains Normal.
	// Resolve selection operators here, where the editor can see the current
	// source range, instead of letting the context-free controller start a
	// pending Vim-style operator sequence.
	if e.Keymap() == "helix" && mode == helix.ModeNormal && !searching {
		switch text {
		case "d", "c", "y":
			a, z := e.selection()
			if a != z {
				e.modalApply(text, a, z)
				e.invalidate()
				return true
			}
		}
	}
	if e.Keymap() == "vim" {
		// Rectangular text selections are owned by the UI host. The standalone
		// Vim keymap sees its underlying Normal mode while this host-only mode
		// override is active, so dispatch block operators and multi-caret insert
		// commands before the text engine interprets them as ordinary motions.
		if mode == helix.ModeBlockVisual && e.block != nil {
			switch text {
			case "c", "d", "y":
				e.applyModalOperator(helix.Intent{Operator: text})
				e.invalidate()
				return true
			case "I", "A":
				e.DispatchVimIntent(helix.Intent{Kind: helix.IntentBlockInsert, InsertAt: text})
				e.invalidate()
				return true
			}
		}
		// muse-editor owns text operators, while a table column selection is a
		// host structural operation. Its block-change path must establish the
		// table's multi-insert session before the text engine sees printable c.
		if text == "c" && mode == helix.ModeBlockVisual && e.tableSelection != nil &&
			!e.tableSelection.cellRegion && e.tableSelection.axis == tableColumns && e.applyTableSelection("c") {
			e.invalidate()
			return true
		}
		// Structural table selections are owned by the UI host. The text engine
		// can only yank/delete their source range, which loses row/cell shape and
		// leaves later table-register paste without a structural destination.
		if (text == "d" || text == "y") && e.tableSelection != nil && e.tableBridge == nil &&
			(mode == helix.ModeVisual || mode == helix.ModeBlockVisual) && e.applyTableSelection(text) {
			e.modalSetMode(helix.ModeNormal)
			e.vimController().SetLinewise(false)
			e.setSessionInclusive(false)
			e.setSessionLinewise(false)
			e.invalidate()
			return true
		}
		motionCount := controller.Sequence().Count
		operatorCount := controller.Sequence().OperatorCount
		beforeTableMotion := e.tableNavigationState()
		intent, handled := e.Session.HandleVimKey(textengine.KeyEvent{Text: text})
		if !handled {
			return false
		}
		// The text engine reports source offsets for motions, but a table
		// selection owns its own focus. Preserve the count before the keymap
		// consumes it so structural motion never has to infer a count from a
		// source range that may be unrelated to the active table cell.
		if intent.Name == "vim.move-horizontal" || intent.Name == "vim.move-vertical" || intent.Name == "vim.move-caret" {
			intent.Count = max(1, motionCount)
		}
		if intent.Name == "vim.delete-range" || intent.Name == "vim.change-range" {
			intent.Count = max(intent.Count, max(motionCount, operatorCount))
		}
		if intent.Name == "vim.extend-caret" && (text == "j" || text == "k" || text == "ArrowDown" || text == "ArrowUp") {
			intent.Count = max(1, motionCount)
		}
		if intent.Name == "vim.start-line-selection" || intent.Name == "vim.start-selection" {
			intent.Count = max(intent.Count, max(1, motionCount))
		}
		e.smoothCursorNavigation = e.opts.SmoothCursor && mode == helix.ModeNormal && !searching &&
			(strings.HasPrefix(intent.Name, "vim.move-") || intent.Name == textvim.IntentMoveDisplayUp || intent.Name == textvim.IntentMoveDisplayDown)
		defer func() { e.smoothCursorNavigation = previousNavigation }()
		if e.dispatchTableVisualRowMotion(text, intent, beforeTableMotion) {
			e.restoreTextVisualAfterTableMotion(beforeTableMotion)
			e.invalidate()
			return true
		}
		if e.dispatchTableVisualHorizontalMotion(text, intent, beforeTableMotion) {
			e.restoreTextVisualAfterTableMotion(beforeTableMotion)
			e.invalidate()
			return true
		}
		e.dispatchTextVimIntent(text, intent)
		e.recordVisualTextTablePromotion(beforeTableMotion, text, max(1, intent.Count))
		e.restoreTextVisualAfterTableMotion(beforeTableMotion)
		e.invalidate()
		return true
	}
	intent, handled := e.legacyVimController().InterpretText(text, time.Now())
	e.smoothCursorNavigation = e.opts.SmoothCursor && mode == helix.ModeNormal && !searching &&
		(intent.Motion != 0 || intent.Kind == helix.IntentCommand &&
			(intent.Command.Kind == helix.CommandVertical || intent.Command.Kind == helix.CommandFirstLine))
	defer func() { e.smoothCursorNavigation = previousNavigation }()
	if !handled {
		return false
	}
	e.DispatchVimIntent(intent)
	defer e.invalidate()
	return true
}

func (e *Editor) applyVimLinewiseOperator(operator string) bool {
	selection := e.Session.Selection()
	first, _ := e.sourceLineRange(min(selection.Anchor, selection.Head))
	last, _ := e.sourceLineRange(max(selection.Anchor, selection.Head))
	_, registerEnd := e.sourceLineRange(last)
	deleteStart, deleteEnd := first, registerEnd
	if deleteEnd < len(e.Source()) && e.Source()[deleteEnd] == '\n' {
		deleteEnd++
	} else if deleteEnd == len(e.Source()) && deleteStart > 0 {
		deleteStart--
	}
	if deleteStart < 0 || registerEnd < first || deleteEnd < deleteStart {
		return false
	}
	register := e.Source()[first:registerEnd]
	e.setVimTextRegister(register, true)
	e.vimController().SetLinewise(false)
	e.setSessionSelectionMode(false, false)
	if operator == "y" {
		e.setSessionSelection(first, registerEnd)
		e.Copy()
		e.startVimYankFlash([]Range{{Start: first, End: registerEnd}})
		e.setSessionSelection(first, first)
		e.modalSetMode(helix.ModeNormal)
		return true
	}
	e.begin(stepEdit)
	e.replace(deleteStart, deleteEnd, "")
	e.commit(deleteStart, deleteStart)
	e.modalSetMode(helix.ModeNormal)
	if operator == "c" {
		e.modalSetMode(helix.ModeInsert)
	}
	return true
}

// dispatchTableVisualHorizontalMotion routes visual h/l through the host when
// either end of the selection is in a table. The text engine's extend-caret
// intent has only a source offset; it cannot promote a cross-cell selection to
// the table's structural region or preserve that region on the next motion.
func (e *Editor) dispatchTableVisualHorizontalMotion(key string, intent textengine.Intent, before tablemodel.Snapshot) bool {
	if (intent.Name != "vim.extend-caret" && intent.Name != "vim.move-horizontal") || e.vimController().Mode() != helix.ModeVisual {
		return false
	}
	direction := 0
	switch key {
	case "h", "ArrowLeft":
		direction = -1
	case "l", "ArrowRight":
		direction = 1
	default:
		return false
	}
	// Characterwise text selection remains owned by the session until it
	// crosses a cell boundary. Applying the engine's destination directly
	// avoids recording the pre-promotion text state as a table navigation step;
	// the complete post-promotion state is recorded below for exact reversal.
	if e.Keymap() == "vim" && e.tableSelection == nil && e.tableBridge == nil && e.tableDocumentRegion == nil {
		inTable := func(offset int) bool {
			_, _, _, ok := e.tableCellAtSourceOffset(offset)
			return ok
		}
		if !inTable(e.Caret()) && !inTable(int(intent.Range.From)) && !inTable(e.Session.Selection().Anchor) {
			return false
		}
		e.nativeSetCaret(int(intent.Range.From), true)
		e.promoteVisualTextToCellRegion()
		e.recordVisualTextTablePromotion(before, key, max(1, intent.Count))
		if e.tableSelection != nil && e.tableSelection.cellRegion && e.tableSelection.origin == tablemodel.SelectionOriginVisualText {
			e.vimController().SetTextVisualActive(true)
		}
		return true
	}
	// The engine's move-horizontal intent is ordinary text navigation. Only
	// extend-caret represents a table-owned visual extension here; letting a
	// move-horizontal intent reach dispatchTextVimIntent preserves its own
	// direction/count semantics.
	if intent.Name != "vim.extend-caret" {
		return false
	}
	inTable := func(offset int) bool {
		_, _, _, ok := e.tableCellAtSourceOffset(offset)
		return ok
	}
	if !inTable(e.Caret()) && !inTable(int(intent.Range.From)) && !inTable(e.Session.Selection().Anchor) {
		return false
	}
	for range max(1, intent.Count) {
		e.modalHorizontal(direction, true)
	}
	e.recordVisualTextTablePromotion(before, key, max(1, intent.Count))
	return true
}

// dispatchTableVisualRowMotion maps visual row motions through the table's
// structural focus. The text session's caret can remain outside that focus,
// so applying helix.extend-caret's source offset directly would move the wrong
// selection.
func (e *Editor) dispatchTableVisualRowMotion(key string, intent textengine.Intent, before tablemodel.Snapshot) bool {
	if (intent.Name != "vim.extend-caret" && intent.Name != "vim.move-vertical") || e.vimController().Mode() != helix.ModeVisual {
		return false
	}
	if e.Keymap() == "vim" && e.tableSelection == nil && e.vimController().View().VimTextVisualActive {
		direction := 0
		switch key {
		case "j", "ArrowDown":
			direction = 1
		case "k", "ArrowUp":
			direction = -1
		default:
			return false
		}
		anchorTable, anchorRow, anchorCol, anchorOK := e.tableCellAtSourceOffset(e.Session.Selection().Anchor)
		focusTable, focusRow, focusCol, focusOK := e.tableCellAtSourceOffset(e.Caret())
		if anchorOK && focusOK && anchorTable.lineStart == focusTable.lineStart {
			targetRow := focusRow + direction*max(1, intent.Count)
			if targetRow >= 0 && targetRow < len(focusTable.tab.rows) {
				target := e.tableVisualCaretAtRow(focusTable, targetRow, e.Caret())
				e.setSessionHead(target)
				if e.selectTableCellRegion(anchorTable, anchorRow, anchorCol, targetRow, min(focusCol, len(focusTable.tab.rows[targetRow])-1)) {
					e.recordVisualTextTablePromotion(before, key, max(1, intent.Count))
					return true
				}
			}
		}
	}
	if e.tableVisualLineMode() {
		if e.tableSelection == nil {
			return e.restoreTableVisualLineTarget(int(intent.Range.From))
		}
		if e.tableSelection.axis != tableRows {
			return false
		}
		direction := 0
		switch key {
		case "j", "ArrowDown":
			direction = 1
		case "k", "ArrowUp":
			direction = -1
		default:
			return false
		}
		count := max(1, intent.Count)
		for remaining := count; remaining > 0; remaining-- {
			s := e.tableSelection
			t := e.tableSelectionLeaf()
			if s == nil || t == nil {
				return false
			}
			next := s.active + direction
			if next < 0 || next >= len(t.tab.rows) {
				target := int(intent.Range.From)
				if e.tableVisualRowAtOffset(t, target) >= 0 {
					if direction > 0 {
						target = min(len(e.Source()), blockEnd(e.Source(), t)+1)
					} else {
						target = max(0, t.lineStart-1)
					}
				}
				if !e.moveTableVisualLineCaret(target) {
					return false
				}
				if remaining > 1 {
					e.nativeMoveLineSelection(direction, remaining-1)
				}
				return true
			}
			if !e.moveStructuralRow(direction, true) {
				return false
			}
		}
		return true
	}
	if s := e.tableSelection; s != nil && s.cellRegion {
		direction := 0
		switch key {
		case "j", "ArrowDown":
			direction = 1
		case "k", "ArrowUp":
			direction = -1
		default:
			return false
		}
		t := e.tableSelectionLeaf()
		if t == nil {
			return false
		}
		row := s.focusRow + direction*max(1, intent.Count)
		if row < 0 || row >= len(t.tab.rows) {
			return false
		}
		col := min(s.focusCol, len(t.tab.rows[row])-1)
		caret := e.tableVisualCaretAtRow(t, row, e.Caret())
		if !e.selectTableCellRegion(t, s.anchorRow, s.anchorCol, row, col) {
			return false
		}
		e.setSessionHead(caret)
		return true
	}
	if e.tableSelection == nil || e.tableSelection.axis != tableRows {
		return false
	}
	direction := 0
	switch key {
	case "j", "ArrowDown":
		direction = 1
	case "k", "ArrowUp":
		direction = -1
	default:
		return false
	}
	return e.moveStructuralRow(direction, true)
}

func (e *Editor) recordVisualTextTablePromotion(before tablemodel.Snapshot, key string, count int) {
	if before.Mode != string(helix.ModeVisual) || before.Selection != nil || before.DocumentRegion != nil {
		return
	}
	selection := e.tableSelection
	if selection == nil || !selection.cellRegion || selection.origin != tablemodel.SelectionOriginVisualText {
		return
	}
	rowDelta, colDelta := 0, 0
	switch key {
	case "j", "ArrowDown":
		rowDelta = max(1, count)
	case "k", "ArrowUp":
		rowDelta = -max(1, count)
	case "l", "ArrowRight":
		colDelta = max(1, count)
	case "h", "ArrowLeft":
		colDelta = -max(1, count)
	default:
		return
	}
	e.recordTableNavigation(before, rowDelta, colDelta)
}

func (e *Editor) restoreTextVisualAfterTableMotion(before tablemodel.Snapshot) {
	if e.Keymap() != "vim" || before.Mode != string(helix.ModeVisual) || e.vimController().Mode() != helix.ModeVisual {
		return
	}
	origin := tablemodel.SelectionOriginText
	if before.Selection != nil {
		origin = before.Selection.Origin
	} else if before.Bridge != nil {
		origin = before.Bridge.Origin
	} else if before.DocumentRegion != nil {
		origin = before.DocumentRegion.Origin
	}
	if origin == tablemodel.SelectionOriginVisualText {
		e.vimController().SetTextVisualActive(true)
	}
}

// dispatchTextVimIntent resolves an engine intent against MyGo's document
// geometry and platform state. Text edits and ordinary source motions execute
// in the independent text session; visual line geometry and non-text objects
// remain host operations.
func (e *Editor) dispatchTextVimIntent(key string, intent textengine.Intent) {
	keymap := e.vimController()
	visual := keymap.Mode() == helix.ModeVisual || keymap.Mode() == helix.ModeSelect || keymap.Mode() == helix.ModeBlockVisual
	if e.Keymap() == "vim" && key == "y" && intent.Name == "vim.collapse-selection" {
		if a, z, ok := e.vimVirtualImageLineRange(); ok {
			e.vimController().SetLinewise(true)
			e.modalApply("y", a, z)
			return
		}
	}
	if e.Keymap() == "vim" && key == "x" && intent.Name == "vim.delete-range" {
		if a, z, ok := e.vimVirtualImageLineRange(); ok {
			e.vimController().SetLinewise(true)
			e.modalApply("d", a, z)
			return
		}
	}
	if e.dispatchVimWordIntent(key, intent, visual) {
		return
	}
	if e.Keymap() == "vim" && key == "y" && intent.Name == "vim.collapse-selection" {
		if start, end, ok := e.vimYankFlashRange(keymap.View().Register, keymap.View().RegisterLine); ok {
			e.startVimYankFlash([]Range{{Start: start, End: end}})
		}
	}
	switch intent.Name {
	case "vim.extend-caret":
		if e.Keymap() == "vim" {
			if _, virtualImageRow := e.vimImageRowState(); virtualImageRow &&
				(key == "j" || key == "k" || key == "ArrowDown" || key == "ArrowUp") {
				direction := 1
				if key == "k" || key == "ArrowUp" {
					direction = -1
				}
				for range max(1, intent.Count) {
					e.nativeVertical(direction, true)
				}
				return
			}
		}
		// Structural and bridge selections own their focus. Applying an engine
		// source offset here can turn a reversible row/column selection into a
		// cell rectangle and discard the bridge provenance.
		if e.tableSelection != nil || e.tableBridge != nil || e.tableDocumentRegion != nil {
			return
		}
		e.nativeSetCaret(int(intent.Range.From), true)
		e.promoteVisualTextToCellRegion()
		if e.tableSelection != nil && e.tableSelection.cellRegion && e.tableSelection.origin == tablemodel.SelectionOriginVisualText {
			e.vimController().SetTextVisualActive(true)
		}
		return
	case textvim.IntentMoveDisplayUp, textvim.IntentMoveDisplayDown, "vim.move-vertical":
		direction := 1
		if intent.Name == textvim.IntentMoveDisplayUp || key == "k" || key == "ArrowUp" ||
			(intent.Name != "vim.move-vertical" && intent.Range.From < textengine.Offset(e.Caret())) {
			direction = -1
		}
		count := max(1, intent.Count)
		if intent.Name == "vim.move-vertical" && keymap.Mode() == helix.ModeBlockVisual && e.tableSelection != nil && !e.tableSelection.cellRegion && e.tableSelection.axis == tableColumns {
			e.moveTableVisualColumn(direction, count)
			return
		}
		_, virtualImageRow := e.vimImageRowState()
		if intent.Name == "vim.move-vertical" && virtualImageRow {
			// The engine's source target may span several logical lines for a
			// single key while the caret is inside a tall image. Virtual rows are
			// visual positions over one source line, so preserve the key count.
			count = max(1, intent.Count)
		}
		for range count {
			if !e.moveTableCellRegion(direction, 0) {
				if !e.moveStructuralRow(direction, visual) {
					e.nativeVertical(direction, visual)
				}
			}
		}
		if visual {
			e.promoteVisualTextToCellRegion()
		}
		return
	case "vim.move-horizontal":
		direction := 1
		if key == "h" || key == "ArrowLeft" || intent.Range.From < textengine.Offset(e.Caret()) && key == "" {
			direction = -1
		}
		count := e.vimGraphemeDistance(e.Caret(), int(intent.Range.From))
		if visual || e.tableSelection != nil && e.tableSelection.cellRegion {
			count = max(1, intent.Count)
		}
		for range count {
			e.modalHorizontal(direction, visual)
		}
		return
	case "vim.start-selection":
		e.enterModalVisual(helix.Intent{Kind: helix.IntentEnterVisual})
		return
	case "vim.start-line-selection":
		e.enterModalVisual(helix.Intent{Kind: helix.IntentEnterLineVisual, Count: max(1, intent.Count)})
		e.Session.VimKeymap().SetMode(textvim.LineVisual)
		return
	case "vim.replace-range":
		e.modalReplaceRange(int(intent.Range.From), int(intent.Range.To), intent.Text)
		return
	case "vim.append", "vim.insert-line-first", "vim.insert-line-end", "vim.open-line-below", "vim.open-line-above":
		focus := e.nativePrepareInsert()
		switch key {
		case "a":
			if !e.modalEmptyLineAt(focus) {
				e.setSessionHead(e.step(focus, 1, false))
			}
		case "I":
			e.setSessionHead(e.nativeLineStart(focus, true))
		case "A":
			e.setSessionHead(e.nativeLineEndAt(focus))
		case "o":
			e.nativeOpenLine(false)
		case "O":
			e.nativeOpenLine(true)
		}
		e.setSessionAnchor(e.Caret())
		e.modalSetMode(helix.ModeInsert)
		e.breakCoalescing()
		return
	case textvim.IntentPasteBlockBefore, textvim.IntentPasteBlockAfter:
		before := intent.Name == textvim.IntentPasteBlockBefore
		e.putRegister(before, max(1, intent.Count))
		return
	case textvim.IntentPasteTableBefore, textvim.IntentPasteTableAfter:
		before := intent.Name == textvim.IntentPasteTableBefore
		e.putRegister(before, max(1, intent.Count))
		return
	case textvim.IntentScrollLineTop:
		e.ApplyOperation(input.Operation{Kind: input.OperationScrollCaretLine, Placement: input.CaretLineTop})
		return
	case textvim.IntentScrollLineCenter:
		e.ApplyOperation(input.Operation{Kind: input.OperationScrollCaretLine, Placement: input.CaretLineCenter})
		return
	case textvim.IntentScrollLineBottom:
		e.ApplyOperation(input.Operation{Kind: input.OperationScrollCaretLine, Placement: input.CaretLineBottom})
		return
	case textvim.IntentFoldClose:
		e.ApplyOperation(input.Operation{Kind: input.OperationFoldAtCaret, Fold: paragraph.FoldClose})
		return
	case textvim.IntentFoldCloseNested:
		e.ApplyOperation(input.Operation{Kind: input.OperationFoldAtCaret, Fold: paragraph.FoldCloseRecursive})
		return
	case textvim.IntentFoldOpen:
		e.ApplyOperation(input.Operation{Kind: input.OperationFoldAtCaret, Fold: paragraph.FoldOpen})
		return
	case textvim.IntentFoldOpenNested:
		e.ApplyOperation(input.Operation{Kind: input.OperationFoldAtCaret, Fold: paragraph.FoldOpenRecursive})
		return
	case textvim.IntentFoldToggle:
		e.ApplyOperation(input.Operation{Kind: input.OperationFoldAtCaret, Fold: paragraph.FoldToggle})
		return
	case textvim.IntentFoldToggleNested:
		e.ApplyOperation(input.Operation{Kind: input.OperationFoldAtCaret, Fold: paragraph.FoldToggleRecursive})
		return
	case textvim.IntentFoldCloseAll:
		e.ApplyOperation(input.Operation{Kind: input.OperationFoldAll, Fold: paragraph.FoldCloseAll})
		return
	case textvim.IntentFoldOpenAll:
		e.ApplyOperation(input.Operation{Kind: input.OperationFoldAll, Fold: paragraph.FoldOpenAll})
		return
	case textvim.IntentRevealCaret:
		e.ApplyOperation(input.Operation{Kind: input.OperationRevealCaret})
		return
	}
	if intent.Name == "vim.move-caret" || intent.Name == "vim.find" {
		at := int(intent.Range.From)
		if (key == "n" || key == "N") && e.vimController().View().LastSearch != "" {
			state := e.vimController().View()
			direction := state.LastSearchDirection
			if key == "N" {
				direction = -direction
			}
			if next, ok := e.modalSearch(state.LastSearch, direction, max(1, intent.Count), e.Caret()); ok {
				at = next
			}
		}
		e.nativeSetCaret(at, visual)
		if (key == "n" || key == "N") && e.vimController().View().LastSearch != "" {
			e.modalApplySearchHighlights(e.vimController().View().LastSearch, at, true)
		}
		return
	}
	if handled, _ := e.Session.ExecuteVimIntent(intent); handled {
		e.invalidate()
		return
	}
	if keymap.View().Searching {
		e.modalRefreshSearchPreview()
	}
	// Unsupported host effects are consumed by the active map, while keeping
	// MyGo's non-text state untouched.
}

func (e *Editor) dispatchVimWordIntent(key string, intent textengine.Intent, visual bool) bool {
	if e.Keymap() != "vim" {
		return false
	}
	if visual && e.vimController().View().Linewise && e.tableSelection != nil {
		return true
	}
	big := key == "W" || key == "B" || key == "E"
	direction := 0
	ending := false
	switch key {
	case "w", "W":
		direction = 1
	case "b", "B":
		direction = -1
	case "e", "E":
		direction, ending = 1, true
	default:
		return false
	}
	count := max(1, intent.Count)
	start := e.Caret()
	if intent.Name == "vim.move-caret" {
		at := start
		if ending {
			for iteration := range count {
				e.applyVimMotion(helix.Intent{Motion: helix.MotionWordEnd, BigWord: big}, iteration, visual)
			}
			return true
		}
		for range count {
			at = e.stepWithWordMode(at, direction, true, big)
		}
		e.nativeSetCaret(at, visual)
		return true
	}
	operator := ""
	switch intent.Name {
	case "vim.delete-range":
		operator = "d"
	case "vim.change-range":
		operator = "c"
	default:
		return false
	}
	// Operator motions share the editor's Unicode-aware word index with
	// selection/navigation, instead of using the engine's context-free token
	// boundaries. The keymap still owns command parsing and register state.
	to := start
	for range count {
		if ending {
			end := e.wordEndAtMode(to, big, false)
			if end <= to {
				end = e.nextWordEndAcrossUnitsMode(to, big)
			}
			if end <= to {
				break
			}
			to = end
		} else {
			to = e.stepWithWordMode(to, direction, true, big)
		}
	}
	if to <= start {
		return false
	}
	e.modalApply(operator, start, to)
	return true
}

func (e *Editor) vimYankFlashRange(register string, linewise bool) (int, int, bool) {
	if register == "" {
		return 0, 0, false
	}
	start, end := e.selection()
	if linewise {
		start, _ = e.modalLine(e.Caret())
		_, end = e.modalLine(e.Caret())
	}
	if start < 0 || end < start || end > len(e.Source()) {
		return 0, 0, false
	}
	if e.Source()[start:end] == register {
		return start, end, true
	}
	// Vim's visual selection is directional and half-open in the document
	// model, while the register payload is inclusive. Use its byte length to
	// recover the exact painted interval without changing the selection model.
	end = start + len(register)
	if end <= len(e.Source()) && e.Source()[start:end] == register {
		return start, end, true
	}
	return 0, 0, false
}

func (e *Editor) vimGraphemeDistance(from, to int) int {
	direction := 1
	if to < from {
		direction = -1
	}
	count := 0
	for at := from; at != to && count < 10000; count++ {
		next := e.step(at, direction, false)
		if next == at {
			break
		}
		at = next
	}
	return max(1, count)
}

// DispatchVimIntent implements the Vim host boundary. Keymap parsing remains
// in editor/input/vim; document and selection effects resolve here.
func (e *Editor) DispatchVimIntent(intent helix.Intent) {
	switch intent.Kind {
	case helix.IntentPrefix, helix.IntentConsumed:
		return
	case helix.IntentCommand:
		e.DispatchVimCommand(intent.Command)
		return
	case helix.IntentSearchText:
		e.modalRefreshSearchPreview()
		return
	case helix.IntentUndo:
		e.Undo()
		e.modalSetMode(helix.ModeNormal)
		e.vimController().SetLinewise(false)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		e.setSessionAnchor(e.Caret())
		return
	case helix.IntentPutRegister:
		e.putRegister(intent.Before, intent.Count)
		return
	case helix.IntentFindCharacter:
		if intent.Character == "" {
			e.vimController().SetAwaitingCharacter(intent.Direction, intent.Till, intent.Count)
		} else {
			e.findCharacter(intent.Character, intent.Direction, intent.Till, intent.Count)
		}
		return
	case helix.IntentReplaceCharacter:
		e.modalReplace(intent.Count, intent.Character)
		return
	case helix.IntentEnterInsert:
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
		e.modalSetMode(helix.ModeInsert)
		e.breakCoalescing()
		return
	case helix.IntentOpenLine:
		e.nativePrepareInsert()
		e.nativeOpenLine(intent.Before)
		e.modalSetMode(helix.ModeInsert)
		return
	case helix.IntentEnterVisual, helix.IntentEnterLineVisual:
		e.enterModalVisual(intent)
		return
	case helix.IntentBlockInsert:
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
		e.modalSetMode(helix.ModeInsert)
		e.setSessionAnchor(e.Caret())
		return
	case helix.IntentDeleteCharacter:
		if a, z, ok := e.vimVirtualImageLineRange(); ok {
			e.vimController().SetLinewise(true)
			e.modalApply("d", a, z)
		} else if a, z, ok := e.nativeSubstituteRange(intent.Count, intent.Before); ok {
			e.vimController().SetLinewise(false)
			e.modalApply("d", a, z)
		}
		return
	case helix.IntentDeleteLineRemainder:
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
		if intent.Motion != helix.MotionLeft && intent.Motion != helix.MotionRight && intent.Motion != helix.MotionDown && intent.Motion != helix.MotionUp {
			_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.ResetNavigation})
		}
		if intent.Motion != helix.MotionDown && intent.Motion != helix.MotionUp {
			e.hasGoal = false
		}
		start := e.Caret()
		mode := e.vimController().Mode()
		extend := intent.Kind == helix.IntentOperator || mode == helix.ModeVisual || mode == helix.ModeSelect || mode == helix.ModeBlockVisual
		for i := 0; i < max(1, intent.Count); i++ {
			e.applyVimMotion(intent, i, extend)
		}
		if extend {
			e.promoteVisualTextToCellRegion()
		}
		if intent.Kind == helix.IntentOperator {
			if intent.Linewise {
				e.vimController().SetLinewise(true)
			}
			e.modalApply(intent.Operator, start, e.Caret())
		} else if e.Keymap() == "helix" && !extend {
			e.setSessionAnchor(start)
		}
		return
	}
	if intent.Kind == helix.IntentOperator {
		e.applyModalOperator(intent)
		return
	}
}

func (e *Editor) modalSetMode(mode helix.Mode) {
	e.vimController().SetMode(mode)
}

func (e *Editor) enterModalVisual(intent helix.Intent) {
	e.vimController().SetTextVisualActive(e.Keymap() == "vim" && intent.Kind == helix.IntentEnterVisual)
	e.setSessionHead(e.selectionFocus())
	e.tableSelection, e.tableBridge = nil, nil
	e.setSessionAnchor(e.Caret())
	if intent.Kind == helix.IntentEnterLineVisual {
		e.modalSetMode(helix.ModeVisual)
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
		e.modalSetMode(helix.ModeSelect)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
	} else {
		e.modalSetMode(helix.ModeVisual)
		_, virtualImageRow := e.vimImageRowState()
		e.setSessionInclusive(!virtualImageRow)
		e.setSessionLinewise(virtualImageRow)
	}
}

func (e *Editor) applyVimMotion(intent helix.Intent, iteration int, extend bool) {
	switch intent.Motion {
	case helix.MotionLeft:
		e.modalHorizontal(-1, extend)
	case helix.MotionRight:
		e.modalHorizontal(1, extend)
	case helix.MotionDown:
		if e.moveTableCellRegion(1, 0) {
			return
		}
		if e.vimController().Sequence().Linewise {
			e.nativeMoveLineSelection(1, 1)
			return
		}
		if e.vimController().Mode() == helix.ModeBlockVisual && e.moveTableVisualColumn(1, 1) {
			return
		}
		if !e.moveStructuralRow(1, extend) {
			e.nativeVertical(1, extend)
		}
	case helix.MotionUp:
		if e.moveTableCellRegion(-1, 0) {
			return
		}
		if e.vimController().Sequence().Linewise {
			e.nativeMoveLineSelection(-1, 1)
			return
		}
		if e.vimController().Mode() == helix.ModeBlockVisual && e.moveTableVisualColumn(-1, 1) {
			return
		}
		if !e.moveStructuralRow(-1, extend) {
			e.nativeVertical(-1, extend)
		}
	case helix.MotionWordForward:
		e.nativeSetCaret(e.stepWithWordMode(e.Caret(), 1, true, intent.BigWord), extend)
	case helix.MotionWordBackward:
		e.nativeSetCaret(e.stepWithWordMode(e.Caret(), -1, true, intent.BigWord), extend)
	case helix.MotionWordEnd:
		z := e.wordEndAtMode(e.Caret(), intent.BigWord, false)
		if iteration > 0 {
			if next := e.nextWordEndAcrossUnitsMode(e.Caret(), intent.BigWord); next > e.Caret() {
				z = next
			}
		} else if z <= e.Caret() || intent.Kind != helix.IntentOperator && e.previousGraphemeBoundary(z) <= e.Caret() {
			if next := e.nextWordEndAcrossUnitsMode(e.Caret(), intent.BigWord); next > e.Caret() {
				z = next
			}
		}
		if intent.Kind != helix.IntentOperator && z > e.Caret() {
			z = e.previousGraphemeBoundary(z)
		}
		e.nativeSetCaret(z, extend)
	case helix.MotionLineStart:
		e.nativeSetCaret(e.nativeLineStart(e.Caret(), false), extend)
	case helix.MotionFirstNonBlank:
		e.nativeSetCaret(e.nativeLineStart(e.Caret(), true), extend)
	case helix.MotionLineEnd:
		_, z := e.modalLine(e.Caret())
		if intent.Kind != helix.IntentOperator {
			start, _ := e.modalLine(e.Caret())
			if z > start {
				z = e.previousGraphemeBoundary(z)
			}
		}
		e.nativeSetCaret(z, extend)
	case helix.MotionDocumentEnd:
		e.nativeSetCaret(len(e.Source()), extend)
	}
}

func (e *Editor) applyModalOperator(intent helix.Intent) {
	if intent.Linewise {
		if a, z, ok := e.vimVirtualImageLineRange(); ok {
			e.modalApply(intent.Operator, a, z)
			return
		}
		buffer, err := textengine.NewBuffer(e.Source())
		if err != nil {
			return
		}
		range_ := textvim.LinewiseOperatorRange(buffer.View(), textengine.Offset(e.Caret()), intent.Count, intent.Operator)
		e.modalApply(intent.Operator, int(range_.From), int(range_.To))
		return
	}
	if e.vimController().Mode() == helix.ModeBlockVisual && e.block != nil {
		ranges := e.SelectionRanges()
		var rows []string
		for _, r := range ranges {
			rows = append(rows, e.Source()[r.Start:r.End])
		}
		e.setVimBlockRegister(rows)
		if intent.Operator == "y" {
			e.Copy()
			e.startVimYankFlash(ranges)
			e.modalSetMode(helix.ModeNormal)
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
			e.modalSetMode(helix.ModeInsert)
		} else {
			e.modalSetMode(helix.ModeNormal)
		}
		return
	}
	if e.tableSelection != nil && e.tableBridge == nil && e.applyTableSelection(intent.Operator) {
		if intent.Operator != "c" {
			e.modalSetMode(helix.ModeNormal)
		}
		return
	}
	mode := e.vimController().Mode()
	if mode == helix.ModeVisual || mode == helix.ModeSelect || mode == helix.ModeBlockVisual {
		a, z := e.selection()
		if a == z {
			z = e.step(z, 1, false)
		}
		e.modalApply(intent.Operator, a, z)
		if intent.Operator != "c" {
			e.modalSetMode(helix.ModeNormal)
		}
	}
}
