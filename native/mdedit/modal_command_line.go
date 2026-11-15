package mdedit

import (
	"muse/locales"

	"github.com/egoist/mygo/ui"
)

// modalCommandLineView is a native editor child, separate from both the
// document text surface and the application's mode label. TextInputBase owns
// query editing, caret placement, and IME composition; Editor only owns the
// search lifecycle and source navigation.
func (e *Editor) modalCommandLineView(c *ui.Context) {
	if !e.modal.searching {
		e.modal.searchInputFocused = false
		return
	}
	t := c.Theme()
	prompt := "/"
	if e.modal.searchDir < 0 {
		prompt = "?"
	}
	ui.Row(c).Height(38).Shrink(0).Margin(0, e.colX).Padding(0, 10).Gap(7).
		AlignItems(ui.Center).Radius(8).Background(t.SurfaceHover).
		Border(1, t.Border).Children(func() {
		ui.Text(c, prompt).FontSize(12.5).FontWeight(620).TextColor(t.TextMuted)
		input := ui.TextInputBase(c, &e.modal.searchQuery).Grow(1).MinWidth(0).
			AutoFocus().FontSize(12.5).TextColor(t.Text).
			Background(t.SurfaceHover).Label(locales.EditorVimSearch()).
			OnSubmit(func() { e.ApplyOperation(EditorOperation{Kind: OperationSubmitSearch}) }).
			OnChange(func() { e.modalRefreshSearchPreview() }).
			OnTextKey(func(k ui.TextKey) bool {
				if k.Key == ui.KeyEscape && k.Mods == 0 {
					e.ApplyOperation(EditorOperation{Kind: OperationCancelSearch})
					return true
				}
				return false
			})
		e.modal.searchInputFocused = input.Focused()
	})
}
