package mdedit

import (
	"github.com/egoist/mygo/ui"
	"strconv"
	"strings"
	"unicode/utf8"
)

// modalState implements a deliberately small modal subset, not a Vim emulator.
// Printable commands are consumed ONLY from text events, never KeyDown.
type modalState struct {
	keymap, mode, pending string
	count, opCount        int
	line                  bool
	register              string
	registerLine          bool
	registerTable         *tableRegister
	registerBlock         []string
	blockInsert           *tableBlockInsert
}

func (e *Editor) SetKeymap(s string) {
	e.finishBlockInput()
	if s != "vim" && s != "helix" {
		s = "default"
	}
	e.modal = modalState{keymap: s, mode: "Normal"}
	e.block, e.blockInput = nil, nil
	e.selectionInclusive = false
	e.selectionLinewise = false
	e.breakCoalescing()
	e.invalidate()
}
func (e *Editor) Keymap() string {
	if e.modal.keymap == "" {
		return "default"
	}
	return e.modal.keymap
}
func (e *Editor) ModeLabel() string {
	if e.Keymap() == "default" {
		return ""
	}
	label := e.Keymap() + " " + e.modal.mode
	if e.modal.opCount > 0 {
		label += " " + strconv.Itoa(e.modal.opCount)
	}
	label += e.modal.pending
	if e.modal.count > 0 {
		label += strconv.Itoa(e.modal.count)
	}
	return label
}

