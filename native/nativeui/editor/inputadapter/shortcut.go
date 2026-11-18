package inputadapter

import editorinput "muse/editor/input"

// ShortcutCommand is a toolkit-neutral editor command selected by a physical
// key plus platform modifiers. Execution remains with the Editor host.
type ShortcutCommand uint8

const (
	ShortcutNone ShortcutCommand = iota
	ShortcutToggleTask
	ShortcutInsertCodeBlock
	ShortcutInsertTable
	ShortcutToggleStrike
	ShortcutUndo
	ShortcutRedo
	ShortcutSelectAll
	ShortcutCopy
	ShortcutCut
	ShortcutPaste
	ShortcutToggleBold
	ShortcutToggleItalic
	ShortcutToggleUnderline
	ShortcutToggleCode
	ShortcutToggleHighlight
	ShortcutInsertLink
	ShortcutSelectWord
	ShortcutSetHeading
	ShortcutShiftHeadingUp
	ShortcutShiftHeadingDown
	ShortcutToggleQuote
	ShortcutToggleOrderedList
	ShortcutToggleUnorderedList
	ShortcutInsertMathBlock
	ShortcutInsertTableRow
)

// Shortcut resolves the editor's command-key bindings without performing any
// editor operation. The caller supplies the platform's Command-key identity.
func Shortcut(key editorinput.Key, shift, alt, isMac bool) ShortcutCommand {
	if alt {
		switch {
		case key == editorinput.KeyX && !shift:
			return ShortcutToggleTask
		case key == editorinput.KeyC && !shift:
			return ShortcutInsertCodeBlock
		case key == editorinput.KeyT && !shift:
			return ShortcutInsertTable
		case key == editorinput.Key5 && shift:
			return ShortcutToggleStrike
		default:
			return ShortcutNone
		}
	}
	switch {
	case key == editorinput.KeyZ && !shift:
		return ShortcutUndo
	case key == editorinput.KeyZ && shift, key == editorinput.KeyY && !isMac:
		return ShortcutRedo
	case key == editorinput.KeyA && !shift:
		return ShortcutSelectAll
	case key == editorinput.KeyC && !shift:
		return ShortcutCopy
	case key == editorinput.KeyX && !shift:
		return ShortcutCut
	case key == editorinput.KeyV:
		return ShortcutPaste
	case key == editorinput.KeyB && !shift:
		return ShortcutToggleBold
	case key == editorinput.KeyI && !shift:
		return ShortcutToggleItalic
	case key == editorinput.KeyU && !shift:
		return ShortcutToggleUnderline
	case key == editorinput.KeyE && !shift, key == editorinput.KeyBackquote && shift:
		return ShortcutToggleCode
	case key == editorinput.KeyX && shift:
		return ShortcutToggleStrike
	case key == editorinput.KeyH && shift:
		return ShortcutToggleHighlight
	case key == editorinput.KeyK && !shift:
		return ShortcutInsertLink
	case key == editorinput.KeyD && !shift:
		return ShortcutSelectWord
	case key >= editorinput.Key0 && key <= editorinput.Key6 && !shift:
		return ShortcutSetHeading
	case key == editorinput.KeyEqual && !shift:
		return ShortcutShiftHeadingUp
	case key == editorinput.KeyMinus && !shift:
		return ShortcutShiftHeadingDown
	case key == editorinput.KeyQ && shift:
		return ShortcutToggleQuote
	case key == editorinput.KeyBracketLeft && shift:
		return ShortcutToggleOrderedList
	case key == editorinput.KeyBracketRight && shift:
		return ShortcutToggleUnorderedList
	case key == editorinput.KeyK && shift:
		return ShortcutInsertCodeBlock
	case key == editorinput.KeyM && shift:
		return ShortcutInsertMathBlock
	case key == editorinput.KeyEnter && !shift:
		return ShortcutInsertTableRow
	default:
		return ShortcutNone
	}
}
