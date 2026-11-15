package editor

import (
	"fmt"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"muse/markdown"
	"muse/markdown/syntax"
)

// bindSessionAdapters connects neutral session actions to the production
// editor's existing incremental input/history path. The session owns all
// document state; these callbacks only supply editor-specific effects such as
// the incremental parser, table controller and viewport invalidation.
func (e *Editor) bindSessionAdapters() {
	e.Session.dispatchInputFn = e.dispatchSessionInput
	e.Session.applyTransactionFn = func(tx Transaction) (DispatchResult, error) {
		e.paintMu.Lock()
		defer e.paintMu.Unlock()
		before := e.rev
		if err := e.ApplyTransaction(tx); err != nil {
			return DispatchResult{}, err
		}
		return DispatchResult{
			Changed:  e.rev != before,
			Revision: e.rev,
		}, nil
	}
	e.Session.setSelectionFn = func(selection Selection) error {
		e.Select(selection.Anchor, selection.Head)
		e.selectionInclusive = selection.Inclusive
		e.selectionLinewise = selection.Linewise
		return nil
	}
	e.Session.applyOperationFn = e.applyOperation
	e.Session.replaceSourceFn = e.SetSource
	e.Session.sourceFn = e.sessionSource
	e.Session.revisionFn = e.sessionRevision
	e.Session.selectionFn = e.sessionSelection
	e.Session.frameFn = e.sessionFrame
}

func (e *Editor) dispatchSessionInput(event InputEvent) (DispatchResult, error) {
	attachedLock := event.Kind != InputSetSelection
	if attachedLock {
		e.paintMu.Lock()
		defer e.paintMu.Unlock()
	}
	beforeSource, beforeSelection := e.src, e.Session.selectionLocked()
	if !attachedLock {
		beforeSource, beforeSelection = e.sessionSource(), e.sessionSelection()
	}
	switch event.Kind {
	case InputInsertText:
		if !utf8.ValidString(event.Text) {
			return DispatchResult{}, fmt.Errorf("inserted text is not valid UTF-8")
		}
		e.typeText(event.Text)
	case InputDeleteBackward:
		e.key(ui.KeyBackspace, 0)
	case InputDeleteForward:
		e.key(ui.KeyDelete, 0)
	case InputMoveHorizontal:
		if event.Direction != -1 && event.Direction != 1 {
			return DispatchResult{}, fmt.Errorf("horizontal direction must be -1 or +1")
		}
		e.ApplyOperation(EditorOperation{Kind: OperationMoveHorizontal, Direction: event.Direction, Extend: event.Extend})
	case InputSetSelection:
		selection := Selection{Anchor: event.Anchor, Head: event.Head, Inclusive: event.Inclusive, Linewise: event.Linewise}
		if err := validateSelection(e.src, selection); err != nil {
			return DispatchResult{}, err
		}
		e.Select(selection.Anchor, selection.Head)
		e.selectionInclusive, e.selectionLinewise = selection.Inclusive, selection.Linewise
	case InputUndo:
		e.Undo()
	case InputRedo:
		e.Redo()
	default:
		return DispatchResult{}, fmt.Errorf("unsupported input kind %d", event.Kind)
	}
	afterSelection := e.Session.selectionLocked()
	if !attachedLock {
		afterSelection = e.sessionSelection()
	}
	return DispatchResult{
		Changed:          e.src != beforeSource,
		SelectionChanged: afterSelection != beforeSelection,
		Revision:         e.rev,
	}, nil
}

func (e *Editor) sessionSource() string {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	return e.src
}

func (e *Editor) sessionRevision() uint64 {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	return e.rev
}

func (e *Editor) sessionSelection() Selection {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	return e.Session.selectionLocked()
}

func (e *Editor) sessionFrame() Frame {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	e.sync()
	if e.Session.frame.Document == nil || e.Session.frame.Revision != e.rev || e.Session.frame.Source != e.src {
		doc := syntax.FromIncremental(e.p.doc)
		if doc == nil {
			return Frame{Revision: e.rev, Source: e.src, Selection: e.Session.selectionLocked()}
		}
		doc.Source = e.src
		parsed := markdown.Project(doc)
		e.Session.frame = Frame{Revision: e.rev, Source: e.src, Document: parsed, Rendered: parsed.RenderDocument()}
	}
	e.Session.frame.Selection = e.Session.selectionLocked()
	return e.Session.frame.snapshot()
}
