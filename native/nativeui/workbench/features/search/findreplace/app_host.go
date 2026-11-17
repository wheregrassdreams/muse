// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (a *App) documentSearchView(c *ui.Context) {
	a.refreshSearch()
	findreplaceui.Render(c, a.findReplace, a.dispatchFindReplaceAction)
}

func (a *App) dispatchFindReplaceAction(action findreplaceui.Action) {
	switch action.Kind {
	case findreplaceui.QueryChanged:
		a.refreshSearch()
		a.invalidate()
	case findreplaceui.ReplacementChanged:
		// The panel owns this text state; no document edit occurs yet.
	case findreplaceui.OpenMatch:
		a.findReplace.Apply(action)
		a.goToHit(action.Index)
	case findreplaceui.ReplaceOne:
		a.replaceHit()
	case findreplaceui.ReplaceAll:
		a.replaceAllHits()
	}
}

// refreshSearch finds the query in the document again when either changed,
// and shows the matches in the editor.
func (a *App) refreshSearch() {
	if a.noteEditor() == nil || !a.findReplace.Sync(a.documentSource()) {
		return
	}
	a.syncFindReplaceHighlights()
}

func (a *App) syncFindReplaceHighlights() {
	if a.noteEditor() == nil {
		return
	}
	snapshot := a.findReplace.Snapshot()
	matches := snapshot.Matches
	ranges := make([]noteeditor.TextRange, len(matches))
	for i, h := range matches {
		ranges[i] = noteeditor.TextRange{Start: h.Start, End: h.End}
	}
	a.noteEditor().SearchCommands().SetHighlights(ranges, snapshot.Selected)
}

// goToHit selects match i in the editor.
func (a *App) goToHit(i int) {
	snapshot := a.findReplace.Snapshot()
	matches := snapshot.Matches
	if i < 0 || i >= len(matches) {
		return
	}
	a.findReplace.Apply(findreplaceui.Action{Kind: findreplaceui.OpenMatch, Index: i})
	h := matches[i]
	ranges := make([]noteeditor.TextRange, len(matches))
	for k, x := range matches {
		ranges[k] = noteeditor.TextRange{Start: x.Start, End: x.End}
	}
	a.noteEditor().SearchCommands().SetHighlights(ranges, i)
	a.noteEditor().NavigationCommands().NavigateToRange(h.Start, h.End)
	a.noteEditor().FocusCommands().Focus()
}

// replaceHit replaces the chosen match, or the first, and goes to the next.
func (a *App) replaceHit() {
	snapshot := a.findReplace.Snapshot()
	matches := snapshot.Matches
	i := max(snapshot.Selected, 0)
	if i >= len(matches) {
		return
	}
	h := matches[i]
	_ = a.noteEditor().Replace(h.Start, h.End, snapshot.Replacement)
	a.refreshSearch()
	if count := len(a.findReplace.Snapshot().Matches); count > 0 {
		a.goToHit(min(i, count-1))
	}
}

// replaceAllHits replaces every match, as one step of undo.
func (a *App) replaceAllHits() {
	snapshot := a.findReplace.Snapshot()
	matches := snapshot.Matches
	if len(matches) == 0 {
		return
	}
	source := a.documentSource()
	replaced, n := findreplace.ReplaceAll(source, matches, snapshot.Replacement)
	if n == 0 {
		return
	}
	a.editDoc(replaced)
	a.refreshSearch()
	a.notice(locales.CommonReplaceSuccess(n))
}
