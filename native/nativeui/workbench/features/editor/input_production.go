package editor

import (
	"muse/editor/layout"
	"muse/locales"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	inlineblock "muse/editor/blocks/inline"
	tablemodel "muse/editor/blocks/table"
	editorinput "muse/editor/input"
	"muse/editor/input/vim"
	syntax "muse/markdown/syntax"
	media "muse/nativeui/workbench/features/editor/blocks/media"
	tableui "muse/nativeui/workbench/features/editor/blocks/table"
	"muse/nativeui/workbench/features/editor/inputadapter"
)

// tableCellDragState exists only for the lifetime of a pointer gesture.
type tableCellDragState struct {
	lineStart            int
	anchorRow, anchorCol int
	crossed              bool
}

// input takes the editor's input as it comes: keys, text typed, the edit
// commands of the menus, and the pointer.
func (e *Editor) input(ev ui.InputEvent) bool {
	defer func() {
		e.finishInputTransaction()
		e.invalidate()
	}()
	switch ev.Kind {
	case ui.InputKeyDown:
		key, ok := inputadapter.KeyEvent(ev)
		return ok && e.DispatchKey(key)
	case ui.InputText:
		event, ok := inputadapter.TextEvent(ev)
		if !ok {
			return false
		}
		result, err := e.Session.Dispatch(event)
		return err == nil && result.Handled
	case ui.InputCompose:
		if !e.ModalAcceptsText() {
			return true
		}
		n := ui.UTF16Len(ev.Text)
		c := min(ui.UTF16Len(string([]rune(ev.Text)[:min(ev.Caret, len([]rune(ev.Text)))])), n)
		e.setComposition(ev.Text, ui.TextInputRange{Start: c, End: c})
		return true
	case ui.InputCommand:
		if !e.ModalAcceptsText() {
			switch ev.Text {
			case "copy", "undo", "redo", "selectAll":
			default:
				return true
			}
		}
		return e.command(ev.Text)
	case ui.InputPointerDown:
		e.pointerX, e.pointerY = ev.X, ev.Y
		if ev.Button == 1 {
			e.layoutIfNeeded()
			e.contextDiagramSource = ""
			if hit, ok := e.imageAt(ev.X, ev.Y); ok && hit.obj != nil {
				e.contextImagePath = hit.obj.imagePath
			} else {
				e.contextImagePath = ""
				if leaf, _, ok := e.diagramBodyAt(ev.X, ev.Y); ok && isMermaid(leaf) {
					e.contextDiagramSource = e.codeText(leaf)
				}
			}
		}
		if ev.Button == 0 {
			if i, ok := e.wikiSuggestionIndexAt(ev.X, ev.Y); ok {
				e.acceptWikiSuggestion(i)
				return true
			}
		}
		// A visible block scrollbar is an overlay interaction, not a text
		// press. Route it before finishing block input or clearing its state so
		// dragging the thumb preserves selections, source mode and caret.
		if ev.Button == 0 {
			e.layoutIfNeeded()
			if l, ok := e.horizontalBarAt(ev.X, ev.Y); ok {
				e.pressBlockHorizontal(l, ev.X)
				return true
			}
		}
		e.finishBlockInput()
		e.block = nil
		handled := e.press(ev)
		e.updateWikiSuggestions()
		return handled
	case ui.InputPointerMove:
		return e.drag(ev)
	case ui.InputScroll:
		return e.scrollBlock(ev)
	case ui.InputPointerUp:
		if e.horizontalDrag != nil {
			e.horizontalDrag = nil
			return true
		}
		if e.imageDrag != nil {
			d := e.imageDrag
			e.imageDrag = nil
			e.resizeImageSource(d.hit, d.gesture.PreviewWidth())
			e.assetGen.Add(1)
			e.invalidate()
			return true
		}
		if e.diagPan != nil {
			e.diagPan = nil
			return true
		}
		if e.diagDrag != nil {
			e.diagDrag = nil
			return true
		}
		if e.dragging {
			e.dragging = false
			e.tableCellDrag = nil
			if e.frozen {
				e.frozen = false
				e.setSessionHead(e.normalize(e.Caret(), 1))
			}
			return true
		}
		return false
	}
	return false
}

// DispatchKey sends a backend-neutral key through the editor's keymap and
// requests a frame when the command changes its state. Native adapters should
// translate toolkit keys into editor/input.KeyEvent before calling it.
func (e *Editor) DispatchKey(event editorinput.KeyEvent) bool {
	defer func() {
		e.finishInputTransaction()
		e.invalidate()
	}()
	return e.Session.DispatchKey(event)
}

// finishInputTransaction closes a Session-owned edit batch at the boundary of
// one native input event. A command may apply several source edits internally,
// but an unfinished batch must never escape to the next event and retain the
// Session's transaction lock.
func (e *Editor) finishInputTransaction() {
	if e.editBatch != nil {
		selection := e.Session.Selection()
		e.commit(selection.Anchor, selection.Head)
	}
}

// command runs an edit command of the menus.
func (e *Editor) command(cmd string) bool {
	switch cmd {
	case "copy":
		e.Copy()
	case "cut":
		e.Cut()
	case "paste":
		e.Paste(false)
	case "pasteAndMatchStyle":
		e.Paste(true)
	case "selectAll":
		e.SelectAll()
	case "undo":
		e.Undo()
	case "redo":
		e.Redo()
	case "delete":
		e.deleteSelection()
	case "bold":
		e.ToggleBold()
	case "italic":
		e.ToggleItalic()
	case "underline":
		e.ToggleUnderline()
	case "strikethrough":
		e.ToggleStrike()
	default:
		return false
	}
	return true
}

// SelectAll selects the whole document.
func (e *Editor) SelectAll() {
	e.finishBlockInput()
	e.block = nil
	e.clearTableSelectionSession()
	e.tableHandleDrag = nil
	e.dropPhantom()
	e.setSessionSelection(0, len(e.Source()))
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	e.wantReveal = false
	e.invalidate()
}

