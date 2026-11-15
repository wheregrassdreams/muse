// Package editor adapts backend-neutral editing sessions to MyGo widgets.
package editor

import (
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"

	"muse/editor"
	editorinput "muse/editor/input"
	mdrender "muse/markdown/render"
	"muse/markdown/syntax"
	markdownui "muse/nativeui/components/markdown"
	"muse/nativeui/window"
)

const maxEditableBlockBytes = 32 << 10

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
	frame   editor.Frame
	active  int
	editors map[int]*sourceBlockEditor
}

type sourceBlockEditor struct {
	start, end int
	source     string
	doc        *richtext.Document
}

// NewSurface adapts a session to the window content lifecycle.
func NewSurface(session *editor.Session, opts Options) *Surface {
	return &Surface{session: session, opts: opts, active: -1}
}

func (s *Surface) Render(c *ui.Context) {
	if s == nil || s.closed {
		return
	}
	if s.session == nil {
		return
	}
	if s.frame.Revision != s.session.Revision() || s.frame.Document == nil {
		s.frame = s.session.Frame()
	}
	frame := s.frame
	frame.Selection = s.session.Selection()
	editingBlock := s.active >= 0 && s.hasActiveBlock(frame)
	if s.active >= 0 && !editingBlock {
		s.active = -1
		s.editors = nil
	}
	opts := s.opts
	opts.Markdown.RenderBlock = s.renderBlock
	opts.Markdown.OnBlockClick = s.activateBlock
	viewFrame(c, s.session, frame, opts, editingBlock)
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
	return viewFrame(c, session, session.Frame(), opts, false)
}

func viewFrame(c *ui.Context, session *editor.Session, frame editor.Frame, opts Options, editingBlock bool) ui.Element {
	label := opts.Label
	if label == "" {
		label = "Markdown editor"
	}
	return ui.Box(c).Fill().Focusable().AutoFocus().Role(ui.RoleTextField).Label(label).
		HandleInput(func(ev ui.InputEvent) bool {
			// The focused richtext block owns ordinary input and IME. Let it
			// handle text and IME. Physical keys still pass through the editor
			// keymap first so Vim and table commands retain their behavior.
			if editingBlock {
				if ev.Kind == ui.InputKeyDown {
					if key, ok := keyEventFromUI(ev); ok && session.DispatchKey(key) {
						return true
					}
				}
				return false
			}
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
			return result.Handled
		}).Children(func() {
		markdownui.View(c, frame.Document, opts.Markdown)
	})
}

func (s *Surface) activateBlock(block mdrender.Block) {
	start, end, source, ok := editableSourceRange(s.frame.Source, block)
	if !ok {
		return
	}
	s.active = start
	state := s.blockEditor(start, end, source)
	selection := s.frame.Selection
	if selection.Anchor >= start && selection.Anchor <= end && selection.Head >= start && selection.Head <= end {
		state.doc.SetSelection(runeOffset(source, selection.Anchor-start), runeOffset(source, selection.Head-start))
	} else {
		state.doc.SetSelection(0, 0)
		_ = s.session.SetSelection(editor.Selection{Anchor: start, Head: start})
	}
}

func (s *Surface) renderBlock(c *ui.Context, block mdrender.Block) bool {
	if s.active < 0 {
		return false
	}
	start, end, source, ok := editableSourceRange(s.frame.Source, block)
	if !ok || start != s.active {
		return false
	}
	state := s.blockEditor(start, end, source)
	selection := s.frame.Selection
	if selection.Anchor >= start && selection.Anchor <= end && selection.Head >= start && selection.Head <= end {
		state.doc.SetSelection(runeOffset(source, selection.Anchor-start), runeOffset(source, selection.Head-start))
	}
	el := richtext.EditorBase(c, state.doc, richtext.EditorOptions{
		Label: "Markdown paragraph", NoMarkdown: true,
	}).FillWidth().Focusable().AutoFocus()
	if el.Changed() {
		s.commitBlockChange(state)
	}
	return true
}

