package nativeui

import (
	"github.com/egoist/mygo/ui"
	"muse/nativeui/surfaces/quickopen"
	"muse/nativeui/window"
)

type quickOpenCandidate struct {
	fileRow
	title        string
	pathText     string
	titleMatches []int
	pathMatches  []int
}

type quickOpenRows []quickOpenCandidate

func (rows quickOpenRows) Len() int { return len(rows) }
func (rows quickOpenRows) At(i int) quickopen.Row {
	item := rows[i]
	return quickopen.Row{Path: item.path, Title: item.title, PathText: item.pathText, TitleMatches: item.titleMatches, PathMatches: item.pathMatches}
}

func (a *App) quickOpenCandidates(queries ...string) []quickOpenCandidate {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		return nil
	}
	query := ""
	if len(queries) > 0 {
		query = queries[0]
	}
	matches := a.vault.QuickOpen(query)
	rows := make([]quickOpenCandidate, 0, len(matches))
	for _, match := range matches {
		rows = append(rows, quickOpenCandidate{
			fileRow: fileRow{path: match.Path}, title: match.Title, pathText: match.PathText,
			titleMatches: match.TitleMatches, pathMatches: match.PathMatches,
		})
	}
	return rows
}

// openQuickOpen shows the palette, or the system file dialog when no
// workspace is open.
func (a *App) openQuickOpen() {
	if a.root == "" {
		a.openFileDialog()
		return
	}
	a.closeQuickOpen()
	content := quickopen.NewContent(quickopen.SurfaceOptions{
		Rows:     func(query string) quickopen.Rows { return quickOpenRows(a.quickOpenCandidates(query)) },
		FileIcon: iconFile,
		IconTint: func(theme *ui.Theme) ui.Color { return iconTint(theme) },
		OnOpen: func(path string) {
			a.selectedPath = path
			a.requestOpen(path)
		},
		OnClose: func() { a.quickOpenHandle = window.Handle{} },
	})
	handle, err := a.windows.Open(content, window.Placement{Host: "main", Layer: window.ModalOverlay})
	if err != nil {
		a.notice(err.Error())
		return
	}
	a.quickOpenHandle = handle
	a.invalidate()
}

func (a *App) closeQuickOpen() {
	if a.windows != nil && !a.quickOpenHandle.IsZero() {
		_ = a.windows.Close(a.quickOpenHandle)
	}
	a.quickOpenHandle = window.Handle{}
	a.invalidate()
}

func (a *App) quickOpenOpen() bool {
	return a.windows != nil && a.windows.Contains(a.quickOpenHandle)
}
