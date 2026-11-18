package editor

import (
	"github.com/egoist/mygo/ui"
	tablemodel "muse/editor/blocks/table"
	"muse/editor/input"
	"muse/editor/input/vim"
	"github.com/wheregrassdreams/muse-editor/text/navigation"
	tableui "muse/nativeui/workbench/features/editor/blocks/table"
	"muse/nativeui/workbench/features/editor/inputadapter"
	"strings"
)

func (e *Editor) vimController() *vim.Controller {
	if e.vimState == nil {
		e.vimState = vim.New()
	}
	return e.vimState
}

// VimViewState returns a detached snapshot for UI labels and prompts.
func (e *Editor) VimViewState() vim.ViewState {
	return e.vimController().View()
}

func (e *Editor) DispatchVimCommand(command vim.Command) {
	if operation, ok := vim.OperationForCommand(command); ok {
		e.ApplyOperation(operation)
		return
	}
	visual := e.vimController().Mode() == vim.ModeVisual || e.vimController().Mode() == vim.ModeSelect || e.vimController().Mode() == vim.ModeBlockVisual
	switch command.Kind {
	case vim.CommandFirstLine:
		off := 0
		for line := 1; line < max(1, command.Count); line++ {
			_, end := e.modalLine(off)
			if end >= len(e.Source()) {
				break
			}
			off = end + 1
		}
		e.nativeSetCaret(off, visual)
	case vim.CommandVertical:
		for range max(1, command.Count) {
			e.nativeVertical(command.Direction, visual)
		}
	}
}

func (e *Editor) SetKeymap(s string) {
	e.feedbackState.CancelCursor()
	e.smoothCursorNavigation = false
	e.clearVimImageRow()
	e.clearVimSearchHighlights()
	e.feedbackState.CancelYankFlash()
	e.finishBlockInput()
	if s != "vim" && s != "helix" {
		s = "default"
	}
	controller := vim.New()
	controller.SetKeymap(s)
	e.vimState = controller
	e.modal = modalState{}
	e.block, e.blockInput = nil, nil
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	e.breakCoalescing()
	e.invalidate()
}
func (e *Editor) Keymap() string {
	return e.vimController().Keymap()
}
func (e *Editor) ModeLabel() string {
	return e.VimViewState().ModeLabel()
}

// ModalAcceptsText must guard all native composition/replacement entry points.
func (e *Editor) ModalAcceptsText() bool { return e.vimController().AcceptsText() }

type editorVimCommandLineHost struct{ editor *Editor }

func (h editorVimCommandLineHost) VimViewState() vim.ViewState {
	return h.editor.VimViewState()
}

func (h editorVimCommandLineHost) CommandLineInset() float32 { return h.editor.colX }

func (h editorVimCommandLineHost) SetSearchInputFocused(focused bool) {
	h.editor.modal.searchInputFocused = focused
}

func (h editorVimCommandLineHost) UpdateSearchQuery(query string) {
	h.editor.vimController().UpdateSearchQuery(query)
	h.editor.modalRefreshSearchPreview()
}

func (h editorVimCommandLineHost) SubmitSearch() {
	h.editor.ApplyOperation(input.Operation{Kind: input.OperationSubmitSearch})
}

func (h editorVimCommandLineHost) CancelSearch() {
	h.editor.ApplyOperation(input.Operation{Kind: input.OperationCancelSearch})
}

