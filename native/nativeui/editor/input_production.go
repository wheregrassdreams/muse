package editor

import (
	mygoinputadapter "github.com/wheregrassdreams/muse-editor-mygo/inputadapter"
	textengine "github.com/wheregrassdreams/muse-editor/text"
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
	"muse/editor/source"
	syntax "muse/markdown/syntax"
	media "muse/nativeui/editor/blocks/media"
	tableui "muse/nativeui/editor/blocks/table"
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
		key, ok := mygoinputadapter.KeyEvent(ev)
		return ok && e.DispatchKey(key)
	case ui.InputText:
		event, ok := mygoinputadapter.TextEvent(ev)
		if !ok {
			return false
		}
		if e.Keymap() == "vim" && e.vimController().Mode() == vim.ModeInsert {
			intent, handled := e.Session.HandleVimKey(event)
			if !handled {
				return false
			}
			if intent.Name != "" {
				_, err := e.Session.ExecuteVimIntent(intent)
				return err == nil
			}
			return true
		}
		result, err := e.Session.Dispatch(editorinput.Event{Kind: editorinput.InsertText, Text: event.Text})
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
		if e.shortcut(event) {
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
func (e *Editor) shortcut(event editorinput.KeyEvent) bool {
	command := mygoinputadapter.Shortcut(event, isMac)
	switch command {
	case mygoinputadapter.ShortcutNone:
		return false
	case mygoinputadapter.ShortcutToggleTask:
		e.ToggleList("- [ ]")
	case mygoinputadapter.ShortcutInsertCodeBlock:
		e.InsertCodeBlock()
	case mygoinputadapter.ShortcutInsertTable:
		e.InsertTable(3, 3)
	case mygoinputadapter.ShortcutToggleStrike:
		e.ToggleStrike()
	case mygoinputadapter.ShortcutUndo:
		e.Undo()
	case mygoinputadapter.ShortcutRedo:
		e.Redo()
	case mygoinputadapter.ShortcutSelectAll:
		e.SelectAll()
	case mygoinputadapter.ShortcutCopy:
		e.Copy()
	case mygoinputadapter.ShortcutCut:
		e.Cut()
	case mygoinputadapter.ShortcutPaste:
		e.Paste(event.Modifiers&editorinput.ModShift != 0)
	case mygoinputadapter.ShortcutToggleBold:
		e.ToggleBold()
	case mygoinputadapter.ShortcutToggleItalic:
		e.ToggleItalic()
	case mygoinputadapter.ShortcutToggleUnderline:
		e.ToggleUnderline()
	case mygoinputadapter.ShortcutToggleCode:
		e.ToggleCode()
	case mygoinputadapter.ShortcutToggleHighlight:
		e.ToggleHighlight()
	case mygoinputadapter.ShortcutInsertLink:
		e.InsertLink()
	case mygoinputadapter.ShortcutSelectWord:
		a, z := e.wordRangeAt(e.Caret())
		e.Select(a, z)
	case mygoinputadapter.ShortcutSetHeading:
		e.SetHeading(int(event.Key[0] - '0'))
	case mygoinputadapter.ShortcutShiftHeadingUp:
		e.ShiftHeading(1)
	case mygoinputadapter.ShortcutShiftHeadingDown:
		e.ShiftHeading(-1)
	case mygoinputadapter.ShortcutToggleQuote:
		e.ToggleQuote()
	case mygoinputadapter.ShortcutToggleOrderedList:
		e.ToggleList("1.")
	case mygoinputadapter.ShortcutToggleUnorderedList:
		e.ToggleList("-")
	case mygoinputadapter.ShortcutInsertMathBlock:
		e.InsertMathBlock()
	case mygoinputadapter.ShortcutInsertTableRow:
		t, r, c := e.cellPos(e.Caret())
		if t == nil {
			return false
		}
		_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.InsertRowAfter, LineStart: t.lineStart, First: r, Focus: tablemodel.Cell{Col: c}})
	}
	return true
}
