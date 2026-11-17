// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (rows appQuickOpenRows) Len() int                     { return len(rows) }

func (rows appQuickOpenRows) At(index int) quickopenui.Row { return rows[index] }

func (source appQuickOpenCandidates) Rows(query string) quickopenui.Rows {
	a := source.app
	if a == nil || a.vault == nil || !a.vaultSnapshot.Ready {
		return appQuickOpenRows(nil)
	}
	matches := a.vault.QuickOpen(query)
	rows := make(appQuickOpenRows, 0, len(matches))
	for _, match := range matches {
		rows = append(rows, quickopenui.Row{
			Path: match.Path, Title: match.Title, PathText: match.PathText,
			TitleMatches: match.TitleMatches, PathMatches: match.PathMatches,
		})
	}
	return rows
}

func (source appQuickOpenCandidates) Revision() quickopenui.Revision {
	if source.app == nil {
		return quickopenui.Revision{}
	}
	return quickopenui.Revision{Catalog: source.app.vaultSnapshot.FilesRevision, Index: source.app.vaultSnapshot.IndexRevision}
}

func (a *App) openQuickOpen() {
	if a == nil {
		return
	}
	if a.root == "" {
		a.openFileDialog()
		return
	}
	if a.quickOpen == nil {
		a.notice(quickopenui.ErrWindowHostUnavailable.Error())
		return
	}
	if err := a.quickOpen.Open(); err != nil {
		a.notice(err.Error())
	}
}

func (a *App) closeQuickOpen() {
	if a != nil && a.quickOpen != nil {
		_ = a.quickOpen.Close()
	}
}

func (a *App) quickOpenOpen() bool {
	return a != nil && a.quickOpen != nil && a.quickOpen.IsOpen()
}

type appQuickOpenRows []quickopenui.Row

type appQuickOpenCandidates struct{ app *App }
