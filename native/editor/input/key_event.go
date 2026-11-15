package input

// Key is a physical, toolkit-neutral key value. Letters and digits use their
// unshifted identity; text input and composition arrive as separate events.
type Key uint8

const (
	KeyUnknown Key = iota
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	KeyBackquote
	KeyBracketLeft
	KeyBracketRight
	KeyBackspace
	KeyDelete
	KeyDown
	KeyEnd
	KeyEnter
	KeyEqual
	KeyEscape
	KeyHome
	KeyLeft
	KeyMinus
	KeyPageDown
	KeyPageUp
	KeyRight
	KeyTab
	KeyUp
)

// Modifiers is a toolkit-neutral set of keyboard modifiers.
type Modifiers uint8

const (
	ModShift Modifiers = 1 << iota
	ModCtrl
	ModAlt
	ModCommand
)

// KeyEvent carries a physical keydown to the editor's keymap policy.
type KeyEvent struct {
	Key       Key
	Modifiers Modifiers
}
