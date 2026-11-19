// Package inputadapter translates MyGo input events to the headless editor's
// stable logical key contract. It does not interpret commands or own state.
package inputadapter

import (
	"github.com/egoist/mygo/ui"
	"github.com/wheregrassdreams/muse-editor/text"
)

// KeyEvent converts a physical MyGo key event to the toolkit-neutral editor
// event. Committed text remains on InputText events so IME composition is not
// confused with physical key presses.
func KeyEvent(ev ui.InputEvent) (text.KeyEvent, bool) {
	if ev.Kind != ui.InputKeyDown {
		return text.KeyEvent{}, false
	}
	key, ok := keyName(ev.Key)
	if !ok {
		return text.KeyEvent{}, false
	}
	return text.KeyEvent{Key: key, Modifiers: modifiers(ev.Mods), Repeat: ev.Repeat}, true
}

// TextEvent converts committed MyGo text. Composition events deliberately
// remain owned by the IME adapter.
func TextEvent(ev ui.InputEvent) (text.KeyEvent, bool) {
	if ev.Kind != ui.InputText || ev.Text == "" {
		return text.KeyEvent{}, false
	}
	return text.KeyEvent{Text: ev.Text, Modifiers: modifiers(ev.Mods), Repeat: ev.Repeat}, true
}

func modifiers(mods ui.Modifiers) text.Modifiers {
	var out text.Modifiers
	if mods&ui.Shift != 0 {
		out |= text.ModShift
	}
	if mods&ui.Ctrl != 0 {
		out |= text.ModControl
	}
	if mods&ui.Alt != 0 {
		out |= text.ModAlt
	}
	if mods&ui.Cmd != 0 {
		out |= text.ModMeta
	}
	return out
}

func keyName(key ui.Key) (string, bool) {
	switch key {
	case ui.Key0:
		return "0", true
	case ui.Key1:
		return "1", true
	case ui.Key2:
		return "2", true
	case ui.Key3:
		return "3", true
	case ui.Key4:
		return "4", true
	case ui.Key5:
		return "5", true
	case ui.Key6:
		return "6", true
	case ui.Key7:
		return "7", true
	case ui.Key8:
		return "8", true
	case ui.Key9:
		return "9", true
	case ui.KeyA:
		return "A", true
	case ui.KeyB:
		return "B", true
	case ui.KeyC:
		return "C", true
	case ui.KeyD:
		return "D", true
	case ui.KeyE:
		return "E", true
	case ui.KeyF:
		return "F", true
	case ui.KeyG:
		return "G", true
	case ui.KeyH:
		return "H", true
	case ui.KeyI:
		return "I", true
	case ui.KeyJ:
		return "J", true
	case ui.KeyK:
		return "K", true
	case ui.KeyL:
		return "L", true
	case ui.KeyM:
		return "M", true
	case ui.KeyN:
		return "N", true
	case ui.KeyO:
		return "O", true
	case ui.KeyP:
		return "P", true
	case ui.KeyQ:
		return "Q", true
	case ui.KeyR:
		return "R", true
	case ui.KeyS:
		return "S", true
	case ui.KeyT:
		return "T", true
	case ui.KeyU:
		return "U", true
	case ui.KeyV:
		return "V", true
	case ui.KeyW:
		return "W", true
	case ui.KeyX:
		return "X", true
	case ui.KeyY:
		return "Y", true
	case ui.KeyZ:
		return "Z", true
	case ui.KeyBackquote:
		return "`", true
	case ui.KeyBracketLeft:
		return "[", true
	case ui.KeyBracketRight:
		return "]", true
	case ui.KeyBackspace:
		return "Backspace", true
	case ui.KeyDelete:
		return "Delete", true
	case ui.KeyDown:
		return "ArrowDown", true
	case ui.KeyEnd:
		return "End", true
	case ui.KeyEnter:
		return "Enter", true
	case ui.KeyEqual:
		return "=", true
	case ui.KeyEscape:
		return "Escape", true
	case ui.KeyHome:
		return "Home", true
	case ui.KeyLeft:
		return "ArrowLeft", true
	case ui.KeyMinus:
		return "-", true
	case ui.KeyPageDown:
		return "PageDown", true
	case ui.KeyPageUp:
		return "PageUp", true
	case ui.KeyRight:
		return "ArrowRight", true
	case ui.KeyTab:
		return "Tab", true
	case ui.KeyUp:
		return "ArrowUp", true
	default:
		return "", false
	}
}
