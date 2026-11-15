// Package editor adapts backend-neutral editing sessions to MyGo widgets.
package editor

import (
	"github.com/egoist/mygo/ui"

	"muse/editor"
	markdownui "muse/nativeui/components/markdown"
)

// Options configure the Markdown presentation hosted by this editor surface.
type Options struct {
	Label    string
	Markdown markdownui.Options
	OnChange func()
}

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
			var event editor.InputEvent
			switch ev.Kind {
			case ui.InputText:
				event = editor.InputEvent{Kind: editor.InputInsertText, Text: ev.Text}
			case ui.InputKeyDown:
				switch ev.Key {
				case ui.KeyBackspace:
					event.Kind = editor.InputDeleteBackward
				case ui.KeyDelete:
					event.Kind = editor.InputDeleteForward
				case ui.KeyLeft:
					event = editor.InputEvent{Kind: editor.InputMoveHorizontal, Direction: -1, Extend: ev.Mods&ui.Shift != 0}
				case ui.KeyRight:
					event = editor.InputEvent{Kind: editor.InputMoveHorizontal, Direction: 1, Extend: ev.Mods&ui.Shift != 0}
				case ui.KeyEnter:
					event = editor.InputEvent{Kind: editor.InputInsertText, Text: "\n"}
				case ui.KeyZ:
					if ev.Mods&ui.Super != 0 || ev.Mods&ui.Ctrl != 0 {
						event.Kind = editor.InputUndo
						if ev.Mods&ui.Shift != 0 {
							event.Kind = editor.InputRedo
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
			result, err := session.Dispatch(event)
			if err != nil {
				return false
			}
			if result.Changed || result.SelectionChanged {
				if opts.OnChange != nil {
					opts.OnChange()
				}
			}
			return result.Changed || result.SelectionChanged || event.Kind == editor.InputInsertText
		}).Children(func() {
		markdownui.View(c, frame.Document, opts.Markdown)
	})
}
