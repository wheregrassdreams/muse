package editor

import (
	"github.com/egoist/mygo/ui"
	"muse/nativeui/editor/ime"
)

// The host supplies only source/edit and leaf-backed geometry capabilities.
// UTF-16 coordinates, marked ranges, and composition lifecycle stay in ime.Client.
var (
	_ ime.TextSession  = (*Editor)(nil)
	_ ime.GeometryHost = (*Editor)(nil)
)

func (e *Editor) TextInputSnapshot() ime.Snapshot {
	a, z := e.selection()
	selection := ime.Selection{
		Range:    ime.Range{Start: a, End: z},
		Reversed: e.Caret() == a && a != z,
	}
	return ime.Snapshot{
		Source:    e.Source(),
		Revision:  e.Session.Revision(),
		Caret:     e.Caret(),
		Selection: selection,
	}
}

func (e *Editor) compositionText() string { return e.imeClient.Composition().Text }

func (e *Editor) compositionSelection() ime.Range { return e.imeClient.Composition().Selection }

func (e *Editor) isComposing() bool { return e.imeClient.Composing() }

func (e *Editor) setComposition(text string, selection ui.TextInputRange) {
	e.imeClient.SetComposition(text, ime.Range{Start: selection.Start, End: selection.End})
}

func (e *Editor) clearComposition() { e.imeClient.SetComposition("", ime.Range{}) }

func (e *Editor) ModalAcceptsIMEText() bool      { return e.ModalAcceptsText() }
func (e *Editor) InsertIMEText(text string)      { e.typeText(text) }
func (e *Editor) InsertModalIMEText(text string) { e.modalText(text) }

func (e *Editor) ReplaceIMEBytes(start, end int, text string) {
	e.dropPhantomIfAway(start)
	if start == end && start == e.Caret() {
		e.layoutIfNeeded()
		e.trackBlankLineInput(text, stepEdit)
	}
	e.begin(stepEdit)
	e.replace(start, end, text)
	e.commit(start+len(text), start+len(text))
}

func (e *Editor) ReconvertIMEBytes(start, end int) {
	e.begin(stepEdit)
	e.replace(start, end, "")
	e.commit(start, start)
}

func (e *Editor) DeleteIMESelection() { e.deleteSelection() }

func (e *Editor) CaretBoundsForIMEOffset(offset int) (ui.Rect, bool) {
	return e.caretRect(offset)
}

func (e *Editor) CompositionCaretBounds() (ui.Rect, bool) { return e.composeCaretRect() }
func (e *Editor) IMESelectionCaretWidth() float32         { return e.lk.size * 0.6 }
func (e *Editor) HitTestIME(point ui.Point) int           { return e.hitTest(point.X, point.Y) }
func (e *Editor) InvalidateIME()                          { e.invalidate() }
func (e *Editor) RevealIMECaret()                         { e.wantReveal = true }

func (e *Editor) noteEdit(from, to int, old, replacement, source string) {
	e.imeClient.NoteEdit(from, to, old, replacement, source, e.Session.Revision())
}

// dropPhantomIfAway takes the phantom away when an edit is not in it.
func (e *Editor) dropPhantomIfAway(at int) {
	if e.phantom != nil && at != e.phantom.at {
		e.dropPhantom()
	}
}