func (e *Editor) modalCommandLineView(c *ui.Context) {
	inputadapter.ModalCommandLineView(c, editorVimCommandLineHost{editor: e})
}
func (e *Editor) modalKeyEvent(event input.KeyEvent) bool {
	k, m := event.Key, event.Modifiers
	if e.Keymap() == "default" || m&input.ModCommand != 0 {
		return false
	}
	if e.vimController().View().Searching {
		switch k {
		case input.KeyEscape:
			e.ApplyOperation(input.Operation{Kind: input.OperationCancelSearch})
			return true
		case input.KeyEnter:
			e.ApplyOperation(input.Operation{Kind: input.OperationSubmitSearch})
			return true
		case input.KeyBackspace:
			e.modalSearchBackspace()
			return true
		default:
			if m&(input.ModAlt|input.ModCtrl) != 0 {
				return true
			}
		}
	}
	if k == input.KeyR && m == input.ModCtrl {
		e.Redo()
		e.modalSetMode(vim.ModeNormal)
		e.vimController().SetLinewise(false)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		e.setSessionAnchor(e.Caret())
		return true
	}
	if k == input.KeyV && m == input.ModCtrl {
		e.setSessionHead(e.selectionFocus())
		if t, row, col, ok := e.tableRegionAtCaret(); ok {
			e.tableBridge = nil
			_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.ResetNavigation})
			e.modalSetMode(vim.ModeBlockVisual)
			e.vimController().SetLinewise(false)
			e.vimController().ClearPending()
			e.vimController().SetCount(0)
			e.setSessionAnchor(e.Caret())
			e.block = nil
			_, _ = e.tableUI().Dispatch(tableui.Action{
				Kind: tableui.SelectCell, LineStart: t.lineStart,
				Anchor: tablemodel.Cell{Row: row, Col: col}, Focus: tablemodel.Cell{Row: row, Col: col},
			})
			e.setSessionInclusive(false)
			e.setSessionLinewise(false)
			e.invalidate()
			return true
		}
		e.tableBridge = nil
		_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.ResetNavigation})
		e.modalSetMode(vim.ModeBlockVisual)
		e.block = nil
		e.startBlockSelection()
		e.vimController().SetLinewise(false)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		e.vimController().ClearPending()
		e.vimController().SetCount(0)
		e.setSessionAnchor(e.Caret())
		e.tableSelection = nil
		e.invalidate()
		return true
	}
	if k == input.KeyEscape {
		wasInsert := e.Keymap() == "vim" && e.vimController().Mode() == vim.ModeInsert
		e.vimController().SetTextVisualActive(false)
		if e.vimHighlightsActive {
			e.clearVimSearchHighlights()
		}
		if e.editBatch != nil {
			e.commit(e.Caret(), e.Caret())
		}
		focus := e.selectionFocus()
		if wasInsert {
			inserted := e.previousGraphemeBoundary(focus)
			if focus > lineStartAt(e.Source(), focus) {
				focus = e.normalize(inserted, -1)
			}
		}
		e.setSessionHead(focus)
		if e.vimImageRow != nil && e.Caret() != e.vimImageRow.lineStart {
			e.clearVimImageRow()
		}
		_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.ResetNavigation})
		e.clearComposition()
		e.modalSetMode(vim.ModeNormal)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		e.vimController().ClearPending()
		e.vimController().ClearAwaitingCharacter()
		e.vimController().ClearAwaitingReplacement()
		e.vimController().ResetSequence()
		e.vimController().SetCount(0)
		e.vimController().SetOperatorCount(0)
		e.vimController().SetLinewise(false)
		e.clearTableSelectionSession()
		e.modal.blockInsert = nil
		e.block = nil
		e.blockInput = nil
		e.setSessionAnchor(e.Caret())
		e.breakCoalescing()
		return true
	}
	if e.blockInput != nil {
		switch k {
		case input.KeyBackspace:
			e.editBlockInputDelete(true)
			return true
		case input.KeyDelete:
			e.editBlockInputDelete(false)
			return true
		case input.KeyEnter:
			e.typeBlockInput("\n")
			return true
		case input.KeyTab:
			e.typeBlockInput("\t")
			return true
		}
		e.finishBlockInput()
	}
	if e.ModalAcceptsText() {
		return false
	}
	if m&(input.ModAlt|input.ModCtrl) != 0 {
		return true
	}
	switch k {
	case input.KeyBackspace, input.KeyDelete, input.KeyEnter, input.KeyTab:
		return true
	}
	// Let printable keys reach the text callback (preserves case and keyboard layout).
	switch k {
	case input.KeyLeft:
		return e.modalText("h")
	case input.KeyRight:
		return e.modalText("l")
	case input.KeyUp:
		return e.modalText("k")
	case input.KeyDown:
		return e.modalText("j")
	}
	return false
}
func (e *Editor) modalLine(at int) (int, int) {
	return e.sourceLineRange(at)
}

func (e *Editor) modalHorizontal(direction int, extend bool) {
	if e.moveTableCellRegion(0, direction) {
		return
	}
	if e.Keymap() == "vim" {
		e.ApplyOperation(input.Operation{
			Kind: input.OperationMoveHorizontal, Direction: direction,
			Extend: extend, WithinLine: true,
		})
		e.promoteVisualTextToCellRegion()
		return
	}
	e.nativeHorizontal(direction, extend)
	e.promoteVisualTextToCellRegion()
}

func (e *Editor) modalEmptyLineAt(off int) bool {
	u := e.unitAt(off)
	return u != nil && (u.kind == kBlank || u.kind == kPhantom)
}

