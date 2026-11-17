// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

// paletteCommands projects app actions into the feature-owned command palette.
func (a *App) paletteCommands() []paletteCommand {
	toggle := func(p panelView) func() {
		return func() { a.togglePanel(p) }
	}
	cmds := []paletteCommand{
		{id: commandNewDocument, name: locales.DocumentNew(), hint: "⌘N", run: a.newDoc},
		{id: commandSave, name: locales.MenuSave(), hint: "⌘S", run: a.save},
		{id: commandSaveAs, name: locales.CommonSaveAs(), hint: "⇧⌘S", run: a.saveAs},
		{id: commandWorkspaceSearch, name: locales.MenuWorkspaceSearch(), hint: "⌘O", run: a.openWorkspaceSearch},
		{id: commandQuickOpen, name: locales.WorkspaceQuickOpenPath(), run: a.openQuickOpen},
		{id: commandOpenFile, name: locales.CommonOpenFile(), run: a.openFileDialog},
		{id: commandOpenFolder, name: locales.CommonOpenFolder(), hint: "⇧⌘O", run: a.openFolderDialog},
		{id: commandNewFile, name: locales.PaletteNewFile(), run: func() {
			a.newFileIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
		}},
		{id: commandNewFolder, name: locales.PaletteNewFolder(), run: func() {
			a.newFolderIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
		}},
		{id: commandFilePanel, name: locales.WorkspaceFilePanel(), run: toggle(panelFiles)},
		{id: commandOutlinePanel, name: locales.WorkspaceOutlinePanel(), run: toggle(panelOutline)},
		{id: commandSearchWorkspace, name: locales.MenuSearchWorkspace(), hint: "⇧⌘F", run: a.openWorkspaceSearch},
		{id: commandSettings, name: locales.MenuSettings(), run: a.openSettings},
	}
	if a.aiEnabled {
		cmds = append(cmds, paletteCommand{id: commandAIAssistant, name: locales.AiAssistantPanel(), run: toggle(panelAI)})
	}
	return cmds
}

func (a *App) paletteProjection() (commandpalette.ViewModel, map[commandpalette.CommandID]paletteCommand) {
	cmds := a.paletteCommands()
	items := make([]commandpalette.Command, len(cmds))
	byID := make(map[commandpalette.CommandID]paletteCommand, len(cmds))
	for i, cmd := range cmds {
		items[i] = commandpalette.Command{ID: cmd.id, Name: cmd.name, Hint: cmd.hint}
		byID[cmd.id] = cmd
	}
	return a.commandPalette.ViewModel(items), byID
}

func (a *App) paletteCandidates() []paletteCommand {
	model, byID := a.paletteProjection()
	out := make([]paletteCommand, 0, len(model.Commands))
	for _, match := range model.Commands {
		if cmd, ok := byID[match.ID]; ok {
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
	model, byID := a.paletteProjection()
	if len(model.Commands) == 0 {
		a.closeCommandPalette()
		return
	}
	selected := model.Commands[model.Selected]
	cmd, ok := byID[selected.ID]
	if !ok {
		a.closeCommandPalette()
		return
	}
	a.runPaletteCommand(cmd.id)
}

func (a *App) runPaletteCommand(id commandpalette.CommandID) {
	for _, cmd := range a.paletteCandidates() {
		if cmd.id == id {
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
		case commandpalette.QueryChanged, commandpalette.MoveSelection:
			a.invalidate()
		case commandpalette.Execute:
			a.runPaletteCommand(action.ID)
		case commandpalette.Dismiss:
			a.closeCommandPalette()
		}
	})
}

const (
	commandNewDocument     commandpalette.CommandID = "document.new"
	commandSave            commandpalette.CommandID = "document.save"
	commandSaveAs          commandpalette.CommandID = "document.save-as"
	commandWorkspaceSearch commandpalette.CommandID = "workspace.search"
	commandQuickOpen       commandpalette.CommandID = "workspace.quick-open"
	commandOpenFile        commandpalette.CommandID = "document.open-file"
	commandOpenFolder      commandpalette.CommandID = "workspace.open-folder"
	commandNewFile         commandpalette.CommandID = "workspace.new-file"
	commandNewFolder       commandpalette.CommandID = "workspace.new-folder"
	commandFilePanel       commandpalette.CommandID = "panel.files"
	commandOutlinePanel    commandpalette.CommandID = "panel.outline"
	commandSearchWorkspace commandpalette.CommandID = "workspace.search-all"
	commandSettings        commandpalette.CommandID = "app.settings"
	commandAIAssistant     commandpalette.CommandID = "panel.ai-assistant"
)

type paletteCommand struct {
	id   commandpalette.CommandID
	name string
	hint string
	run  func()
}
