package inputadapter

import "github.com/wheregrassdreams/muse-editor/text"

// ShortcutCommand identifies a reusable rich-editor action. Hosts register
// these IDs with their own command implementation.
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

// Shortcut resolves standard editor shortcuts without performing an edit.
func Shortcut(event text.KeyEvent, isMac bool) ShortcutCommand {
	key := event.Key
	mods := event.Modifiers
	shift := mods&text.ModShift != 0
	alt := mods&text.ModAlt != 0
	command := mods&text.ModMeta != 0 || !isMac && mods&text.ModControl != 0
	if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
		key = string(key[0] - ('a' - 'A'))
	}
	if alt {
		switch {
		case key == "X" && !shift:
			return ShortcutToggleTask
		case key == "C" && !shift:
			return ShortcutInsertCodeBlock
		case key == "T" && !shift:
			return ShortcutInsertTable
		case key == "5" && shift:
			return ShortcutToggleStrike
		default:
			return ShortcutNone
		}
	}
	if !command {
		return ShortcutNone
	}
	switch {
	case key == "Z" && !shift:
		return ShortcutUndo
	case key == "Z" && shift || key == "Y" && !isMac:
		return ShortcutRedo
	case key == "A" && !shift:
		return ShortcutSelectAll
	case key == "C" && !shift:
		return ShortcutCopy
	case key == "X" && !shift:
		return ShortcutCut
	case key == "V":
		return ShortcutPaste
	case key == "B" && !shift:
		return ShortcutToggleBold
	case key == "I" && !shift:
		return ShortcutToggleItalic
	case key == "U" && !shift:
		return ShortcutToggleUnderline
	case key == "E" && !shift || key == "`" && shift:
		return ShortcutToggleCode
	case key == "X" && shift:
		return ShortcutToggleStrike
	case key == "H" && shift:
		return ShortcutToggleHighlight
	case key == "K" && !shift:
		return ShortcutInsertLink
	case key == "D" && !shift:
		return ShortcutSelectWord
	case key >= "0" && key <= "6" && !shift:
		return ShortcutSetHeading
	case key == "=" && !shift:
		return ShortcutShiftHeadingUp
	case key == "-" && !shift:
		return ShortcutShiftHeadingDown
	case key == "Q" && shift:
		return ShortcutToggleQuote
	case key == "[" && shift:
		return ShortcutToggleOrderedList
	case key == "]" && shift:
		return ShortcutToggleUnorderedList
	case key == "K" && shift:
		return ShortcutInsertCodeBlock
	case key == "M" && shift:
		return ShortcutInsertMathBlock
	case key == "Enter" && !shift:
		return ShortcutInsertTableRow
	default:
		return ShortcutNone
	}
}
