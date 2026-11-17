package nativeui

import "muse/nativeui/surfaces/noteeditor"

// noteEditor resolves a short lived feature capability from the generic pane
// handle. The App keeps only the handle; noteeditor owns the type assertion.
// Callers run on the UI-owner thread, as required by window.Manager.
func (a *App) noteEditor() noteeditor.Commands {
	if a == nil || a.windows == nil || a.editorPaneHandle.IsZero() {
		return nil
	}
	var commands noteeditor.Commands
	if err := noteeditor.WithCommands(a.windows, a.editorPaneHandle, func(value noteeditor.Commands) error {
		commands = value
		return nil
	}); err != nil {
		return nil
	}
	return commands
}