func (e *Editor) modalApply(op string, a, z int) {
	if a > z {
		a, z = z, a
	}
	registerLinewise := e.vimController().Sequence().Linewise || e.Session.Selection().Linewise
	e.setVimTextRegister(e.Source()[a:z], registerLinewise)
	e.vimController().SetLinewise(false)
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	e.setSessionSelection(a, z)
	if op == "y" {
		e.Copy()
		e.startVimYankFlash([]Range{{Start: a, End: z}})
		e.setSessionSelection(a, a)
		return
	}
	e.begin(stepEdit)
	e.replace(a, z, "")
	e.commit(a, a)
	if op == "c" {
		e.modalSetMode(vim.ModeInsert)
	}
}

func (e *Editor) setVimTextRegister(text string, linewise bool) {
	e.modal.registerTable = nil
	e.vimController().SetTextRegister(text, linewise)
}

func (e *Editor) setVimBlockRegister(rows []string) {
	e.modal.registerTable = nil
	e.vimController().SetBlockRegister(rows)
}

func (e *Editor) setVimTableRegister(reg *tableRegister) {
	e.modal.registerTable = reg
	if reg != nil {
		e.vimController().SetTableRegister()
	} else {
		e.vimController().ClearRegister()
	}
}

func (e *Editor) putRegister(before bool, count int) bool {
	m := &e.modal
	register := e.vimController().View()
	if register.RegisterKind == vim.RegisterBlock && len(register.RegisterBlock) > 0 {
		e.pasteBlockRegister(register.RegisterBlock, before, count)
		return true
	}
	if m.registerTable != nil {
		for i := 0; i < count; i++ {
			if !e.pasteTableRegister(before) {
				return i > 0
			}
		}
		return true
	}
	if register.RegisterValid || register.Register != "" {
		at := e.Caret()
		value := strings.Repeat(register.Register, count)
		if register.RegisterLine {
			value = vim.LinewiseRegisterPayload(register.Register, count)
			if before {
				at, _ = e.modalLine(at)
				value += "\n"
			} else {
				_, at = e.modalLine(at)
				if at < len(e.Source()) {
					at++
					value += "\n"
				} else {
					value = "\n" + value
				}
			}
		} else {
			_, lineEnd := e.modalLine(at)
			if at < lineEnd {
				at = e.nextGraphemeBoundary(at)
			}
		}
		e.begin(stepEdit)
		e.replace(at, at, value)
		e.commit(at, at)
		return true
	}
	e.setSessionAnchor(e.Caret())
	e.Paste(false)
	return true
}

func (e *Editor) modalStartSearch(dir int) {
	e.clearVimSearchHighlights()
	e.modalSetMode(vim.ModeNormal)
	e.vimController().SetSearching(dir, "", e.Caret(), e.Session.Selection().Anchor, e.Session.Selection().Inclusive, e.Session.Selection().Linewise)
	e.vimController().SetMode(vim.ModeNormal)
	e.breakCoalescing()
	e.invalidate()
}

func (e *Editor) modalCancelSearch() {
	state := e.vimController().View()
	e.vimController().EndSearch()
	e.modalSetMode(vim.ModeNormal)
	e.setSessionSelection(state.SearchStartAnchor, state.SearchStartCaret)
	e.setSessionSelectionMode(state.SearchStartInclusive, state.SearchStartLinewise)
	e.clearVimSearchHighlights()
	e.breakCoalescing()
	e.Focus()
	e.invalidate()
}

func (e *Editor) modalSearchBackspace() {
	if text := e.compositionText(); text != "" {
		e.setComposition(vim.TrimLastGrapheme(text), ui.TextInputRange{Start: e.compositionSelection().Start, End: e.compositionSelection().End})
		e.invalidate()
		return
	}
	q := e.vimController().View().SearchQuery
	if q == "" {
		return
	}
	query := vim.TrimLastGrapheme(q)
	e.vimController().UpdateSearchQuery(query)
	e.modalRefreshSearchPreview()
	e.invalidate()
}

func (e *Editor) modalSubmitSearch() {
	state := e.vimController().View()
	query := state.SearchQuery
	if query != "" {
		e.vimController().SetLastSearch(query, state.SearchDirection)
	} else {
		query = state.LastSearch
		if query != "" {
			e.vimController().SetLastSearch(query, state.SearchDirection)
		}
	}
	matchAt, matchFound := -1, false
	if query != "" {
		if at, ok := e.modalSearch(query, state.SearchDirection, 1, state.SearchStartCaret); ok {
			matchAt, matchFound = at, true
			e.nativeSetCaret(at, false)
			e.setSessionSelectionMode(false, false)
			e.modalSetMode(vim.ModeNormal)
		} else {
			e.setSessionSelection(state.SearchStartAnchor, state.SearchStartCaret)
			e.setSessionSelectionMode(state.SearchStartInclusive, state.SearchStartLinewise)
		}
		e.modalApplySearchHighlights(query, matchAt, matchFound)
	}
	e.vimController().EndSearch()
	e.modalSetMode(vim.ModeNormal)
	e.Focus()
	e.invalidate()
}