// key handles a key pressed, and reports whether it took it.
func (e *Editor) key(event editorinput.KeyEvent) bool {
	key, mods := event.Key, event.Modifiers
	shift := mods&editorinput.ModShift != 0
	m := mods &^ editorinput.ModShift
	word := (isMac && m == editorinput.ModAlt) || (!isMac && m == editorinput.ModCtrl)
	if m == editorinput.ModCommand || m == editorinput.ModCommand|editorinput.ModAlt {
		if e.shortcut(key, shift, m&editorinput.ModAlt != 0) {
			return true
		}
	}
	switch key {
	case editorinput.KeyLeft, editorinput.KeyRight:
		dir := 1
		if key == editorinput.KeyLeft {
			dir = -1
		}
		if m == 0 && (shift || e.tableSelection != nil || e.tableBridge != nil) {
			e.nativeHorizontal(dir, shift)
			return true
		}
		switch {
		case isMac && m == editorinput.ModCommand:
			s, z := e.lineEdges(e.Caret())
			if dir < 0 {
				e.setCaret(s, shift)
			} else {
				e.setCaret(z, shift)
			}
		case word:
			e.setCaret(e.step(e.Caret(), dir, true), shift)
		case m != 0:
			return false
		case e.Session.Selection().Anchor != e.Caret() && !shift:
			a, z := e.selection()
			if dir < 0 {
				e.setCaret(a, false)
			} else {
				e.setCaret(z, false)
			}
		default:
			if e.Session.Selection().Anchor == e.Caret() {
				e.nativeHorizontal(dir, shift)
			} else {
				e.setCaret(e.step(e.Caret(), dir, false), shift)
			}
		}
		e.hasGoal = false
		e.upstream = false
		return true
	case editorinput.KeyUp, editorinput.KeyDown:
		dir := 1
		if key == editorinput.KeyUp {
			dir = -1
		}
		if m == 0 && e.moveStructuralRow(dir, shift) {
			return true
		}
		if isMac && m == editorinput.ModCommand {
			if dir < 0 {
				e.setCaret(e.firstStop(), shift)
			} else {
				e.setCaret(e.lastStop(), shift)
			}
			return true
		}
		if m != 0 {
			return false
		}
		e.nativeVertical(dir, shift)
		return true
	case editorinput.KeyHome, editorinput.KeyEnd:
		if m == editorinput.ModCtrl || m == editorinput.ModCommand {
			if key == editorinput.KeyHome {
				e.setCaret(e.firstStop(), shift)
			} else {
				e.setCaret(e.lastStop(), shift)
			}
			return true
		}
		s, z := e.lineEdges(e.Caret())
		if key == editorinput.KeyHome {
			e.setCaret(s, shift)
			e.upstream = false
		} else {
			e.setCaret(z, shift)
			e.upstream = true
		}
		e.hasGoal = false
		return true
	case editorinput.KeyPageUp, editorinput.KeyPageDown:
		e.page(key == editorinput.KeyPageDown, shift)
		return true
	case editorinput.KeyBackspace:
		if isMac && m == editorinput.ModCommand {
			s, _ := e.lineEdges(e.Caret())
			if s < e.Caret() {
				e.setSessionAnchor(s)
				e.deleteSelection()
			}
			return true
		}
		e.backspace(word)
		return true
	case editorinput.KeyDelete:
		e.deleteForward(word)
		return true
	case editorinput.KeyEnter:
		if m == editorinput.ModCommand {
			if t, r, c := e.cellPos(e.Caret()); t != nil {
				_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.InsertRowAfter, LineStart: t.lineStart, First: r, Focus: tablemodel.Cell{Col: c}})
				return true
			}
			return false
		}
		if m != 0 {
			return false
		}
		e.enter(shift)
		return true
	case editorinput.KeyTab:
		if m != 0 {
			return false
		}
		return e.tab(shift)
	case editorinput.KeyEscape:
		if e.imageDrag != nil {
			e.imageDrag = nil
			e.assetGen.Add(1)
			e.invalidate()
			return true
		}
		if _, _, _, ok := e.activeInlineMath(); ok && !e.inlineMath.PreviewHidden() {
			e.inlineMath.SetPreviewHidden(true)
			return true
		}
		if e.Session.Selection().Anchor != e.Caret() {
			e.setSessionAnchor(e.Caret())
			return true
		}
		return false
	}
	return false
}

// shortcut runs the editor's own shortcut of Cmd (Ctrl) with key.
func (e *Editor) shortcut(key editorinput.Key, shift, alt bool) bool {
	command := inputadapter.Shortcut(key, shift, alt, isMac)
	switch command {
	case inputadapter.ShortcutNone:
		return false
	case inputadapter.ShortcutToggleTask:
		e.ToggleList("- [ ]")
	case inputadapter.ShortcutInsertCodeBlock:
		e.InsertCodeBlock()
	case inputadapter.ShortcutInsertTable:
		e.InsertTable(3, 3)
	case inputadapter.ShortcutToggleStrike:
		e.ToggleStrike()
	case inputadapter.ShortcutUndo:
		e.Undo()
	case inputadapter.ShortcutRedo:
		e.Redo()
	case inputadapter.ShortcutSelectAll:
		e.SelectAll()
	case inputadapter.ShortcutCopy:
		e.Copy()
	case inputadapter.ShortcutCut:
		e.Cut()
	case inputadapter.ShortcutPaste:
		e.Paste(shift)
	case inputadapter.ShortcutToggleBold:
		e.ToggleBold()
	case inputadapter.ShortcutToggleItalic:
		e.ToggleItalic()
	case inputadapter.ShortcutToggleUnderline:
		e.ToggleUnderline()
	case inputadapter.ShortcutToggleCode:
		e.ToggleCode()
	case inputadapter.ShortcutToggleHighlight:
		e.ToggleHighlight()
	case inputadapter.ShortcutInsertLink:
		e.InsertLink()
	case inputadapter.ShortcutSelectWord:
		a, z := e.wordRangeAt(e.Caret())
		e.Select(a, z)
	case inputadapter.ShortcutSetHeading:
		e.SetHeading(int(key - editorinput.Key0))
	case inputadapter.ShortcutShiftHeadingUp:
		e.ShiftHeading(1)
	case inputadapter.ShortcutShiftHeadingDown:
		e.ShiftHeading(-1)
	case inputadapter.ShortcutToggleQuote:
		e.ToggleQuote()
	case inputadapter.ShortcutToggleOrderedList:
		e.ToggleList("1.")
	case inputadapter.ShortcutToggleUnorderedList:
		e.ToggleList("-")
	case inputadapter.ShortcutInsertMathBlock:
		e.InsertMathBlock()
	case inputadapter.ShortcutInsertTableRow:
		t, r, c := e.cellPos(e.Caret())
		if t == nil {
			return false
		}
		_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.InsertRowAfter, LineStart: t.lineStart, First: r, Focus: tablemodel.Cell{Col: c}})
	}
	return true
}

