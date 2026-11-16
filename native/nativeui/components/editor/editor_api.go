package editor

import (
	"strings"

	editormodel "muse/editor"
)

// The app edits the document through these, as an AI tool or a command
// does: each is one step of undo, so the user takes it back as their own.

// ReplaceAll makes src the document, as an edit of what changed rather
// than a new document: the caret stays where it was, around the change,
// and undo brings the old text back.
func (e *Editor) ReplaceAll(src string) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	if src == e.Source() {
		return
	}
	p := 0
	for p < len(e.Source()) && p < len(src) && e.Source()[p] == src[p] {
		p++
	}
	s := 0
	for s < len(e.Source())-p && s < len(src)-p && e.Source()[len(e.Source())-1-s] == src[len(src)-1-s] {
		s++
	}
	// Keep the change on whole runes.
	for p > 0 && p < len(e.Source()) && !runeStart(e.Source()[p]) {
		p--
	}
	for s > 0 && len(e.Source())-s < len(e.Source()) && !runeStart(e.Source()[len(e.Source())-s]) {
		s--
	}
	e.replaceRange(p, len(e.Source())-s, src[p:len(src)-s])
}

// Replace replaces the source's bytes [from, to) with text, as one step of
// undo.
func (e *Editor) Replace(from, to int, text string) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	e.replaceRange(from, to, text)
}

func (e *Editor) replaceRange(from, to int, text string) {
	e.dropPhantom()
	from = max(0, min(from, len(e.Source())))
	to = max(from, min(to, len(e.Source())))
	e.breakCoalescing()
	e.begin(stepEdit)
	e.replace(from, to, text)
	e.commit(e.Session.Selection().Anchor, e.Caret())
	e.breakCoalescing()
	e.invalidate()
}

// AppendBlock adds Markdown as a block of its own at the end of the
// document, and puts the caret at its end.
func (e *Editor) AppendBlock(md string) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	md = strings.Trim(md, "\n")
	if md == "" {
		return
	}
	body := strings.TrimRight(e.Source(), "\n")
	sep := "\n\n"
	if body == "" {
		sep = ""
	}
	e.dropPhantom()
	e.breakCoalescing()
	e.begin(stepEdit)
	e.replace(len(body), len(e.Source()), sep+md+"\n")
	end := len(body) + len(sep) + len(md)
	e.commit(end, end)
	e.breakCoalescing()
	e.invalidate()
}

// PrependBlock adds Markdown as a block of its own at the start of the
// document.
func (e *Editor) PrependBlock(md string) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	md = strings.Trim(md, "\n")
	if md == "" {
		return
	}
	rest := strings.TrimLeft(e.Source(), "\n")
	sep := "\n\n"
	if rest == "" {
		sep = "\n"
	}
	e.dropPhantom()
	e.breakCoalescing()
	e.begin(stepEdit)
	e.replace(0, len(e.Source())-len(rest), md+sep)
	e.commit(len(md), len(md))
	e.breakCoalescing()
	e.invalidate()
}

// InsertAtCaret types Markdown at the caret, as pasted.
func (e *Editor) InsertAtCaret(md string) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	e.insertText(md)
}

// Caret returns the caret's offset in the source.
func (e *Editor) Caret() int { return e.Session.Selection().Head }

func runeStart(b byte) bool { return b&0xC0 != 0x80 }

// WindowPoint returns where the caret at source offset off shows in the
// window, at the middle of its line, as of the last frame: for an app's
// popover by the caret, or a test's click.
func (e *Editor) WindowPoint(off int) (x, y float32, ok bool) {
	r, ok := e.caretRect(off)
	if !ok {
		return 0, 0, false
	}
	return e.elemRect.X + r.X, e.elemRect.Y + r.Y + r.H/2, true
}

// setSessionSelection updates the Session-owned text selection. Callers keep
// table/visual structure state in their own subsystem and pass only the
// document range here.
func (e *Editor) setSessionSelection(anchor, head int) {
	selection := e.Session.Selection()
	selection.Anchor, selection.Head = anchor, head
	e.updateSessionSelection(selection)
}

func (e *Editor) setSessionAnchor(anchor int) {
	selection := e.Session.Selection()
	selection.Anchor = anchor
	e.updateSessionSelection(selection)
}

func (e *Editor) setSessionHead(head int) {
	selection := e.Session.Selection()
	selection.Head = head
	e.updateSessionSelection(selection)
}

func (e *Editor) setSessionSelectionMode(inclusive, linewise bool) {
	selection := e.Session.Selection()
	selection.Inclusive, selection.Linewise = inclusive, linewise
	e.updateSessionSelection(selection)
}

func (e *Editor) setSessionInclusive(inclusive bool) {
	selection := e.Session.Selection()
	selection.Inclusive = inclusive
	e.updateSessionSelection(selection)
}

func (e *Editor) setSessionLinewise(linewise bool) {
	selection := e.Session.Selection()
	selection.Linewise = linewise
	e.updateSessionSelection(selection)
}

func (e *Editor) updateSessionSelection(selection editormodel.Selection) {
	if e.editBatch != nil {
		_ = e.Session.UpdateTransactionSelection(e.editBatch, selection)
		return
	}
	_ = e.Session.SetSelection(selection)
}
