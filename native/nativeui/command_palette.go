package nativeui

import (
	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/nativeui/panels/commandpalette"
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
		{name: locales.PaletteNewFile(), run: func() {
			a.newFileIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
		}},
		{name: locales.PaletteNewFolder(), run: func() {
			a.newFolderIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
		}},
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

func (a *App) paletteProjection() (commandpalette.ViewModel, map[string]paletteCommand) {
	cmds := a.paletteCommands()
	items := make([]commandpalette.Command, len(cmds))
	byName := make(map[string]paletteCommand, len(cmds))
	for i, cmd := range cmds {
		items[i] = commandpalette.Command{ID: cmd.name, Name: cmd.name, Hint: cmd.hint}
		byName[cmd.name] = cmd
	}
	return a.commandPalette.ViewModel(items), byName
}

func (a *App) paletteCandidates() []paletteCommand {
	model, byName := a.paletteProjection()
	out := make([]paletteCommand, 0, len(model.Commands))
	for _, match := range model.Commands {
		if cmd, ok := byName[match.ID]; ok {
			out = append(out, cmd)
		}
	}
	return out
}

func (a *App) openCommandPalette() {
	a.commandPalette.Open()
	a.invalidate()
}

func (a *App) closeCommandPalette() {
	a.commandPalette.Close()
	a.invalidate()
}

func (a *App) applyCommandPalette() {
	model, byName := a.paletteProjection()
	if len(model.Commands) == 0 {
		a.closeCommandPalette()
		return
	}
	selected := model.Commands[model.Selected]
	cmd, ok := byName[selected.ID]
	if !ok {
		a.closeCommandPalette()
		return
	}
	a.runPaletteCommand(cmd.name)
}

func (a *App) runPaletteCommand(id string) {
	for _, cmd := range a.paletteCandidates() {
		if cmd.name == id {
			a.commandPalette.Close()
			a.invalidate()
			cmd.run()
			return
		}
	}
	a.closeCommandPalette()
}

func (a *App) commandPaletteDialog(c *ui.Context) {
	if !a.commandPalette.IsOpen() {
		return
	}
	model, _ := a.paletteProjection()
	commandpalette.Render(c, model, func(action commandpalette.Action) {
		a.commandPalette.Apply(action)
		switch action.Kind {
		case commandpalette.QueryChanged:
			a.invalidate()
		case commandpalette.MoveSelection:
			a.invalidate()
		case commandpalette.Execute:
			a.runPaletteCommand(action.ID)
		case commandpalette.Dismiss:
			a.closeCommandPalette()
		}
	})
}