// page moves the caret a view's height up or down.
func (e *Editor) page(down bool, extend bool) {
	r, ok := e.caretRect(e.Caret())
	if !ok {
		return
	}
	if !e.hasGoal {
		e.goalX, e.hasGoal = r.X, true
	}
	h := max(e.viewH-e.lk.size*3, e.lk.size*3)
	y := r.Y + r.H/2 - h
	if down {
		y = r.Y + r.H/2 + h
	}
	off := e.hitTest(e.goalX, y)
	g := e.goalX
	e.setCaretAtPoint(off, y, extend)
	e.goalX, e.hasGoal = g, true
	if down {
		e.scroll.Y += h
	} else {
		e.scroll.Y = max(e.scroll.Y-h, 0)
	}
}

// press takes a press of the pointer: it puts the caret there, selects
// the word of a double click and the block of a triple one, and starts a
// drag selecting from there; it checks a task's box, opens a link
// Cmd-clicked and goes to a code block's language.
func (e *Editor) press(ev ui.InputEvent) bool {
	if ev.Button != 0 {
		return false
	}
	if e.Keymap() == "vim" {
		e.vimController().SetTextVisualActive(false)
	}
	e.layoutIfNeeded()
	x, y := ev.X, ev.Y
	e.pointerX, e.pointerY = x, y
	e.tableCellDrag = nil
	e.tableDocumentRegion = nil
	if h := e.foldChevronAt(x, y); h != nil {
		e.toggleFold(h)
		return true
	}
	if hit, ok := e.imageAt(x, y); ok {
		action := media.ImageToolAt(hit.box, e.lk.size, x, y, hit.obj != nil && hit.obj.block)
		switch action {
		case media.ImageToolPreview:
			if e.opts.OnImagePreview != nil {
				e.opts.OnImagePreview(hit.obj.imagePath)
			}
			return true
		case media.ImageToolSource:
			if hit.leaf != nil {
				e.toggleSource(hit.leaf)
			}
			return true
		case media.ImageToolAlign:
			e.toggleImageAlignment(hit)
			return true
		case media.ImageToolResize:
			e.imageDrag = &imageResizeDrag{hit: hit, gesture: media.NewImageResizeGesture(x, y, hit.obj.w)}
			return true
		default:
			if hit.obj != nil && hit.obj.bmp == nil && e.opts.OnImagePreview != nil {
				e.opts.OnImagePreview(hit.obj.imagePath)
				return true
			}
		}
	}
	if l, v, box, contentW, ok := e.diagramBarAt(x, y); ok {
		e.pressDiagramBar(l, v, box, contentW, x, y)
		return true
	}
	if it, ok := e.taskBoxAt(x, y); ok {
		e.toggleTaskAt(it)
		return true
	}
	if l := e.diagramToggleAt(x, y); l != nil {
		e.toggleDiagramSource(l)
		return true
	}
	if l := e.sourceChipAt(x, y); l != nil {
		// The control at a block's corner: show it as the Markdown it is
		// written as, or draw it again.
		e.toggleSource(l)
		return true
	}
	if l := e.codeLabelAt(x, y); l != nil {
		// The label at a code block's corner is its source control, as a
		// table's and a picture's tag is: it shows the fence and the lines,
		// and the caret stays where it was. The fence's own line is text
		// once it shows, and clicking it edits the language.
		e.toggleSource(l)
		return true
	}
	// A diagram is an object of its own: dragging it pans it, and a press
	// without a drag leaves the source alone. Its corner control is the way
	// in to edit it.
	if l, _, ok := e.diagramBodyAt(x, y); ok {
		e.diagPan = &diagPan{leaf: l, start: x, from: l.view.diagX}
		e.cursor = ui.CursorGrabbing
		e.invalidate()
		return true
	}
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	off := e.hitTest(x, y)
	if ev.Clicks <= 1 && ev.Mods&(ui.Shift|ui.Cmd|ui.Ctrl|ui.Alt) == 0 {
		if target, ok := e.footnoteTargetAt(off); ok {
			e.NavigateToOffset(target, layout.PageJumpNearest)
			return true
		}
	}
	if target, bounds := e.wikiLinkHoverAt(x, y, off); target != "" || e.opts.OnWikiLinkHover != nil && e.wikiHover != "" {
		e.setWikiHover(target, x, y, bounds)
	}
	link := e.linkAt(off)
	if strings.HasPrefix(link, "wiki:") && ev.Mods&ui.Shift == 0 {
		if r, ok := e.linkRangeAt(off); ok && e.pointInLink(x, y, r) {
			if e.opts.OnLink != nil {
				e.opts.OnLink(link)
			}
			return true
		}
	}
	if ev.Mods&ui.Cmd != 0 {
		if url := link; url != "" {
			if e.opts.OnLink != nil {
				e.opts.OnLink(url)
			}
			return true
		}
	}
	if e.Keymap() == "vim" && e.vimController().Mode() != vim.ModeInsert && ev.Mods&ui.Shift == 0 {
		// A fresh pointer press leaves an old visual selection and starts from
		// Vim Normal. A drag will enter Visual below, using the same inclusive
		// anchor/head model as the v command.
		e.modalSetMode(vim.ModeNormal)
		if ev.Clicks <= 1 {
			if _, _, object := e.objectAt(off); !object {
				off = e.vimPointerOffset(off, x, y)
			}
		}
	}
	// Below the last leaf, a paragraph comes after a block that is not
	// text.
	if len(e.leaves) > 0 && ev.Clicks <= 1 && ev.Mods&ui.Shift == 0 {
		last := e.leaves[len(e.leaves)-1]
		if y > last.y+last.h+e.lk.size*0.5 && e.phantomAtEdge(len(e.leaves)-1, 1) {
			return true
		}
	}
	// A comment's lines are blank in the parse, and the comment shows once
	// the caret is in it: a click there puts the caret in the comment
	// itself, not on a blank line beside it.
	if ev.Clicks <= 1 && ev.Mods&ui.Shift == 0 && len(e.p.Comments()) > 0 {
		if at, ok := e.gapPhantomAt(y); ok {
			if _, in := commentAt(e.p.Comments(), at); in {
				e.setCaret(at, false)
				return true
			}
		}
	}
	// A gap no line of the document covers takes the caret as a phantom
	// paragraph, so that the line can be typed in.
	if ev.Clicks <= 1 && ev.Mods&ui.Shift == 0 && !e.onALine(y) {
		if at, ok := e.gapPhantomAt(y); ok {
			e.setPhantom(&phantom{at: at})
			return true
		}
	}
	e.dragging = true
	e.hasGoal = false
	if row, col, table := e.tableCellAtPoint(x, y); table != nil && ev.Clicks <= 1 {
		e.tableCellDrag = &tableCellDragState{lineStart: table.lineStart, anchorRow: row, anchorCol: col}
	}
	if ev.Mods&ui.Shift == 0 {
		e.clearTableSelection()
	}
	switch {
	case ev.Clicks >= 3:
		a, z := e.blockRangeAt(off)
		e.dropPhantom()
		e.setSessionSelection(a, z)
		e.dragUnit, e.dragFrom = 2, [2]int{a, z}
	case ev.Clicks == 2:
		a, z := e.wordRangeAt(off)
		e.dropPhantom()
		e.setSessionSelection(a, z)
		e.dragUnit, e.dragFrom = 1, [2]int{a, z}
	default:
		extend := ev.Mods&ui.Shift != 0
		if a, z, ok := e.objectAt(off); ok && !extend {
			if e.isInlineMathRange(a, z) {
				e.setSessionSelection(a+1, a+1)
				break
			}
			// An object is one unit: a click takes it whole, and the
			// selection is what opens its source (docs/editor-model.md).
			// Typing or Backspace then replace it, as they would a letter.
			e.dropPhantom()
			e.setSessionSelection(a, z)
			e.dragUnit, e.dragFrom = 0, [2]int{a, z}
			break
		}
		e.setCaretAtPoint(off, y, extend)
		e.dragUnit, e.dragFrom = 0, [2]int{e.Session.Selection().Anchor, e.Session.Selection().Anchor}
	}
	e.frozen, e.frozenAt = true, e.Caret()
	e.blinkStart = time.Now()
	return true
}

