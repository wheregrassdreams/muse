package input

// EventKind names source-editing intents independently of a UI toolkit.
type EventKind uint8

const (
	InsertText EventKind = iota + 1
	DeleteBackward
	DeleteForward
	MoveHorizontal
	SetSelection
	Undo
	Redo
)

// Event carries a keymap-neutral editing intent. Coordinates are UTF-8 byte
// offsets into the current source snapshot.
type Event struct {
	Kind      EventKind
	Text      string
	Direction int
	Extend    bool
	Anchor    int
	Head      int
	Inclusive bool
	Linewise  bool
}
