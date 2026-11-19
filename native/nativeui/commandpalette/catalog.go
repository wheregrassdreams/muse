package commandpalette

import "muse/locales"

const (
	DocumentNew         CommandID = "document.new"
	DocumentSave        CommandID = "document.save"
	DocumentSaveAs      CommandID = "document.save-as"
	WorkspaceSearch     CommandID = "workspace.search"
	WorkspaceQuickOpen  CommandID = "workspace.quick-open"
	DocumentOpenFile    CommandID = "document.open-file"
	WorkspaceOpenFolder CommandID = "workspace.open-folder"
	WorkspaceNewFile    CommandID = "workspace.new-file"
	WorkspaceNewFolder  CommandID = "workspace.new-folder"
	PanelFiles          CommandID = "panel.files"
	PanelOutline        CommandID = "panel.outline"
	WorkspaceSearchAll  CommandID = "workspace.search-all"
	AppSettings         CommandID = "app.settings"
	PanelAssistant      CommandID = "panel.ai-assistant"
)

// Catalog returns the commands available in the current workbench state.
// Availability and localized labels are owned by the command-palette feature;
// the caller only supplies cross-feature capability facts.
func Catalog(assistantEnabled bool) []Command {
	commands := []Command{
		{ID: DocumentNew, Name: locales.DocumentNew(), Hint: "⌘N"},
		{ID: DocumentSave, Name: locales.MenuSave(), Hint: "⌘S"},
		{ID: DocumentSaveAs, Name: locales.CommonSaveAs(), Hint: "⇧⌘S"},
		{ID: WorkspaceSearch, Name: locales.MenuWorkspaceSearch(), Hint: "⌘O"},
		{ID: WorkspaceQuickOpen, Name: locales.WorkspaceQuickOpenPath()},
		{ID: DocumentOpenFile, Name: locales.CommonOpenFile()},
		{ID: WorkspaceOpenFolder, Name: locales.CommonOpenFolder(), Hint: "⇧⌘O"},
		{ID: WorkspaceNewFile, Name: locales.PaletteNewFile()},
		{ID: WorkspaceNewFolder, Name: locales.PaletteNewFolder()},
		{ID: PanelFiles, Name: locales.WorkspaceFilePanel()},
		{ID: PanelOutline, Name: locales.WorkspaceOutlinePanel()},
		{ID: WorkspaceSearchAll, Name: locales.MenuSearchWorkspace(), Hint: "⇧⌘F"},
		{ID: AppSettings, Name: locales.MenuSettings()},
	}
	if assistantEnabled {
		commands = append(commands, Command{ID: PanelAssistant, Name: locales.AiAssistantPanel()})
	}
	return commands
}