func (e *Editor) footnoteTargetAt(off int) (int, bool) {
	u := e.unitAt(off)
	if u == nil || u.id < 0 || e.p.Parser() == nil {
		return 0, false
	}
	for _, in := range e.p.Parser().Inlines(u.id) {
		if in.Kind != syntax.IncrementalFootnoteRef || off < in.Start || off > in.End {
			continue
		}
		block, ok := e.p.Parser().Footnote(in.Text)
		if !ok {
			return 0, false
		}
		return e.p.Parser().Block(block).Start, true
	}
	return 0, false
}

// drag extends the selection to the pointer, by the unit the press chose,
// or drags a diagram's scroll bar.
func (e *Editor) drag(ev ui.InputEvent) bool {
	e.pointerX, e.pointerY = ev.X, ev.Y
	if e.horizontalDrag != nil {
		e.dragBlockHorizontal(ev.X)
		return true
	}
	if e.imageDrag != nil {
		if ev.Button != 0 {
			return false
		}
		e.imageDrag.gesture.Drag(ev.X, ev.Y)
		e.assetGen.Add(1)
		e.invalidate()
		return true
	}
	if e.diagPan != nil {
		if ev.Button != 0 {
			e.diagPan = nil
			return false
		}
		e.dragDiagramPan(ev.X)
		return true
	}
	if e.diagDrag != nil {
		if ev.Button != 0 {
			return false
		}
		e.dragDiagramBar(ev.X)
		return true
	}
	if !e.dragging || ev.Button != 0 {
		e.hover(ev.X, ev.Y)
		return false
	}
	e.dragTo(ev.X, ev.Y)
	return true
}

// dragTo extends the selection of a drag to the pointer at x, y in the
// element.
func (e *Editor) dragTo(x, y float32) {
	off := e.hitTest(x, y)
	if capture := e.tableCellDrag; capture != nil {
		if t := e.tableByLineStart(capture.lineStart); t != nil {
			row, col, hit := e.tableCellAtPoint(x, y)
			inside := hit == t
			if inside {
				if row != capture.anchorRow || col != capture.anchorCol {
					capture.crossed = true
				}
				if capture.crossed {
					mode := e.vimController().Mode()
					if e.Keymap() == "vim" && (mode == vim.ModeNormal || mode == vim.ModeVisual) {
						e.modalSetMode(vim.ModeVisual)
						e.setSessionInclusive(false)
						e.setSessionLinewise(false)
					}
					_, _ = e.tableUI().Dispatch(tableui.Action{
						Kind: tableui.SelectCell, LineStart: t.lineStart,
						Anchor: tablemodel.Cell{Row: capture.anchorRow, Col: capture.anchorCol},
						Focus:  tablemodel.Cell{Row: row, Col: col},
					})
					return
				}
			} else if capture.crossed {
				if s := e.tableSelection; s != nil && s.cellRegion {
					v := *s
					e.tableDocumentRegion = &v
				}
				e.tableCellDrag = nil
			}
		}
	}
	mode := e.vimController().Mode()
	if e.Keymap() == "vim" && (mode == vim.ModeNormal || mode == vim.ModeVisual) && e.dragUnit == 0 {
		off = e.vimPointerOffset(off, x, y)
		e.modalSetMode(vim.ModeVisual)
		e.setSessionInclusive(true)
		e.setSessionLinewise(false)
	}
	// Pointer motion owns its own anchor/head, rather than retaining a
	// structural focus created by an earlier keyboard selection.
	e.clearTableSelection()
	a, z := off, off
	switch e.dragUnit {
	case 1:
		a, z = e.wordRangeAt(off)
	case 2:
		a, z = e.blockRangeAt(off)
	}
	if a < e.dragFrom[0] {
		e.setSessionSelection(e.dragFrom[1], a)
	} else {
		e.setSessionSelection(e.dragFrom[0], max(z, e.dragFrom[1]))
		if e.dragUnit == 0 {
			e.setSessionHead(off)
		}
	}
	e.structuralSelectionForRange()
}

