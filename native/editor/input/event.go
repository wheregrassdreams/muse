package input

import text "github.com/wheregrassdreams/muse-editor/text/input"

type EventKind = text.EventKind

const (
	InsertText     = text.InsertText
	DeleteBackward = text.DeleteBackward
	DeleteForward  = text.DeleteForward
	MoveHorizontal = text.MoveHorizontal
	SetSelection   = text.SetSelection
	Undo           = text.Undo
	Redo           = text.Redo
)

type Event = text.Event
