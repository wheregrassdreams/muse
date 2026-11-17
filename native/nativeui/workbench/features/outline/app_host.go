// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (a *App) outlineView(c *ui.Context) {
	if a.noteEditor() == nil {
		return
	}
	parsedHeadings := a.noteEditor().NavigationCommands().Headings()
	headings := make([]outlineui.Heading, 0, len(parsedHeadings))
	for _, heading := range parsedHeadings {
		headings = append(headings, outlineui.Heading{Level: heading.Level, Text: heading.Text, Offset: heading.Offset})
	}
	a.outlinePanel.SyncActiveHeading(headings, a.noteEditor().NavigationCommands().Caret())
	a.outlinePanel.Render(c, outlineui.ViewModel{Headings: headings}, func(action outlineui.Action) {
		if action.Kind != outlineui.SelectHeading {
			return
		}
		a.noteEditor().NavigationCommands().NavigateToOffset(action.Offset)
		a.noteEditor().FocusCommands().Focus()
	})
}