func (e *Editor) tableCellAtPoint(x, y float32) (row, col int, table *leaf) {
	for _, frameTable := range e.tableUI().Frame().Tables {
		if cell, ok := tableui.CellAtPoint(frameTable, x, y); ok {
			if table := e.tableByLineStart(frameTable.LineStart); table != nil {
				return cell.Row, cell.Col, table
			}
		}
	}
	return 0, 0, nil
}

func (e *Editor) tableByLineStart(lineStart int) *leaf {
	for _, candidate := range e.leaves {
		if candidate.kind == kTable && candidate.tab != nil && candidate.lineStart == lineStart {
			return candidate
		}
	}
	return nil
}

// autoScroll scrolls the view while a drag holds the pointer above or below
// it, faster the farther, and extends the selection with it. It reports
// whether it scrolled, for another frame to scroll on.
func (e *Editor) autoScroll() bool {
	if !e.dragging || e.viewH <= 0 {
		return false
	}
	top, bottom := e.scroll.Y, e.scroll.Y+e.viewH
	var d float32
	switch {
	case e.pointerY < top:
		d = max((e.pointerY-top)/3, -40)
	case e.pointerY > bottom:
		d = min((e.pointerY-bottom)/3, 40)
	default:
		return false
	}
	y := max(min(e.scroll.Y+d, e.docH-e.viewH), 0)
	if y == e.scroll.Y {
		return false
	}
	e.pointerY += y - e.scroll.Y
	e.scroll.Y = y
	e.dragTo(e.pointerX, e.pointerY)
	return true
}

// hover sets the pointer's shape for where it rests: a hand over what a
// click acts on (a check box, a code block's language, a link, a
// diagram's source control), the grab shape over a diagram, the default
// over its scroll bar, an I-beam over the text.
func (e *Editor) hover(x, y float32) {
	upstream := e.upstream
	defer func() { e.upstream = upstream }()
	e.pointerX, e.pointerY = x, y
	target, bounds := e.wikiLinkHoverAt(x, y, e.hitTest(x, y))
	e.setWikiHover(target, x, y, bounds)
	foldHover := -1
	if h := e.foldHeadingAt(x, y); h != nil {
		foldHover = h.lineStart
	}
	if foldHover != e.foldHover {
		e.foldHover = foldHover
		e.invalidate()
	}
	if e.foldChevronAt(x, y) != nil {
		if e.cursor != ui.CursorPointer {
			e.cursor = ui.CursorPointer
			e.invalidate()
		}
		return
	}
	if _, ok := e.horizontalBarAt(x, y); ok {
		if e.cursor != ui.CursorDefault {
			e.cursor = ui.CursorDefault
			e.invalidate()
		}
		return
	}
	hoverLink := Range{}
	off := e.hitTest(x, y)
	if r, ok := e.linkRangeAt(off); ok && e.pointInLink(x, y, r) {
		hoverLink = r
	}
	if hoverLink != e.hoverLink {
		e.hoverLink = hoverLink
		e.leavesLaid = false
		e.invalidate()
	}
	if hit, ok := e.imageAt(x, y); ok {
		action := media.ImageToolAt(hit.box, e.lk.size, x, y, hit.obj != nil && hit.obj.block)
		if action == media.ImageToolPreview || action == media.ImageToolSource || action == media.ImageToolAlign || hit.obj != nil && hit.obj.bmp == nil && e.opts.OnImagePreview != nil {
			e.cursor = ui.CursorPointer
		} else if action == media.ImageToolResize {
			e.cursor = ui.CursorResizeNWSE
		} else {
			e.cursor = ui.CursorText
		}
		e.invalidate()
		return
	}
	// The pointer on the source control of the block it is on keeps that
	// block: the control rides above the block, where the pointer is on
	// nothing else.
	if l := e.sourceChipAt(x, y); l != nil {
		if e.hoverBlock != l.lineStart {
			e.hoverBlock = l.lineStart
			e.invalidate()
		}
		if e.cursor != ui.CursorPointer {
			e.cursor = ui.CursorPointer
			e.invalidate()
		}
		return
	}
	block := -1
	if l := e.leafAtPoint(y); l != nil && e.gridBlock(l) {
		block = l.lineStart
	}
	if block != e.hoverBlock {
		e.hoverBlock = block
		e.invalidate()
	}
	cursor := ui.CursorText
	if _, _, _, _, ok := e.diagramBarAt(x, y); ok {
		cursor = ui.CursorDefault
	} else if _, ok := e.taskBoxAt(x, y); ok || e.diagramToggleAt(x, y) != nil || e.codeLabelAt(x, y) != nil {
		cursor = ui.CursorPointer
	} else if _, _, ok := e.diagramBodyAt(x, y); ok {
		cursor = ui.CursorGrab
	} else if e.linkAt(e.hitTest(x, y)) != "" {
		cursor = ui.CursorPointer
	}
	if cursor != e.cursor {
		e.cursor = cursor
		e.invalidate()
	}
}

// diagramBarAt returns the diagram leaf whose scroll bar is at x, y, when
// the diagram is wider than its column.
func (e *Editor) diagramBarAt(x, y float32) (*leaf, *leafView, ui.Rect, float32, bool) {
	for _, l := range e.leaves {
		v := l.view
		if v == nil || v.diagramModel == nil || !v.diagBarShown {
			continue
		}
		box, contentW := e.diagramBox(l, v, 0, 0)
		if contentW <= box.W {
			continue
		}
		if track, _ := diagramBarRects(box, contentW, v.diagX); track.Contains(x, y) {
			return l, v, box, contentW, true
		}
	}
	return nil, nil, ui.Rect{}, 0, false
}

