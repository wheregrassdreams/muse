// Package editor adapts backend-neutral editing sessions to MyGo widgets.
package editor

import (
	"github.com/egoist/mygo/ui"

	"muse/editor"
	editorinput "muse/editor/input"
	markdownui "muse/nativeui/components/markdown"
	"muse/nativeui/window"
)

// Options configure the Markdown presentation hosted by this editor surface.
type Options struct {
	Label    string
	Markdown markdownui.Options
	OnChange func()
	OnClose  func()
}

// Surface hosts one editor session as generic pane content. Its session owns
// document state; the component owns the toolkit view and input translation.
type Surface struct {
	session *editor.Session
	opts    Options
	closed  bool
}

// NewSurface adapts a session to the window content lifecycle.
func NewSurface(session *editor.Session, opts Options) *Surface {
	return &Surface{session: session, opts: opts}
}

func (s *Surface) Render(c *ui.Context) {
	if s == nil || s.closed {
		return
	}
	View(c, s.session, s.opts)
}

func (s *Surface) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	if s.opts.OnClose != nil {
		s.opts.OnClose()
	}
}

var _ window.Content = (*Surface)(nil)

// View renders the current session frame and translates MyGo input callbacks
// into backend-neutral editor intents. The frame is refreshed after each
// dispatch, so edits flow through Session before Markdown projection/render.
func View(c *ui.Context, session *editor.Session, opts Options) ui.Element {
	if session == nil {
		return ui.Box(c).Fill()
	}
	frame := session.Frame()
	label := opts.Label
	if label == "" {
		label = "Markdown editor"
	}
	return ui.Box(c).Fill().Focusable().AutoFocus().Role(ui.RoleTextField).Label(label).
		HandleInput(func(ev ui.InputEvent) bool {
			var event editorinput.Event
			var operation editor.EditorOperation
			switch ev.Kind {
			case ui.InputText:
				event = editorinput.Event{Kind: editorinput.InsertText, Text: ev.Text}
			case ui.InputKeyDown:
				if key, ok := keyEventFromUI(ev); ok {
					beforeSource, beforeSelection := session.Source(), session.Selection()
					if session.DispatchKey(key) {
						changed := session.Source() != beforeSource || session.Selection() != beforeSelection
						if changed && opts.OnChange != nil {
							opts.OnChange()
						}
						return true
					}
				}
				switch ev.Key {
				case ui.KeyBackspace:
					event.Kind = editorinput.DeleteBackward
				case ui.KeyDelete:
					event.Kind = editorinput.DeleteForward
				case ui.KeyLeft:
					operation = editor.EditorOperation{Kind: editor.OperationMoveHorizontal, Direction: -1, Extend: ev.Mods&ui.Shift != 0}
				case ui.KeyRight:
					operation = editor.EditorOperation{Kind: editor.OperationMoveHorizontal, Direction: 1, Extend: ev.Mods&ui.Shift != 0}
				case ui.KeyEnter:
					event = editorinput.Event{Kind: editorinput.InsertText, Text: "\n"}
				case ui.KeyZ:
					if ev.Mods&ui.Super != 0 || ev.Mods&ui.Ctrl != 0 {
						event.Kind = editorinput.Undo
						if ev.Mods&ui.Shift != 0 {
							event.Kind = editorinput.Redo
						}
					} else {
						return false
					}
				default:
					return false
				}
			default:
				return false
			}
			if operation.Kind != "" {
				before := session.Selection()
				handled := session.ApplyOperation(operation)
				if !handled {
					event = editorinput.Event{Kind: editorinput.MoveHorizontal, Direction: operation.Direction, Extend: operation.Extend}
				} else {
					changed := session.Selection() != before
					if changed && opts.OnChange != nil {
						opts.OnChange()
					}
					return changed
				}
			}
			result, err := session.Dispatch(event)
			if err != nil {
				return false
			}
			if result.Changed || result.SelectionChanged {
				if opts.OnChange != nil {
					opts.OnChange()
				}
			}
			return result.Changed || result.SelectionChanged || event.Kind == editorinput.InsertText
		}).Children(func() {
		markdownui.View(c, frame.Document, opts.Markdown)
	})
}
