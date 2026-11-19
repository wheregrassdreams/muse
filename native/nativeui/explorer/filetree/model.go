package filetree

import (
	"path/filepath"

	"muse/locales"
)

const (
	CommandCreateFile   CommandID = "create-file"
	CommandCreateFolder CommandID = "create-folder"
	CommandPaste        CommandID = "paste"
	CommandCollapse     CommandID = "collapse"
	CommandCopy         CommandID = "copy"
	CommandCut          CommandID = "cut"
	CommandChangeIcon   CommandID = "change-icon"
	CommandCopyRelative CommandID = "copy-relative"
	CommandCopyAbsolute CommandID = "copy-absolute"
	CommandRename       CommandID = "rename"
	CommandTrash        CommandID = "trash"
	CommandReveal       CommandID = "reveal"
	CommandOpenFile     CommandID = "open-file"
	CommandOpenFolder   CommandID = "open-folder"
)

// BuildViewModel projects catalog rows into the explorer's rendered state.
// The caller supplies only catalog facts and visual resources; selection,
// per-row capabilities, drag IDs, drop hints, and menu policy stay with the
// feature.
func BuildViewModel(handle *Handle, input ModelInput) ViewModel {
	rows := make([]Row, len(input.Entries))
	for i, entry := range input.Entries {
		rows[i] = entry.Row
	}

	model := ViewModel{
		PopoverAnchorID:  input.PopoverAnchorID,
		Commands:         append([]Command(nil), input.Commands...),
		EmptyMessage:     input.EmptyMessage,
		EmptyActionLabel: input.EmptyActionLabel,
		EmptyActions:     append([]Command(nil), input.EmptyActions...),
		Recent:           append([]RecentItem(nil), input.Recent...),
		RootLabel:        input.RootLabel,
		RootCanDrop:      input.RootCanDrop,
		NativeDrop:       input.NativeDrop,
		Entries:          make([]Entry, 0, len(input.Entries)),
	}
	dropRange := DropTargetRange{}
	hasClipboard := false
	if handle != nil && handle.state != nil {
		dropRange = handle.DropTargetRange()
		hasClipboard = len(handle.Clipboard().IDs) > 0
	}
	if input.RootLabel != "" {
		model.RootContextActions = rootContextActions(hasClipboard)
	}
	for i, source := range input.Entries {
		row := source.Row
		kind := File
		if row.Directory {
			kind = Directory
		}
		selectedPaths := []string(nil)
		selected := false
		if handle != nil {
			selectedPaths = handle.SelectedPaths(rows, row.Path)
			selected = handle.IsSelected(row.Path)
		}
		inDropRange := dropRange.TargetID != "" && i >= dropRange.StartRow && i < dropRange.EndRow && !selected
		model.Entries = append(model.Entries, Entry{
			ID: row.Path, Label: filepath.Base(row.Path), Detail: source.Detail, Kind: kind, Depth: row.Depth,
			CanSelect: true, CanExpand: row.Directory, CanOpen: !row.Directory,
			CanDrag: true, CanDrop: true, DragIDs: selectedPaths, DragActive: input.DragActive,
			ContextActions: entryContextActions(row.Directory, hasClipboard),
			Previewable:    source.Previewable, DropHint: inDropRange,
			Icon: source.Icon, IconTint: source.IconTint,
		})
	}
	return model
}

func rootContextActions(hasClipboard bool) []Command {
	commands := []Command{
		{ID: CommandCreateFile, Label: locales.FilesNewFile(), Enabled: true},
		{ID: CommandCreateFolder, Label: locales.FilesNewFolderMenu(), Enabled: true},
	}
	if hasClipboard {
		commands = append(commands, Command{ID: CommandPaste, Label: locales.FilesPasteItem(), Enabled: true})
	}
	return append(commands, Command{ID: CommandCollapse, Label: locales.FilesCollapseMenu(), Enabled: true})
}

func entryContextActions(isDirectory, hasClipboard bool) []Command {
	commands := []Command{{ID: CommandCollapse, Label: locales.FilesCollapseMenu(), Enabled: true}}
	if isDirectory {
		commands = append(commands,
			Command{ID: CommandCreateFile, Label: locales.FilesNewFile(), Enabled: true},
			Command{ID: CommandCreateFolder, Label: locales.FilesNewFolderMenu(), Enabled: true},
		)
	}
	commands = append(commands,
		Command{ID: CommandCopy, Label: locales.FilesCopyItem(), Enabled: true},
		Command{ID: CommandCut, Label: locales.FilesCutItem(), Enabled: true},
	)
	if hasClipboard {
		commands = append(commands, Command{ID: CommandPaste, Label: locales.FilesPasteItem(), Enabled: true})
	}
	return append(commands,
		Command{ID: CommandChangeIcon, Label: locales.FilesChangeIcon(), Enabled: true},
		Command{ID: CommandCopyRelative, Label: locales.FilesCopyRelativePath(), Enabled: true},
		Command{ID: CommandCopyAbsolute, Label: locales.FilesCopyAbsolutePath(), Enabled: true},
		Command{ID: CommandRename, Label: locales.FilesRenameEllipsis(), Enabled: true},
		Command{ID: CommandTrash, Label: locales.FilesMoveToTrash(), Enabled: true},
		Command{ID: CommandReveal, Label: locales.FilesRevealInFinder(), Enabled: true},
	)
}