// diagramAt returns the diagram leaf whose box is at x, y, when the
// diagram is wider than its column.
func (e *Editor) diagramAt(x, y float32) (*leaf, *leafView, ui.Rect, float32, bool) {
	for _, l := range e.leaves {
		v := l.view
		if v == nil || v.diagramModel == nil {
			continue
		}
		box, contentW := e.diagramBox(l, v, 0, 0)
		if contentW > box.W && box.Contains(x, y) {
			return l, v, box, contentW, true
		}
	}
	return nil, nil, ui.Rect{}, 0, false
}

// diagramBodyAt returns the diagram leaf whose drawn body is at x, y,
// whether it overflows its column or not: a press there pans it.
func (e *Editor) diagramBodyAt(x, y float32) (*leaf, *leafView, bool) {
	for _, l := range e.leaves {
		v := l.view
		if v == nil || v.diagramModel == nil {
			continue
		}
		if box, _ := e.diagramBox(l, v, 0, 0); box.Contains(x, y) {
			return l, v, true
		}
	}
	return nil, nil, false
}

// diagramToggleAt returns the Mermaid block whose corner control is at x,
// y: the one that shows its source, or its diagram again.
func (e *Editor) diagramToggleAt(x, y float32) *leaf {
	for _, l := range e.leaves {
		if !isMermaid(l) || l.view == nil || l.view.reveal {
			continue
		}
		if y >= l.y && y <= l.y+e.lk.size*1.4 && x >= l.x+l.w-e.lk.size*6 && x <= l.x+l.w {
			return l
		}
	}
	return nil
}

// toggleDiagramSource shows a Mermaid block's source, or brings its diagram
// back. It is the same control a table and a picture have, over the same
// state: the block shows its source, the caret stays where it was.
func (e *Editor) toggleDiagramSource(l *leaf) {
	e.toggleSource(l)
}

// dragDiagramPan pans the diagram whose body is being dragged to x.
func (e *Editor) dragDiagramPan(x float32) {
	d := e.diagPan
	if d == nil || d.leaf.view == nil || d.leaf.view.diagramModel == nil {
		return
	}
	v := d.leaf.view
	box, contentW := e.diagramBox(d.leaf, v, 0, 0)
	maxOff := max(contentW-box.W, 0)
	v.diagX = media.DiagramPanOffset(d.from, d.start, x, maxOff)
	e.invalidate()
}

// pressDiagramBar starts dragging a diagram's scroll bar, putting the
// thumb under the pointer first when the press missed it.
func (e *Editor) pressDiagramBar(l *leaf, v *leafView, box ui.Rect, contentW float32, x, y float32) {
	track, thumb := diagramBarRects(box, contentW, v.diagX)
	v.diagX = media.DiagramBarPressOffset(v.diagX, x, track.X, track.W, thumb.X, thumb.W, contentW-box.W, thumb.Contains(x, y))
	e.diagDrag = &diagDrag{leaf: l, start: x, from: v.diagX}
	e.invalidate()
}

// dragDiagramBar scrolls the diagram whose bar is being dragged to x.
func (e *Editor) dragDiagramBar(x float32) {
	d := e.diagDrag
	if d == nil || d.leaf.view == nil || d.leaf.view.diagramModel == nil {
		return
	}
	v := d.leaf.view
	box, contentW := e.diagramBox(d.leaf, v, 0, 0)
	maxOff := contentW - box.W
	if maxOff <= 0 {
		return
	}
	track, thumb := diagramBarRects(box, contentW, v.diagX)
	v.diagX = media.DiagramBarDragOffset(d.from, d.start, x, maxOff, track.W, thumb.W)
	e.invalidate()
}

// scrollDiagram scrolls a diagram sideways when the wheel, a swipe or
// Shift+wheel is over one wider than its column. It reports whether it
// took the scroll; one it leaves scrolls the document.
func (e *Editor) scrollDiagram(ev ui.InputEvent) bool {
	_, v, box, contentW, ok := e.diagramAt(ev.X, ev.Y)
	if !ok {
		return false
	}
	dx := ev.DX
	if dx == 0 && ev.Mods&ui.Shift != 0 {
		dx = ev.DY
	}
	if dx == 0 {
		return false
	}
	v.diagX = media.DiagramScrollOffset(v.diagX, dx, contentW-box.W)
	e.invalidate()
	return true
}

// taskBoxAt returns the item whose check box is at x, y.
func (e *Editor) taskBoxAt(x, y float32) (cont, bool) {
	for _, l := range e.leaves {
		if y < l.y-4 || y > l.y+e.lk.size*e.lk.lh+4 {
			continue
		}
		for _, k := range l.firstOf {
			c := l.conts[k]
			if c.kind != cItem || c.task == syntax.IncrementalNoTask {
				continue
			}
			if e.itemPrefixOpen(l) {
				continue
			}
			x0 := e.colX + e.indentOf(l.conts[:k])
			w := e.indentOf(l.conts[k : k+1])
			if x >= x0 && x <= x0+w {
				return c, true
			}
		}
	}
	return cont{}, false
}

// codeLabelAt returns the code block whose language label is at x, y.
func (e *Editor) codeLabelAt(x, y float32) *leaf {
	for _, l := range e.leaves {
		if l.kind != kCode || !l.fenced || l.view == nil || l.view.reveal {
			continue
		}
		if y >= l.y && y <= l.y+e.lk.size*1.4 && x >= l.x+l.w-e.lk.size*6 && x <= l.x+l.w {
			if l.language() != "" || e.leafAt(e.Caret()) == l {
				return l
			}
		}
	}
	return nil
}