// ModalAcceptsText must guard all native composition/replacement entry points.
func (e *Editor) ModalAcceptsText() bool { return e.Keymap() == "default" || e.modal.mode == "Insert" }
func (e *Editor) modalKey(k ui.Key, m ui.Modifiers) bool {
	if e.Keymap() == "default" || m&ui.Cmd != 0 {
		return false
	}
	if k == ui.KeyR && m == ui.Ctrl {
		e.Redo()
		e.modal.mode = "Normal"
		e.modal.line = false
		e.selectionInclusive = false
		e.selectionLinewise = false
		e.anchor = e.caret
		return true
	}
	if k == ui.KeyV && m == ui.Ctrl {
		e.caret = e.selectionFocus()
		if u := e.unitAt(e.caret); u != nil && u.table != nil {
			e.tableBridge = nil
			e.tableNavigation = nil
			e.modal.mode = "BlockVisual"
			e.modal.line = false
			e.modal.pending = ""
			e.modal.count = 0
			e.anchor = e.caret
			e.block = nil
			e.nativeStartTableColumnVisual(e.caret)
			e.selectionInclusive = false
			e.selectionLinewise = false
			e.invalidate()
			return true
		}
		e.tableBridge = nil
		e.tableNavigation = nil
		e.modal.mode = "BlockVisual"
		e.block = nil
		e.startBlockSelection()
		e.modal.line = false
		e.selectionInclusive = false
		e.selectionLinewise = false
		e.modal.pending = ""
		e.modal.count = 0
		e.anchor = e.caret
		e.tableSelection = nil
		e.invalidate()
		return true
	}
	if k == ui.KeyEscape {
		if e.hist.open != nil {
			e.commit(e.caret, e.caret)
		}
		e.caret = e.selectionFocus()
		e.tableNavigation = nil
		e.compose = ""
		e.composing = false
		e.modal.mode = "Normal"
		e.selectionInclusive = false
		e.selectionLinewise = false
		e.modal.pending = ""
		e.modal.count = 0
		e.modal.opCount = 0
		e.modal.line = false
		e.tableSelection = nil
		e.tableHandleDrag = nil
		e.tableBridge = nil
		e.modal.blockInsert = nil
		e.block = nil
		e.blockInput = nil
		e.anchor = e.caret
		e.breakCoalescing()
		return true
	}
	if e.blockInput != nil {
		switch k {
		case ui.KeyBackspace:
			e.editBlockInputDelete(true)
			return true
		case ui.KeyDelete:
			e.editBlockInputDelete(false)
			return true
		case ui.KeyEnter:
			e.typeBlockInput("\n")
			return true
		case ui.KeyTab:
			e.typeBlockInput("\t")
			return true
		}
		e.finishBlockInput()
	}
	if e.ModalAcceptsText() {
		return false
	}
	if m&(ui.Alt|ui.Ctrl) != 0 {
		return true
	}
	switch k {
	case ui.KeyBackspace, ui.KeyDelete, ui.KeyEnter, ui.KeyTab:
		return true
	}
	// Let printable keys reach the text callback (preserves case and keyboard layout).
	switch k {
	case ui.KeyLeft:
		return e.modalText("h")
	case ui.KeyRight:
		return e.modalText("l")
	case ui.KeyUp:
		return e.modalText("k")
	case ui.KeyDown:
		return e.modalText("j")
	}
	return false
}
func (e *Editor) modalLine(at int) (int, int) {
	return e.sourceLineRange(at)
}
func (e *Editor) modalApply(op string, a, z int) {
	if a > z {
		a, z = z, a
	}
	e.modal.register = e.src[a:z]
	e.modal.registerLine = e.modal.line
	e.modal.registerTable = nil
	e.modal.registerBlock = nil
	e.modal.line = false
	e.selectionInclusive = false
	e.selectionLinewise = false
	e.anchor, e.caret = a, z
	if op == "y" {
		e.Copy()
		e.anchor, e.caret = a, a
		return
	}
	e.begin(stepEdit)
	e.replace(a, z, "")
	e.commit(a, a)
	if op == "c" {
		e.modal.mode = "Insert"
	}
}
func (e *Editor) modalText(s string) bool {
	if e.ModalAcceptsText() {
		return false
	}
	defer e.invalidate()
	// IME commits and multi-character payloads must never be interpreted as commands.
	if utf8.RuneCountInString(s) != 1 {
		return true
	}
	m := &e.modal
	c := s
	if c != "h" && c != "l" && !(c >= "0" && c <= "9") {
		e.tableNavigation = nil
	}
	if c >= "0" && c <= "9" && (c != "0" || m.count > 0) {
		m.count = min(9999, m.count*10+int(c[0]-'0'))
		return true
	}
	n := max(1, m.count)
	m.count = 0
	if m.pending == "g" {
		m.pending = ""
		if c == "g" {
			e.nativeSetCaret(0, m.mode == "Visual" || m.mode == "Select")
		} else if c == "j" || c == "k" {
			dir := 1
			if c == "k" {
				dir = -1
			}
			e.nativeVertical(dir, m.mode == "Visual" || m.mode == "Select" || m.mode == "BlockVisual")
		}
		return true
	}
	if c != "j" && c != "k" {
		e.hasGoal = false
	}
	if m.mode == "BlockVisual" && e.block != nil {
		switch c {
		case "y", "d", "c":
			ranges := e.SelectionRanges()
			m.registerBlock = nil
			for _, r := range ranges {
				m.registerBlock = append(m.registerBlock, e.src[r.Start:r.End])
			}
			if c == "y" {
				e.Copy()
				m.mode = "Normal"
				e.block = nil
				e.anchor = e.caret
				return true
			}
			at := e.caret
			if c == "c" {
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
				e.blockInput = &blockInput{points: points}
				e.sync()
				if len(points) > 0 {
					at = points[0]
				}
			} else {
				at = e.deleteBlock()
			}
			e.block = nil
			e.anchor, e.caret = at, at
			if c == "c" {
				m.mode = "Insert"
			} else {
				m.mode = "Normal"
			}
			return true
		case "I", "A":
			points := make([]int, 0)
			for _, r := range e.SelectionRanges() {
				if c == "I" {
					points = append(points, r.Start)
				} else {
					points = append(points, r.End)
				}
			}
			e.blockInput = &blockInput{points: points}
			e.block = nil
			m.mode = "Insert"
			e.anchor = e.caret
			return true
		}
	}
	if m.pending != "" && c == m.pending {
		op := m.pending
		m.pending = ""
		a, z := e.modalLine(e.caret)
		for i := 0; i < n*max(1, m.opCount); i++ {
			if z < len(e.src) {
				z++
				if i+1 < n*max(1, m.opCount) {
					_, z = e.modalLine(z)
				}
			}
		}
		m.opCount = 0
		m.line = true
		if op == "c" && z > a && e.src[z-1] == '\n' {
			z--
		}
		e.modalApply(op, a, z)
		return true
	}
	extend := m.mode == "Visual" || m.mode == "Select" || m.mode == "BlockVisual" || m.pending != ""
	start := e.caret
	motion := true
	for i := 0; i < n*max(1, m.opCount); i++ {
		switch c {
		case "h":
			e.nativeHorizontal(-1, extend)
		case "l":
			e.nativeHorizontal(1, extend)
		case "j":
			if m.line {
				e.nativeMoveLineSelection(1, 1)
			} else if m.mode == "BlockVisual" && e.moveTableVisualColumn(1, 1) {
			} else if !e.moveStructuralRow(1, extend) {
				e.nativeVertical(1, extend)
			}
		case "k":
			if m.line {
				e.nativeMoveLineSelection(-1, 1)
			} else if m.mode == "BlockVisual" && e.moveTableVisualColumn(-1, 1) {
			} else if !e.moveStructuralRow(-1, extend) {
				e.nativeVertical(-1, extend)
			}
		case "w":
			e.nativeSetCaret(e.step(e.caret, 1, true), extend)
		case "b":
			e.nativeSetCaret(e.step(e.caret, -1, true), extend)
		case "e":
			z := e.wordEndAt(e.caret)
			if i > 0 {
				if next := e.nextWordEndAt(e.caret); next > e.caret {
					z = next
				}
			} else if m.pending == "" && e.previousGraphemeBoundary(z) <= e.caret {
				// Vim `e` advances to the next word when the caret is already
				// on the current word's final grapheme. Keep operator-pending
				// `de` on the current inclusive word end.
				if next := e.nextWordEndAt(e.caret); next > e.caret {
					z = next
				}
			}
			if m.pending == "" && z > e.caret {
				z = e.previousGraphemeBoundary(z)
			}
			e.nativeSetCaret(z, extend)
		case "0":
			a := e.nativeLineStart(e.caret, false)
			e.nativeSetCaret(a, extend)
		case "^":
			a := e.nativeLineStart(e.caret, true)
			e.nativeSetCaret(a, extend)
		case "$":
			_, z := e.modalLine(e.caret)
			if m.pending == "" {
				lineStart, _ := e.modalLine(e.caret)
				if z > lineStart {
					z = e.previousGraphemeBoundary(z)
				}
			}
			e.nativeSetCaret(z, extend)
		case "G":
			e.nativeSetCaret(len(e.src), extend)
		default:
			motion = false
		}
	}
	if m.mode == "BlockVisual" && motion {
		if (c == "j" || c == "k") && e.hasGoal {
			e.blockChangedVertical(e.goalX)
		} else {
			e.blockChanged()
		}
	}
	if motion {
		if m.pending != "" {
			op := m.pending
			m.pending = ""
			m.opCount = 0
			e.modalApply(op, start, e.caret)
		} else if e.Keymap() == "helix" && !extend {
			e.anchor = start
		}
		return true
	}
	m.opCount = 0
	switch c {
	case "g":
		m.pending = "g"
	case "i", "a":
		focus := e.nativePrepareInsert()
		if c == "a" {
			e.caret = e.step(focus, 1, false)
		}
		e.anchor = e.caret
		m.mode = "Insert"
		e.breakCoalescing()
	case "I":
		focus := e.nativePrepareInsert()
		e.caret = e.nativeLineStart(focus, true)
		e.anchor = e.caret
		m.mode = "Insert"
		e.breakCoalescing()
	case "A":
		focus := e.nativePrepareInsert()
		e.caret = e.nativeLineEndAt(focus)
		e.anchor = e.caret
		m.mode = "Insert"
		e.breakCoalescing()
		e.invalidate()
	case "o":
		e.nativePrepareInsert()
		e.nativeOpenLine(false)
		m.mode = "Insert"
	case "O":
		e.nativePrepareInsert()
		e.nativeOpenLine(true)
		m.mode = "Insert"
	case "v":
		e.caret = e.selectionFocus()
		e.tableSelection = nil
		e.tableBridge = nil
		m.line = false
		if e.Keymap() == "helix" {
			m.mode = "Select"
			e.selectionInclusive = false
			e.selectionLinewise = false
		} else {
			m.mode = "Visual"
			e.selectionInclusive = true
			e.selectionLinewise = false
		}
		e.anchor = e.caret
	case "V":
		e.caret = e.selectionFocus()
		e.tableSelection = nil
		e.tableBridge = nil
		m.mode = "Visual"
		m.line = true
		e.selectionInclusive = false
		e.selectionLinewise = true
		e.nativeSelectLine(e.caret)
		if n > 1 {
			e.nativeMoveLineSelection(1, n-1)
		}
	case "d", "c", "y":
		if e.tableSelection != nil && e.tableBridge == nil && e.applyTableSelection(c) {
			if c != "c" {
				m.mode = "Normal"
			}
			return true
		}
		if e.Keymap() == "helix" || extend {
			a, z := e.selection()
			if m.line {
				// V mode keeps its complete source-line range in the native
				// selection throughout motion, so editing consumes it directly.
			}
			if a == z {
				z = e.step(z, 1, false)
			}
			e.modalApply(c, a, z)
			if c != "c" {
				m.mode = "Normal"
			}
		} else {
			m.pending = c
			m.opCount = n
		}
	case "p", "P":
		if len(m.registerBlock) > 0 {
			e.pasteBlockRegister(m.registerBlock, c == "P", n)
			return true
		}
		if m.registerTable != nil && e.pasteTableRegister(c == "P") {
			return true
		}
		if m.register != "" {
			at := e.caret
			value := strings.Repeat(m.register, n)
			if m.registerLine {
				if c == "P" {
					at, _ = e.modalLine(at)
				} else {
					_, at = e.modalLine(at)
					if at < len(e.src) {
						at++
					} else {
						value = "\n" + strings.TrimSuffix(value, "\n")
					}
				}
			} else if at < len(e.src) {
				_, size := utf8.DecodeRuneInString(e.src[at:])
				at += size
			}
			e.begin(stepEdit)
			e.replace(at, at, value)
			e.commit(at, at)
		} else {
			e.anchor = e.caret
			e.Paste(false)
		}
	case "u":
		e.Undo()
		m.mode = "Normal"
		m.line = false
		e.selectionInclusive = false
		e.selectionLinewise = false
		e.anchor = e.caret
	case "x":
		if a, z, ok := e.nativeSubstituteRange(n, false); ok {
			m.line = false
			e.modalApply("d", a, z)
		}
	case "X":
		if a, z, ok := e.nativeSubstituteRange(n, true); ok {
			m.line = false
			e.modalApply("d", a, z)
		}
	case "D":
		if a, z, ok := e.nativeLineRemainder(); ok {
			m.line = false
			e.modalApply("d", a, z)
		}
	case "C":
		if a, z, ok := e.nativeLineRemainder(); ok {
			m.line = false
			e.modalApply("c", a, z)
		}
	}
	return true
}
