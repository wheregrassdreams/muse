package nativeui

import (
	"muse/locales"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Command palette (⌘P): every menu action, searchable. Shares the quick
// open overlay's shape (fuzzy filter, ↑↓ selection, ↵ run, Esc dismiss).

type paletteCommand struct {
	name string
	hint string
	run  func()
}

// paletteCommands lists the palette's actions: the file menu, panel switches
// and appearance — the commands a writer reaches for without hunting menus.
func (a *App) paletteCommands() []paletteCommand {
	toggle := func(p panelView) func() {
		return func() { a.togglePanel(p) }
	}
	cmds := []paletteCommand{
		{name: locales.DocumentNew(), hint: "⌘N", run: a.newDoc},
		{name: locales.MenuSave(), hint: "⌘S", run: a.save},
		{name: locales.CommonSaveAs(), hint: "⇧⌘S", run: a.saveAs},
		{name: locales.MenuWorkspaceSearch(), hint: "⌘O", run: a.openWorkspaceSearch},
		{name: locales.WorkspaceQuickOpenPath(), run: a.openQuickOpen},
		{name: locales.CommonOpenFile(), hint: "", run: a.openFileDialog},
		{name: locales.CommonOpenFolder(), hint: "⇧⌘O", run: a.openFolderDialog},
		{name: locales.PaletteNewFile(), hint: "", run: func() { a.newFileIn(a.createTarget()) }},
		{name: locales.PaletteNewFolder(), hint: "", run: func() { a.newFolderIn(a.createTarget()) }},
		{name: locales.WorkspaceFilePanel(), hint: "", run: toggle(panelFiles)},
		{name: locales.WorkspaceOutlinePanel(), hint: "", run: toggle(panelOutline)},
		{name: locales.MenuSearchWorkspace(), hint: "⇧⌘F", run: a.openWorkspaceSearch},
		{name: locales.MenuSettings(), hint: "", run: a.openSettings},
	}
	if a.aiEnabled {
		cmds = append(cmds, paletteCommand{name: locales.AiAssistantPanel(), run: toggle(panelAI)})
	}
	return cmds
}

func (a *App) paletteCandidates() []paletteCommand {
	cmds := a.paletteCommands()
	q := strings.ToLower(strings.TrimSpace(a.cmdValue))
	if q == "" {
		return cmds
	}
	type scored struct {
		cmd   paletteCommand
		score int
	}
	matches := make([]scored, 0, 8)
	for _, c := range cmds {
		if s, ok := fuzzyScore(strings.ToLower(c.name), q); ok {
			matches = append(matches, scored{c, s})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		return matches[i].cmd.name < matches[j].cmd.name
	})
	out := make([]paletteCommand, len(matches))
	for i, m := range matches {
		out[i] = m.cmd
	}
	return out
}

func (a *App) openCommandPalette() {
	a.cmdPalette = true
	a.cmdValue = ""
	a.cmdSel = 0
	a.invalidate()
}

func (a *App) closeCommandPalette() {
	a.cmdPalette = false
	a.invalidate()
}

// applyCommandPalette runs the selected action.
func (a *App) applyCommandPalette() {
	cands := a.paletteCandidates()
	if len(cands) == 0 {
		a.closeCommandPalette()
		return
	}
	if a.cmdSel >= len(cands) {
		a.cmdSel = 0
	}
	if a.cmdSel < 0 {
		a.cmdSel = len(cands) - 1
	}
	cmd := cands[a.cmdSel]
	a.cmdPalette = false
	a.invalidate()
	cmd.run()
}

func (a *App) commandPaletteDialog(c *ui.Context) {
	if !a.cmdPalette {
		return
	}
	if c.Shortcut(0, ui.KeyEscape) {
		a.closeCommandPalette()
		return
	}
	if c.Shortcut(0, ui.KeyUp) {
		if a.cmdSel > 0 {
			a.cmdSel--
		}
	}
	if c.Shortcut(0, ui.KeyDown) {
		if a.cmdSel < len(a.paletteCandidates())-1 {
			a.cmdSel++
		}
	}
	t := c.Theme()
	ui.Box(c).Fill().Background(ui.RGBA(0, 0, 0, 0.32)).Children(func() {
		ui.Column(c).Width(560).Margin(110, ui.Auto, 0, ui.Auto).Radius(12).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 12, 32, -6, ui.RGBA(0, 0, 0, 0.28)).
			Padding(10).Gap(8).Children(func() {
			ui.TextInput(c, &a.cmdValue).Placeholder(locales.CommandPalettePlaceholder()).Label(locales.CommandPaletteTitle()).
				AutoFocus().OnSubmit(func() { a.applyCommandPalette() })
			cands := a.paletteCandidates()
			if len(cands) == 0 {
				ui.Text(c, locales.CommandPaletteNoMatch()).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				return
			}
			if a.cmdSel >= len(cands) {
				a.cmdSel = len(cands) - 1
			}
			if a.cmdSel < 0 {
				a.cmdSel = 0
			}
			n := len(cands)
			if n > 12 {
				n = 12
			}
			for i := 0; i < n; i++ {
				cmd := cands[i]
				i := i
				row := ui.Row(c).Height(30).Radius(6).Padding(0, 10).Gap(8).
					AlignItems(ui.Center).Cursor(ui.CursorPointer)
				switch {
				case i == a.cmdSel:
					if t.Dark {
						row.Background(ui.RGBA(107, 147, 255, 0.30))
					} else {
						row.Background(ui.RGBA(107, 147, 255, 0.20))
					}
				case row.Hovered():
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					ui.Text(c, cmd.name).FontSize(12.5).TextColor(t.Text).SingleLine().Ellipsis("…")
					if cmd.hint != "" {
						ui.Spacer(c)
						ui.Text(c, cmd.hint).FontSize(10.5).TextColor(t.TextMuted)
					}
				})
				row.OnClick(func() {
					a.cmdSel = i
					a.applyCommandPalette()
				})
			}
			ui.Row(c).Height(18).Padding(0, 2).Children(func() {
				ui.Text(c, locales.CommandPaletteCount(len(cands))).
					FontSize(10).TextColor(t.TextMuted)
			})
		})
	})
}