// linkAt returns what a click at off opens: a link's destination, or a
// `[[wiki link]]`'s target as `wiki:target` (the app resolves it, as the
// editor knows nothing of the workspace).
func (e *Editor) linkAt(off int) string {
	u := e.unitAt(off)
	if u == nil || u.id < 0 || !(u.kind == kPara || u.kind == kHeading) {
		return ""
	}
	target, _ := inlineblock.LinkTargetAt(e.Source(), e.inlineLinkCandidates(u), off)
	return target
}

func (e *Editor) linkRangeAt(off int) (Range, bool) {
	u := e.unitAt(off)
	if u == nil || u.id < 0 || !(u.kind == kPara || u.kind == kHeading) {
		return Range{}, false
	}
	start, end, ok := inlineblock.LinkRangeAt(e.Source(), e.inlineLinkCandidates(u), off)
	return Range{Start: start, End: end}, ok
}

func (e *Editor) inlineLinkCandidates(u *leaf) []inlineblock.LinkCandidate {
	if u == nil || u.id < 0 || e.p.Parser() == nil {
		return nil
	}
	inlines := e.p.Parser().Inlines(u.id)
	candidates := make([]inlineblock.LinkCandidate, 0, len(inlines))
	for _, in := range inlines {
		var kind inlineblock.LinkCandidateKind
		switch in.Kind {
		case syntax.IncrementalText:
			kind = inlineblock.LinkCandidateText
		case syntax.IncrementalLink:
			kind = inlineblock.LinkCandidateLink
		case syntax.IncrementalImage:
			kind = inlineblock.LinkCandidateImage
		default:
			continue
		}
		candidates = append(candidates, inlineblock.LinkCandidate{
			Kind: kind, Start: in.Start, End: in.End, Destination: in.Dest,
		})
	}
	return candidates
}

func (e *Editor) pointInLink(x, y float32, link Range) bool {
	_, ok := e.linkRectAt(x, y, link)
	return ok
}

// linkRectAt returns the display rectangle of the visible link fragment under
// the pointer, in editor-local coordinates. Hover previews use it as an anchor
// so their panel can avoid covering the link that opened them.
func (e *Editor) linkRectAt(x, y float32, link Range) (ui.Rect, bool) {
	l := e.leafAtPoint(y)
	if l == nil || l.view == nil || l.view.layout == nil || link.Start >= l.end || link.End <= l.lineStart {
		return ui.Rect{}, false
	}
	v := l.view
	local := ui.Point{X: x - l.x - v.tx, Y: y - l.y - v.ty}
	for _, p := range v.disp.pieces {
		start, end := p.s0+l.lineStart, p.s1+l.lineStart
		isIcon := p.kind == pDeco && start == link.End && strings.Contains(p.text, "↗")
		if !isIcon && (p.kind == pDeco || end <= link.Start || start >= link.End) {
			continue
		}
		for _, r := range v.layout.SelectionRects(ui.TextInputRange{Start: p.d0, End: p.d1}) {
			if r.Contains(local.X, local.Y) {
				r.X += l.x + v.tx
				r.Y += l.y + v.ty
				return r, true
			}
		}
	}
	return ui.Rect{}, false
}