func (e *Editor) modalRepeatSearch(reverse bool, count int) {
	searchState := e.vimController().View()
	dir := searchState.LastSearchDirection
	if reverse {
		dir = -dir
	}
	if searchState.LastSearch == "" || dir == 0 {
		return
	}
	if at, ok := e.modalSearch(searchState.LastSearch, dir, count, e.Caret()); ok {
		e.nativeSetCaret(at, false)
		e.setSessionSelectionMode(false, false)
		e.modalSetMode(vim.ModeNormal)
		e.vimController().SetMode(vim.ModeNormal)
		e.modalApplySearchHighlights(searchState.LastSearch, at, true)
	}
}

func (e *Editor) modalSearchRanges(query string) []Range {
	matches := e.modalTextSearch().SearchRanges(query)
	out := make([]Range, len(matches))
	for i, match := range matches {
		out[i] = Range{Start: match.Start, End: match.End}
	}
	return out
}

func (e *Editor) modalTextSearch() navigation.TextSearch {
	return navigation.NewTextSearchWithBoundaries(e.Source(), e.graphemeEdges())
}

func (e *Editor) modalApplySearchHighlights(query string, at int, found bool) {
	ranges := e.modalSearchRanges(query)
	current := -1
	if found {
		for i, r := range ranges {
			if r.Start == at {
				current = i
				break
			}
		}
	}
	e.setVimSearchHighlights(ranges, current)
}

func (e *Editor) modalRefreshSearchPreview() {
	state := e.vimController().View()
	if !state.Searching {
		return
	}
	query := state.SearchQuery
	at, found := e.modalSearch(query, state.SearchDirection, 1, state.SearchStartCaret)
	e.modalApplySearchHighlights(query, at, found)
}

func (e *Editor) modalSearch(query string, dir, count, from int) (int, bool) {
	return e.modalTextSearch().Search(query, from, navigation.Direction(dir), count)
}

func (e *Editor) modalFind(char string, dir int, till bool, count int) (int, bool) {
	direction := navigation.Backward
	if dir > 0 {
		direction = navigation.Forward
	}
	return e.modalTextSearch().Find(char, e.Caret(), direction, till, count)
}

func (e *Editor) modalReplace(count int, replacement string) {
	replacement = vim.FirstGrapheme(replacement)
	if replacement == "" {
		e.vimController().ClearPending()
		return
	}
	start := e.Caret()
	_, lineEnd := e.modalLine(start)
	if _, _, virtual := e.vimVirtualImageLineRange(); virtual {
		e.vimController().ClearPending()
		return
	}
	mode := e.vimController().Mode()
	visual := mode == vim.ModeVisual || mode == vim.ModeSelect
	if visual {
		start, _ = e.selection()
		_, lineEnd = e.modalLine(start)
	}
	end := start
	limit := max(1, count)
	if visual {
		_, end = e.selection()
		if e.Session.Selection().Inclusive && end < len(e.Source()) {
			end = e.nextGraphemeBoundary(end)
		}
		limit = 0
	}
	for i := 0; i < limit && end < lineEnd; i++ {
		next := e.nextGraphemeBoundary(end)
		if next <= end || next > lineEnd {
			break
		}
		// Keep table cell separators intact: replacing one would change the
		// document structure rather than replace a cell's text.
		if e.Source()[end:next] == "|" {
			break
		}
		end = next
	}
	if end == start {
		e.vimController().ClearPending()
		return
	}
	if strings.Contains(e.Source()[start:end], "\n") || strings.Contains(e.Source()[start:end], "|") || replacement == "\n" || replacement == "|" {
		e.vimController().ClearPending()
		return
	}
	count = 0
	for p := start; p < end; p = e.nextGraphemeBoundary(p) {
		count++
	}
	value := strings.Repeat(replacement, count)
	e.begin(stepEdit)
	e.replace(start, end, value)
	e.commit(start, start+len(value)-len(replacement))
	e.modalSetMode(vim.ModeNormal)
	e.vimController().SetLinewise(false)
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	e.setSessionAnchor(e.Caret())
	e.vimController().ClearPending()
}
