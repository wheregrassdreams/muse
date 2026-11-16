package inputadapter

import (
	"github.com/egoist/mygo/ui"
	editorinput "muse/editor/input"
)

// KeyEvent converts one MyGo key event into the editor's backend-neutral
// input contract. It contains no editor/session state and can be reused by
// every native editor surface.
func KeyEvent(ev ui.InputEvent) (editorinput.KeyEvent, bool) {
	var key editorinput.Key
	switch ev.Key {
	case ui.Key0:
		key = editorinput.Key0
	case ui.Key1:
		key = editorinput.Key1
	case ui.Key2:
		key = editorinput.Key2
	case ui.Key3:
		key = editorinput.Key3
	case ui.Key4:
		key = editorinput.Key4
	case ui.Key5:
		key = editorinput.Key5
	case ui.Key6:
		key = editorinput.Key6
	case ui.Key7:
		key = editorinput.Key7
	case ui.Key8:
		key = editorinput.Key8
	case ui.Key9:
		key = editorinput.Key9
	case ui.KeyA:
		key = editorinput.KeyA
	case ui.KeyB:
		key = editorinput.KeyB
	case ui.KeyC:
		key = editorinput.KeyC
	case ui.KeyD:
		key = editorinput.KeyD
	case ui.KeyE:
		key = editorinput.KeyE
	case ui.KeyF:
		key = editorinput.KeyF
	case ui.KeyG:
		key = editorinput.KeyG
	case ui.KeyH:
		key = editorinput.KeyH
	case ui.KeyI:
		key = editorinput.KeyI
	case ui.KeyJ:
		key = editorinput.KeyJ
	case ui.KeyK:
		key = editorinput.KeyK
	case ui.KeyL:
		key = editorinput.KeyL
	case ui.KeyM:
		key = editorinput.KeyM
	case ui.KeyN:
		key = editorinput.KeyN
	case ui.KeyO:
		key = editorinput.KeyO
	case ui.KeyP:
		key = editorinput.KeyP
	case ui.KeyQ:
		key = editorinput.KeyQ
	case ui.KeyR:
		key = editorinput.KeyR
	case ui.KeyS:
		key = editorinput.KeyS
	case ui.KeyT:
		key = editorinput.KeyT
	case ui.KeyU:
		key = editorinput.KeyU
	case ui.KeyV:
		key = editorinput.KeyV
	case ui.KeyW:
		key = editorinput.KeyW
	case ui.KeyX:
		key = editorinput.KeyX
	case ui.KeyY:
		key = editorinput.KeyY
	case ui.KeyZ:
		key = editorinput.KeyZ
	case ui.KeyBackquote:
		key = editorinput.KeyBackquote
	case ui.KeyBracketLeft:
		key = editorinput.KeyBracketLeft
	case ui.KeyBracketRight:
		key = editorinput.KeyBracketRight
	case ui.KeyBackspace:
		key = editorinput.KeyBackspace
	case ui.KeyDelete:
		key = editorinput.KeyDelete
	case ui.KeyDown:
		key = editorinput.KeyDown
	case ui.KeyEnd:
		key = editorinput.KeyEnd
	case ui.KeyEnter:
		key = editorinput.KeyEnter
	case ui.KeyEqual:
		key = editorinput.KeyEqual
	case ui.KeyEscape:
		key = editorinput.KeyEscape
	case ui.KeyHome:
		key = editorinput.KeyHome
	case ui.KeyLeft:
		key = editorinput.KeyLeft
	case ui.KeyMinus:
		key = editorinput.KeyMinus
	case ui.KeyPageDown:
		key = editorinput.KeyPageDown
	case ui.KeyPageUp:
		key = editorinput.KeyPageUp
	case ui.KeyRight:
		key = editorinput.KeyRight
	case ui.KeyTab:
		key = editorinput.KeyTab
	case ui.KeyUp:
		key = editorinput.KeyUp
	default:
		return editorinput.KeyEvent{}, false
	}
	var modifiers editorinput.Modifiers
	if ev.Mods&ui.Shift != 0 {
		modifiers |= editorinput.ModShift
	}
	if ev.Mods&ui.Ctrl != 0 {
		modifiers |= editorinput.ModCtrl
	}
	if ev.Mods&ui.Alt != 0 {
		modifiers |= editorinput.ModAlt
	}
	if ev.Mods&ui.Cmd != 0 {
		modifiers |= editorinput.ModCommand
	}
	return editorinput.KeyEvent{Key: key, Modifiers: modifiers}, true
}
