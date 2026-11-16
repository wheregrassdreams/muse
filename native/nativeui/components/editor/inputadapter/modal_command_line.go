package inputadapter

import (
	"muse/editor/input/vim"
	"muse/locales"

	"github.com/egoist/mygo/ui"
)

// CommandLineHost supplies the editor-owned effects needed by the Vim search
// prompt. The adapter owns only MyGo presentation and text event translation.
type CommandLineHost interface {
	VimViewState() vim.ViewState
	CommandLineInset() float32
	SetSearchInputFocused(bool)
	UpdateSearchQuery(string)
	SubmitSearch()
	CancelSearch()
}

// ModalCommandLineView renders the search prompt from a detached Vim snapshot.
func ModalCommandLineView(c *ui.Context, host CommandLineHost) {
	state := host.VimViewState()
	if !state.Searching {
		host.SetSearchInputFocused(false)
		return
	}
	t := c.Theme()
	prompt := "/"
	if state.SearchDirection < 0 {
		prompt = "?"
	}
	query := state.SearchQuery
	ui.Row(c).Height(38).Shrink(0).Margin(0, host.CommandLineInset()).Padding(0, 10).Gap(7).
		AlignItems(ui.Center).Radius(8).Background(t.SurfaceHover).
		Border(1, t.Border).Children(func() {
		ui.Text(c, prompt).FontSize(12.5).FontWeight(620).TextColor(t.TextMuted)
		input := ui.TextInputBase(c, &query).Grow(1).MinWidth(0).
			AutoFocus().FontSize(12.5).TextColor(t.Text).
			Background(t.SurfaceHover).Label(locales.EditorVimSearch()).
			OnSubmit(host.SubmitSearch).
			OnChange(func() { host.UpdateSearchQuery(query) }).
			OnTextKey(func(k ui.TextKey) bool {
				if k.Key == ui.KeyEscape && k.Mods == 0 {
					host.CancelSearch()
					return true
				}
				return false
			})
		host.SetSearchInputFocused(input.Focused())
	})
}
