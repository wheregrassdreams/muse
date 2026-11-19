package editor

import (
	editorsource "github.com/wheregrassdreams/muse-editor/markdown/source"
	"github.com/wheregrassdreams/muse-editor/markdown/syntax"
	"github.com/wheregrassdreams/muse-editor/text/input"
	editormodel "github.com/wheregrassdreams/muse-editor/text/session"
)

// bindSessionAdapters connects the production presentation layer to the
// backend-neutral Session. The Session remains the only owner of source,
// selection, revision and undo history; callbacks here read state or provide
// block-aware effects only.
func (e *Editor) bindSessionAdapters() {
	e.Session.BindPresentationAdapter(editormodel.PresentationAdapter{
		DispatchKey: func(event input.KeyEvent) bool {
			if e.wikiSuggestionKey(event) {
				return true
			}
			if e.tableHandleDrag != nil {
				if event.Key == input.KeyEscape {
					e.finishTableHandleDrag(true)
				}
				return true
			}
			if e.modalKeyEvent(event) {
				if event.Key == input.KeyEscape {
					e.inlineMath.SetPreviewHidden(true)
				}
				return true
			}
			if e.key(event) {
				e.updateWikiSuggestions()
				return true
			}
			return false
		},
		ApplyOperation: e.applyOperation,
	})
	e.Session.SetChangeHook(func(change editormodel.Change) {
		e.changeMu.Lock()
		e.pendingChanges = append(e.pendingChanges, change)
		e.changeMu.Unlock()
		if e.opts.Invalidate != nil {
			e.opts.Invalidate()
		}
	})
}

// drainSessionChanges applies committed source deltas to the incremental
// parser. Changes are queued because Session invokes the hook after releasing
// its own lock, potentially while a UI callback already holds paintMu.
func (e *Editor) drainSessionChanges() {
	e.changeMu.Lock()
	changes := e.pendingChanges
	e.pendingChanges = nil
	e.changeMu.Unlock()
	if len(changes) == 0 {
		return
	}
	for _, change := range changes {
		if change.Kind == editormodel.ChangeSelection {
			continue
		}
		if change.Kind == editormodel.ChangeReplaceSource || e.parserRevision != change.BeforeRevision {
			e.resetParserForChange(e.Session.Source(), e.Session.Revision())
			continue
		}
		ok := true
		// Edits address the transaction's original source. Apply right to left
		// so replacing a later range cannot shift an earlier range.
		for i := len(change.Edits) - 1; i >= 0; i-- {
			edit := change.Edits[i]
			if edit.Range.Start < 0 || edit.Range.End < edit.Range.Start || edit.Range.End > len(e.p.Source()) {
				ok = false
				break
			}
			if !e.p.ApplyEdit(edit.Range.Start, edit.Range.End, edit.Text) {
				ok = false
				break
			}
		}
		if !ok {
			e.resetParserForChange(e.Session.Source(), e.Session.Revision())
			continue
		}
		e.parserRevision = change.Revision
	}
}

func (e *Editor) resetParserForSource(source string) {
	e.resetParserForChange(source, e.Session.Revision())
}

func (e *Editor) resetParserForChange(source string, revision uint64) {
	frontmatterEnd := editorsource.FrontmatterEnd(source)
	var imageInlines []syntax.IncrementalInline
	if frontmatterEnd == 0 {
		imageInlines, _ = syntax.LargeDataImageOnlySource(source)
	}
	e.p.Set(source, frontmatterEnd, imageInlines)
	e.pending, e.pendingMulti = nil, false
	e.parserRevision = revision
}
