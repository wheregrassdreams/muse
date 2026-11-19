package findreplace

import (
	editfind "github.com/wheregrassdreams/muse-editor/text/findreplace"
	noteeditor "muse/nativeui/workbench/features/editor/surface"
)

// NewEditorAdapter binds find/replace to whichever editor is active when an
// action runs. The feature stays independent of the root App and editor view.
func NewEditorAdapter(resolve func() noteeditor.Commands, onError func(error)) EditorPort {
	return editorAdapter{resolve: resolve, onError: onError}
}

type editorAdapter struct {
	resolve func() noteeditor.Commands
	onError func(error)
}

func (a editorAdapter) commands() noteeditor.Commands {
	if a.resolve == nil {
		return nil
	}
	return a.resolve()
}

func (a editorAdapter) Source() string {
	if commands := a.commands(); commands != nil {
		return commands.Source()
	}
	return ""
}

func (a editorAdapter) SetHighlights(matches []editfind.Match, selected int) {
	commands := a.commands()
	if commands == nil || commands.SearchCommands() == nil {
		return
	}
	ranges := make([]noteeditor.TextRange, len(matches))
	for i, match := range matches {
		ranges[i] = noteeditor.TextRange{Start: match.Start, End: match.End}
	}
	commands.SearchCommands().SetHighlights(ranges, selected)
}

func (a editorAdapter) NavigateToRange(start, end int) {
	if commands := a.commands(); commands != nil && commands.NavigationCommands() != nil {
		commands.NavigationCommands().NavigateToRange(start, end)
	}
}

func (a editorAdapter) Focus() {
	if commands := a.commands(); commands != nil && commands.FocusCommands() != nil {
		commands.FocusCommands().Focus()
	}
}

func (a editorAdapter) Replace(start, end int, value string) error {
	if commands := a.commands(); commands != nil {
		return commands.Replace(start, end, value)
	}
	return nil
}

func (a editorAdapter) ReplaceAll(source string) {
	if commands := a.commands(); commands != nil {
		if err := commands.ReplaceUser(source); err != nil && a.onError != nil {
			a.onError(err)
		}
	}
}
