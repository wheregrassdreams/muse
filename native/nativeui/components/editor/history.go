package editor

import (
	"fmt"
	"time"

	editormodel "muse/editor"
)

// stepKind classifies transactions at the editor boundary. The actual
// transaction, undo stack, coalescing decision and source selection all belong
// to Session.
type stepKind uint8

const (
	stepEdit stepKind = iota
	stepTyping
	stepDelete
)

func historyGroup(kind stepKind) editormodel.HistoryGroup {
	switch kind {
	case stepTyping:
		return editormodel.HistoryGroupTyping
	case stepDelete:
		return editormodel.HistoryGroupDelete
	default:
		return editormodel.HistoryGroupExplicit
	}
}

func (e *Editor) begin(kind stepKind) { e.beginWithMerge(kind, false) }

func (e *Editor) beginMergePrevious(kind stepKind) { e.beginWithMerge(kind, true) }

func (e *Editor) beginWithMerge(kind stepKind, mergePrevious bool) {
	if e.editBatch != nil {
		return
	}
	batch, err := e.Session.BeginTransaction(editormodel.TransactionOptions{
		Group: historyGroup(kind), At: time.Now(), MergePrevious: mergePrevious,
		BeforeMetadata: e.Session.CaptureHistory(),
	})
	if err != nil {
		panic(fmt.Errorf("begin editor transaction: %w", err))
	}
	e.editBatch = batch
}

// replace applies an edit immediately through the Session-owned batch so
// commands can continue reading their intermediate source without keeping a
// second component-owned source buffer.
func (e *Editor) replace(from, to int, text string) {
	source := e.Source()
	from = max(0, min(from, len(source)))
	to = max(from, min(to, len(source)))
	old := source[from:to]
	if old == text {
		return
	}
	e.clearTableVirtualEdit()
	e.feedbackState.CancelYankFlash()
	e.shiftImageAlignments(from, to, len(text))
	if e.editBatch == nil {
		e.begin(stepEdit)
	}
	e.inlineMath.AdjustAfterEdit(from, to, len(text))
	if _, err := e.Session.ApplyEdit(e.editBatch, editormodel.TextEdit{
		Range: editormodel.TextRange{Start: from, End: to}, Text: text,
	}); err != nil {
		e.abortEditBatch()
		panic(fmt.Errorf("apply editor source edit [%d,%d): %w", from, to, err))
	}
	e.resetFolds()
	e.moveTypedBreaks(from, to, text)
	e.parsed = invalidParsedRevision
	after := e.Source()
	e.noteEdit(from, to, old, text, after)
	if e.pending == nil {
		e.pending = &pendingEdit{from: from, to: to, text: text}
	} else {
		e.pendingMulti = true
	}
	shift := func(p int) int {
		switch {
		case p >= to:
			return p + len(text) - (to - from)
		case p > from:
			return from + len(text)
		}
		return p
	}
	for i := range e.highlights {
		e.highlights[i].Start, e.highlights[i].End = shift(e.highlights[i].Start), shift(e.highlights[i].End)
	}
	for i := range e.vimHighlights {
		e.vimHighlights[i].Start, e.vimHighlights[i].End = shift(e.vimHighlights[i].Start), shift(e.vimHighlights[i].End)
	}
	for i := range e.sourceRevealRanges {
		e.sourceRevealRanges[i].Start, e.sourceRevealRanges[i].End = shift(e.sourceRevealRanges[i].Start), shift(e.sourceRevealRanges[i].End)
	}
}

func (e *Editor) commit(anchor, caret int) {
	e.clearVimImageRow()
	e.resetTableNavigation()
	e.tableSelection = nil
	e.tableHandleDrag = nil
	e.tableBridge = nil
	e.tableDocumentRegion = nil
	selection := e.Session.Selection()
	selection.Anchor, selection.Head = anchor, caret
	selection.Inclusive, selection.Linewise = false, false
	// A structural delete can remove the entire document while the command's
	// legacy caret target still refers to the old source. Clamp before parsing
	// or committing so an empty-document selection remains valid.
	sourceLength := len(e.Source())
	selection.Anchor = max(0, min(selection.Anchor, sourceLength))
	selection.Head = max(0, min(selection.Head, sourceLength))
	if e.phantom != nil && e.Session.Revision() != e.parsed {
		if !(caret == e.phantom.at && e.phantomStays) {
			e.phantom = nil
		}
	}
	e.phantomStays = false
	e.parsed = invalidParsedRevision
	e.sync()
	selection.Anchor = e.normalize(selection.Anchor, -1)
	selection.Head = e.normalize(selection.Head, 1)
	if anchor == caret {
		selection.Anchor = selection.Head
	}
	e.wantReveal = true
	e.hasGoal = false
	e.clearComposition()
	if e.editBatch != nil {
		batch := e.editBatch
		e.editBatch = nil
		result, err := e.Session.CommitTransaction(batch, selection)
		if err != nil {
			// CommitTransaction aborts and unlocks the batch when final selection
			// validation fails. Do not try to abort that closed batch a second
			// time; the Session remains the sole batch lifecycle owner.
			e.pending, e.pendingMulti = nil, false
			e.parsed = invalidParsedRevision
			panic(fmt.Errorf("commit editor transaction: %w", err))
		}
		if result.Changed {
			e.changed()
		}
	} else {
		_ = e.Session.SetSelection(e.Session.Selection())
	}
}

func (e *Editor) abortEditBatch() {
	if e.editBatch == nil {
		return
	}
	batch := e.editBatch
	e.editBatch = nil
	if err := e.Session.AbortTransaction(batch); err != nil {
		panic(fmt.Errorf("abort editor transaction: %w", err))
	}
	e.pending, e.pendingMulti = nil, false
	e.parsed = invalidParsedRevision
}

func (e *Editor) mergeLastStep() {
	// The corresponding batch was opened with MergePrevious. This marker is
	// retained at the old call site to make the structural merge explicit.
}

func (e *Editor) breakCoalescing() { e.Session.CloseHistoryGroup() }

func (e *Editor) changed() {
	e.lastChange = time.Now()
	if e.opts.OnChange != nil {
		e.opts.OnChange()
	}
}

func (e *Editor) Undo() bool {
	e.vimController().SetTextVisualActive(false)
	if e.editBatch != nil {
		e.commit(e.Caret(), e.Caret())
	}
	e.inlineMath.Clear()
	result, err := e.Session.Undo()
	if err != nil || !result.Changed {
		return false
	}
	e.parsed = invalidParsedRevision
	e.pending, e.pendingMulti = nil, false
	e.sync()
	e.changed()
	e.invalidate()
	return true
}

func (e *Editor) Redo() bool {
	e.vimController().SetTextVisualActive(false)
	if e.editBatch != nil {
		e.commit(e.Caret(), e.Caret())
	}
	e.inlineMath.Clear()
	result, err := e.Session.Redo()
	if err != nil || !result.Changed {
		return false
	}
	e.parsed = invalidParsedRevision
	e.pending, e.pendingMulti = nil, false
	e.sync()
	e.changed()
	e.invalidate()
	return true
}

func (e *Editor) CanUndo() bool { return e.Session.CanUndo() }
func (e *Editor) CanRedo() bool { return e.Session.CanRedo() }