// menu builds the editor's context menu.
func (e *Editor) menu(m *ui.Menu) {
	// Context menus are opened at the pointer position; keep the hit image across
	// menu construction and item selection, which may move the pointer to a row.
	e.layoutIfNeeded()
	if imagePath := e.contextImagePath; imagePath != "" {
		if m.Item(locales.CommonCopyAction()).Chosen() {
			e.copyImageObject(imagePath)
		}
		if m.Item(locales.ImagePreviewTitle()).Chosen() && e.opts.OnImagePreview != nil {
			e.opts.OnImagePreview(imagePath)
		}
		return
	}
	if source := e.contextDiagramSource; source != "" && e.opts.OnDiagramPreview != nil {
		if m.Item(locales.EditorDiagramPreviewAction()).Chosen() {
			e.previewDiagram(source)
		}
		m.Separator()
	}
	a, z := e.selection()
	if m.Item(locales.CommonCutAction()).Shortcut(ui.Cmd, ui.KeyX).Disabled(a == z).Chosen() {
		e.Cut()
	}
	if m.Item(locales.CommonCopyAction()).Shortcut(ui.Cmd, ui.KeyC).Disabled(a == z).Chosen() {
		e.Copy()
	}
	if m.Item(locales.CommonPasteAction()).Shortcut(ui.Cmd, ui.KeyV).Chosen() {
		e.Paste(false)
	}
	if m.Item(locales.CommonPastePlainText()).Shortcut(ui.Cmd|ui.Shift, ui.KeyV).Chosen() {
		e.Paste(true)
	}
	m.Separator()
	if url := e.linkAt(e.Caret()); url != "" {
		if m.Item(locales.CommonOpenLinkAction()).Chosen() && e.opts.OnLink != nil {
			e.opts.OnLink(url)
		}
		if m.Item(locales.CommonCopyLinkAddress()).Chosen() {
			if e.opts.WriteClipboard != nil {
				e.opts.WriteClipboard(textData(url))
			} else {
				e.services.WriteClipboard(url)
			}
		}
		m.Separator()
	}
	if t, row, col := e.cellPos(e.Caret()); t != nil {
		m.Submenu(locales.EditorTableMenu(), func(m *ui.Menu) {
			for _, it := range []struct {
				label  string
				action tableui.Action
				sep    bool
			}{
				{label: locales.EditorInsertRowAbove(), action: tableui.Action{Kind: tableui.InsertRowAfter, LineStart: t.lineStart, First: max(row-1, 0), Focus: tablemodel.Cell{Col: col}}},
				{label: locales.EditorInsertRowBelow(), action: tableui.Action{Kind: tableui.InsertRowAfter, LineStart: t.lineStart, First: row, Focus: tablemodel.Cell{Col: col}}},
				{label: locales.EditorDeleteRow(), action: tableui.Action{Kind: tableui.DeleteRows, LineStart: t.lineStart, First: row, Last: row}, sep: false},
				{sep: true},
				{label: locales.EditorInsertColumnLeft(), action: tableui.Action{Kind: tableui.InsertColumnBefore, LineStart: t.lineStart, First: col}},
				{label: locales.EditorInsertColumnRight(), action: tableui.Action{Kind: tableui.InsertColumnBefore, LineStart: t.lineStart, First: col + 1}},
				{label: locales.EditorDeleteColumn(), action: tableui.Action{Kind: tableui.DeleteColumns, LineStart: t.lineStart, First: col, Last: col}},
				{sep: true},
				{label: locales.EditorAlignLeft(), action: tableui.Action{Kind: tableui.AlignColumn, LineStart: t.lineStart, Focus: tablemodel.Cell{Row: row, Col: col}, Alignment: tablemodel.AlignmentLeft}},
				{label: locales.EditorAlignCenter(), action: tableui.Action{Kind: tableui.AlignColumn, LineStart: t.lineStart, Focus: tablemodel.Cell{Row: row, Col: col}, Alignment: tablemodel.AlignmentCenter}},
				{label: locales.EditorAlignRight(), action: tableui.Action{Kind: tableui.AlignColumn, LineStart: t.lineStart, Focus: tablemodel.Cell{Row: row, Col: col}, Alignment: tablemodel.AlignmentRight}},
				{sep: true},
				{label: locales.EditorDeleteTable(), action: tableui.Action{Kind: tableui.DeleteTable, LineStart: t.lineStart}},
			} {
				if it.sep {
					m.Separator()
					continue
				}
				if m.Item(it.label).Chosen() {
					if it.action.Kind == tableui.DeleteRows && row == 0 || it.action.Kind == tableui.DeleteColumns && t.tab.cols() <= 1 {
						continue
					}
					_, _ = e.tableUI().Dispatch(it.action)
				}
			}
		})
	}
	m.Submenu(locales.EditorParagraphMenu(), func(m *ui.Menu) {
		lv := e.headingLevel()
		headings := []string{locales.ParagraphHeading1(), locales.ParagraphHeading2(), locales.ParagraphHeading3(), locales.ParagraphHeading4(), locales.ParagraphHeading5(), locales.ParagraphHeading6()}
		for i := 1; i <= 6; i++ {
			if m.Item(headings[i-1]).Shortcut(ui.Cmd, ui.Key0+ui.Key(i)).Checked(lv == i).Chosen() {
				e.SetHeading(i)
			}
		}
		if m.Item(locales.ParagraphBody()).Shortcut(ui.Cmd, ui.Key0).Checked(lv == 0).Chosen() {
			e.SetHeading(0)
		}
		m.Separator()
		if m.Item(locales.ParagraphQuote()).Shortcut(ui.Cmd|ui.Shift, ui.KeyQ).Chosen() {
			e.ToggleQuote()
		}
		if m.Item(locales.ParagraphOrderedList()).Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketLeft).Chosen() {
			e.ToggleList("1.")
		}
		if m.Item(locales.ParagraphUnorderedList()).Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketRight).Chosen() {
			e.ToggleList("-")
		}
		if m.Item(locales.ParagraphTaskList()).Chosen() {
			e.ToggleList("- [ ]")
		}
		m.Separator()
		if m.Item(locales.ParagraphCodeBlock()).Shortcut(ui.Cmd|ui.Shift, ui.KeyK).Chosen() {
			e.InsertCodeBlock()
		}
		if m.Item(locales.ParagraphMathBlock()).Shortcut(ui.Cmd|ui.Shift, ui.KeyM).Chosen() {
			e.InsertMathBlock()
		}
		if m.Item(locales.ParagraphTable()).Shortcut(ui.Cmd|ui.Alt, ui.KeyT).Chosen() {
			e.InsertTable(3, 3)
		}
		if m.Item(locales.ParagraphRule()).Chosen() {
			e.InsertRule()
		}
	})
	m.Submenu(locales.EditorFormatMenu(), func(m *ui.Menu) {
		if m.Item(locales.FormatBold()).Shortcut(ui.Cmd, ui.KeyB).Chosen() {
			e.ToggleBold()
		}
		if m.Item(locales.FormatItalic()).Shortcut(ui.Cmd, ui.KeyI).Chosen() {
			e.ToggleItalic()
		}
		if m.Item(locales.FormatUnderline()).Shortcut(ui.Cmd, ui.KeyU).Chosen() {
			e.ToggleUnderline()
		}
		if m.Item(locales.FormatStrike()).Shortcut(ui.Cmd|ui.Shift, ui.KeyX).Chosen() {
			e.ToggleStrike()
		}
		if m.Item(locales.FormatInlineCode()).Shortcut(ui.Cmd, ui.KeyE).Chosen() {
			e.ToggleCode()
		}
		if m.Item(locales.FormatHighlight()).Shortcut(ui.Cmd|ui.Shift, ui.KeyH).Chosen() {
			e.ToggleHighlight()
		}
		if m.Item(locales.FormatLink()).Shortcut(ui.Cmd, ui.KeyK).Chosen() {
			e.InsertLink()
		}
	})
	m.Separator()
	if m.Item(locales.CommonSelectAllAction()).Shortcut(ui.Cmd, ui.KeyA).Chosen() {
		e.SelectAll()
	}
}

// copyImageObject writes an image embed to the Markdown clipboard format. A
// subsequent paste in Muse therefore creates an internal image link and keeps
// the original image file untouched.
func (e *Editor) copyImageObject(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	target := filepath.Base(path)
	if root := strings.TrimSpace(e.opts.AssetRoot); root != "" {
		if rel, err := filepath.Rel(root, path); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			target = filepath.ToSlash(rel)
		}
	}
	target = strings.ReplaceAll(target, "|", "\\|")
	target = strings.ReplaceAll(target, "]", "\\]")
	md := "![[" + target + "]]"
	data := transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.Text, []byte(md)),
		transfer.Bytes(transfer.Markdown, []byte(md)),
	))
	if e.opts.WriteClipboard != nil {
		e.opts.WriteClipboard(data)
	} else {
		e.services.WriteClipboardData(data)
	}
}

// CopyImageObject places an image embed in the Markdown clipboard format so
// pasting into another Muse note keeps referring to the original asset.
func (e *Editor) CopyImageObject(path string) { e.copyImageObject(path) }