func (s *Surface) hasActiveBlock(frame editor.Frame) bool {
	if frame.Document == nil {
		return false
	}
	for _, block := range frame.Rendered.Blocks {
		start, _, _, ok := editableSourceRange(frame.Source, block)
		if ok && start == s.active {
			return true
		}
	}
	return false
}

func (s *Surface) blockEditor(start, end int, source string) *sourceBlockEditor {
	if s.editors == nil {
		s.editors = make(map[int]*sourceBlockEditor)
	}
	state := s.editors[start]
	if state == nil || state.source != source || state.end != end {
		state = &sourceBlockEditor{start: start, end: end, source: source, doc: richtext.NewDocument(richtext.NewText(source))}
		s.editors[start] = state
	}
	return state
}

func (s *Surface) commitBlockChange(state *sourceBlockEditor) {
	updated := state.doc.Text().PlainText()
	old := state.source
	if old == updated {
		return
	}
	prefix, suffix := sourceDiff(old, updated)
	oldEnd, newEnd := len(old)-suffix, len(updated)-suffix
	start, end := state.start+prefix, state.start+oldEnd
	before := s.session.Selection()
	_, caret := state.doc.Selection()
	caretByte := state.start + byteOffset(updated, caret)
	tx := editor.Transaction{
		Before: before,
		After:  editor.Selection{Anchor: caretByte, Head: caretByte},
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: start, End: end}, Text: updated[prefix:newEnd]}},
	}
	if _, err := s.session.ApplyTransaction(tx); err != nil {
		state.doc.SetText(richtext.NewText(old))
		return
	}
	state.source = updated
	state.end = state.start + len(updated)
	s.frame = editor.Frame{}
	if s.opts.OnChange != nil {
		s.opts.OnChange()
	}
}

func editableSourceRange(source string, block mdrender.Block) (int, int, string, bool) {
	if block.Kind != syntax.Paragraph && block.Kind != syntax.Heading || len(block.Inlines) != 1 || len(block.Inlines[0].Text) > maxEditableBlockBytes {
		return 0, 0, "", false
	}
	span := block.Inlines[0]
	if span.Bold || span.Italic || span.Code || span.Strike || span.Link != "" || span.Math || span.SoftBreak || span.Mention || span.Reference {
		return 0, 0, "", false
	}
	if span.SourceStart < 0 || span.SourceEnd < span.SourceStart || span.SourceEnd > len(source) || source[span.SourceStart:span.SourceEnd] != span.Text {
		return 0, 0, "", false
	}
	return span.SourceStart, span.SourceEnd, span.Text, true
}

func sourceDiff(old, updated string) (prefix, suffix int) {
	limit := min(len(old), len(updated))
	for prefix < limit && old[prefix] == updated[prefix] {
		prefix++
	}
	for prefix > 0 && prefix < len(old) && prefix < len(updated) && (!utf8.RuneStart(old[prefix]) || !utf8.RuneStart(updated[prefix])) {
		prefix--
	}
	limit = min(len(old)-prefix, len(updated)-prefix)
	for suffix < limit && old[len(old)-suffix-1] == updated[len(updated)-suffix-1] {
		suffix++
	}
	for suffix > 0 {
		oldStart, newStart := len(old)-suffix, len(updated)-suffix
		if (oldStart == len(old) || utf8.RuneStart(old[oldStart])) && (newStart == len(updated) || utf8.RuneStart(updated[newStart])) {
			break
		}
		suffix--
	}
	return prefix, suffix
}

func runeOffset(text string, byteOffset int) int {
	byteOffset = max(0, min(byteOffset, len(text)))
	return utf8.RuneCountInString(text[:byteOffset])
}

func byteOffset(text string, runeOffset int) int {
	if runeOffset <= 0 {
		return 0
	}
	for byteOffset, n := 0, 0; byteOffset < len(text); n++ {
		if n == runeOffset {
			return byteOffset
		}
		_, size := utf8.DecodeRuneInString(text[byteOffset:])
		byteOffset += size
	}
	return len(text)
}
