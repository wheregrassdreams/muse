package mdedit

import (
	"github.com/egoist/mygo/ui"
	"strconv"
	"strings"
	"unicode"
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
}

func (e *Editor) SetKeymap(s string) {
	if s != "vim" && s != "helix" {
		s = "default"
	}
	e.modal = modalState{keymap: s, mode: "Normal"}
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
	if k == ui.KeyEscape {
		e.compose = ""
		e.composing = false
		e.modal.mode = "Normal"
		e.modal.pending = ""
		e.modal.count = 0
		e.modal.opCount = 0
		e.modal.line = false
		e.anchor = e.caret
		e.breakCoalescing()
		return true
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
	a := strings.LastIndex(e.src[:at], "\n") + 1
	z := strings.Index(e.src[at:], "\n")
	if z < 0 {
		return a, len(e.src)
	}
	return a, at + z
}
func (e *Editor) modalApply(op string, a, z int) {
	if a > z {
		a, z = z, a
	}
	e.modal.register = e.src[a:z]
	e.modal.registerLine = e.modal.line
	e.modal.line = false
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
	if c >= "0" && c <= "9" && (c != "0" || m.count > 0) {
		m.count = min(9999, m.count*10+int(c[0]-'0'))
		return true
	}
	n := max(1, m.count)
	m.count = 0
	if m.pending == "g" {
		m.pending = ""
		if c == "g" {
			e.setCaret(0, m.mode == "Visual" || m.mode == "Select")
		}
		return true
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
	extend := m.mode == "Visual" || m.mode == "Select" || m.pending != ""
	start := e.caret
	motion := true
	for i := 0; i < n*max(1, m.opCount); i++ {
		switch c {
		case "h":
			e.setCaret(e.step(e.caret, -1, false), extend)
		case "l":
			e.setCaret(e.step(e.caret, 1, false), extend)
		case "j":
			e.vertical(1, extend)
		case "k":
			e.vertical(-1, extend)
		case "w":
			at := e.step(e.caret, 1, true)
			for at < len(e.src) {
				r, size := utf8.DecodeRuneInString(e.src[at:])
				if !unicode.IsSpace(r) {
					break
				}
				at += size
			}
			e.setCaret(at, extend)
		case "b":
			e.setCaret(e.step(e.caret, -1, true), extend)
		case "e":
			_, z := wordAround(e.src, e.caret)
			e.setCaret(z, extend)
		case "0":
			a, _ := e.modalLine(e.caret)
			e.setCaret(a, extend)
		case "$":
			_, z := e.modalLine(e.caret)
			e.setCaret(z, extend)
		case "G":
			e.setCaret(len(e.src), extend)
		default:
			motion = false
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
		if c == "a" {
			e.caret = e.step(e.caret, 1, false)
		}
		e.anchor = e.caret
		m.mode = "Insert"
		e.breakCoalescing()
	case "o":
		_, z := e.modalLine(e.caret)
		e.begin(stepEdit)
		e.replace(z, z, "\n")
		e.commit(z+1, z+1)
		m.mode = "Insert"
	case "v":
		if e.Keymap() == "helix" {
			m.mode = "Select"
		} else {
			m.mode = "Visual"
		}
		e.anchor = e.caret
	case "V":
		m.mode = "Visual"
		m.line = true
		e.anchor = e.caret
	case "d", "c", "y":
		if e.Keymap() == "helix" || extend {
			a, z := e.selection()
			if m.line {
				a, _ = e.modalLine(a)
				_, z = e.modalLine(z)
				if z < len(e.src) {
					z++
				}
			} else if m.mode == "Visual" {
				if z < len(e.src) {
					_, size := utf8.DecodeRuneInString(e.src[z:])
					z += size
				}
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
	case "p":
		if m.register != "" {
			at := e.caret
			value := strings.Repeat(m.register, n)
			if m.registerLine {
				_, at = e.modalLine(at)
				if at < len(e.src) {
					at++
				} else {
					value = "\n" + strings.TrimSuffix(value, "\n")
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
	}
	return true
}
