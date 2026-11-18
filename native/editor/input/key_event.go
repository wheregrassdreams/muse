package input

import text "github.com/wheregrassdreams/muse-editor/text"

// Key and KeyEvent use the editor engine's stable logical key contract.
// Native toolkit normalization lives in muse-editor-mygo/inputadapter.
type Key = string
type Modifiers = text.Modifiers
type KeyEvent = text.KeyEvent

const (
	KeyUnknown      Key = ""
	Key0            Key = "0"
	Key1            Key = "1"
	Key2            Key = "2"
	Key3            Key = "3"
	Key4            Key = "4"
	Key5            Key = "5"
	Key6            Key = "6"
	Key7            Key = "7"
	Key8            Key = "8"
	Key9            Key = "9"
	KeyA            Key = "A"
	KeyB            Key = "B"
	KeyC            Key = "C"
	KeyD            Key = "D"
	KeyE            Key = "E"
	KeyF            Key = "F"
	KeyG            Key = "G"
	KeyH            Key = "H"
	KeyI            Key = "I"
	KeyJ            Key = "J"
	KeyK            Key = "K"
	KeyL            Key = "L"
	KeyM            Key = "M"
	KeyN            Key = "N"
	KeyO            Key = "O"
	KeyP            Key = "P"
	KeyQ            Key = "Q"
	KeyR            Key = "R"
	KeyS            Key = "S"
	KeyT            Key = "T"
	KeyU            Key = "U"
	KeyV            Key = "V"
	KeyW            Key = "W"
	KeyX            Key = "X"
	KeyY            Key = "Y"
	KeyZ            Key = "Z"
	KeyBackquote    Key = "`"
	KeyBracketLeft  Key = "["
	KeyBracketRight Key = "]"
	KeyBackspace    Key = "Backspace"
	KeyDelete       Key = "Delete"
	KeyDown         Key = "ArrowDown"
	KeyEnd          Key = "End"
	KeyEnter        Key = "Enter"
	KeyEqual        Key = "="
	KeyEscape       Key = "Escape"
	KeyHome         Key = "Home"
	KeyLeft         Key = "ArrowLeft"
	KeyMinus        Key = "-"
	KeyPageDown     Key = "PageDown"
	KeyPageUp       Key = "PageUp"
	KeyRight        Key = "ArrowRight"
	KeyTab          Key = "Tab"
	KeyUp           Key = "ArrowUp"
)

const (
	ModShift   = text.ModShift
	ModCtrl    = text.ModControl
	ModAlt     = text.ModAlt
	ModCommand = text.ModMeta
)
