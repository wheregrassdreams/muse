package editor

import (
	"github.com/egoist/mygo/ui"
	"muse/nativeui/components/editor/ime"
)

// The host supplies session source/selection, editor-owned edit transactions,
// and leaf-backed caret geometry. UTF-16 coordinates and composition lifecycle
// are handled by ime.Client.
var _ ui.TextInputClient = (*Editor)(nil)

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

func (e *Editor) TextForRange(r ui.TextInputRange) (string, ui.TextInputRange) {
	return e.imeClient.TextForRange(r)
}

func (e *Editor) Selection() ui.TextInputSelection { return e.imeClient.Selection() }

func (e *Editor) MarkedRange() (ui.TextInputRange, bool) { return e.imeClient.MarkedRange() }

func (e *Editor) ReplaceText(r *ui.TextInputRange, text string) { e.imeClient.ReplaceText(r, text) }

func (e *Editor) SetMarkedText(r *ui.TextInputRange, text string, selected ui.TextInputRange) {
	e.imeClient.SetMarkedText(r, text, selected)
}

func (e *Editor) UnmarkText() { e.imeClient.UnmarkText() }

func (e *Editor) BoundsForRange(r ui.TextInputRange) (ui.Rect, ui.TextInputRange, bool) {
	return e.imeClient.BoundsForRange(r)
}

func (e *Editor) IndexForPoint(point ui.Point) (int, bool) { return e.imeClient.IndexForPoint(point) }

// dropPhantomIfAway takes the phantom away when an edit is not in it.
func (e *Editor) dropPhantomIfAway(at int) {
	if e.phantom != nil && at != e.phantom.at {
		e.dropPhantom()
	}
}
