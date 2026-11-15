package nativeui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/nativeui/panels/commandpalette"
	vaultsearch "muse/search"
)

type paletteCommand struct {
	name string
	hint string
	run  func()
}

// paletteCommands projects the app's currently available actions. The panel
// receives names and IDs only; command effects stay in this root adapter.
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
		{name: locales.CommonOpenFile(), run: a.openFileDialog},
		{name: locales.CommonOpenFolder(), hint: "⇧⌘O", run: a.openFolderDialog},
		{name: locales.PaletteNewFile(), run: func() { a.newFileIn(a.createTarget()) }},
		{name: locales.PaletteNewFolder(), run: func() { a.newFolderIn(a.createTarget()) }},
		{name: locales.WorkspaceFilePanel(), run: toggle(panelFiles)},
		{name: locales.WorkspaceOutlinePanel(), run: toggle(panelOutline)},
		{name: locales.MenuSearchWorkspace(), hint: "⇧⌘F", run: a.openWorkspaceSearch},
		{name: locales.MenuSettings(), run: a.openSettings},
	}
	if a.aiEnabled {
		cmds = append(cmds, paletteCommand{name: locales.AiAssistantPanel(), run: toggle(panelAI)})
	}
	return cmds
}

func (a *App) paletteCandidates() []paletteCommand {
	cmds := a.paletteCommands()
	query := strings.TrimSpace(a.cmdValue)
	if query == "" {
		return cmds
	}
	items := make([]vaultsearch.QuickOpenItem, len(cmds))
	byName := make(map[string]paletteCommand, len(cmds))
	for i, cmd := range cmds {
		items[i] = vaultsearch.QuickOpenItem{Path: cmd.name, PathText: cmd.name, Title: cmd.name}
		byName[cmd.name] = cmd
	}
	matches := vaultsearch.RankQuickOpen(query, items)
	out := make([]paletteCommand, 0, len(matches))
	for _, match := range matches {
		if cmd, ok := byName[match.Path]; ok {
			out = append(out, cmd)
		}
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

func (a *App) applyCommandPalette() {
	commands := a.paletteCandidates()
	if len(commands) == 0 {
		a.closeCommandPalette()
		return
	}
	index := max(0, min(a.cmdSel, len(commands)-1))
	a.runPaletteCommand(commands[index].name)
}

func (a *App) runPaletteCommand(id string) {
	for _, cmd := range a.paletteCandidates() {
		if cmd.name == id {
			a.cmdPalette = false
			a.invalidate()
			cmd.run()
			return
		}
	}
	a.closeCommandPalette()
}

func (a *App) commandPaletteDialog(c *ui.Context) {
	if !a.cmdPalette {
		return
	}
	commands := a.paletteCandidates()
	viewCommands := make([]commandpalette.Command, len(commands))
	for i, cmd := range commands {
		viewCommands[i] = commandpalette.Command{ID: cmd.name, Name: cmd.name, Hint: cmd.hint}
	}
	model := commandpalette.ViewModel{Query: a.cmdValue, Selected: a.cmdSel, Commands: viewCommands}
	commandpalette.Render(c, model, func(action commandpalette.Action) {
		switch action.Kind {
		case commandpalette.QueryChanged:
			a.cmdValue, a.cmdSel = action.Query, 0
			a.invalidate()
		case commandpalette.MoveSelection:
			a.cmdSel = action.Index
			a.invalidate()
		case commandpalette.Execute:
			a.runPaletteCommand(action.ID)
		case commandpalette.Dismiss:
			a.closeCommandPalette()
		}
	})
}
