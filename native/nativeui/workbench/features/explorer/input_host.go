package nativeui

import (
	"errors"
	"muse/locales"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	applicationvault "muse/application/vault"
	commands "muse/nativeui/workbench/commands"
	filetree "muse/nativeui/workbench/features/explorer"
	focus "muse/nativeui/workbench/focus"
	keybindings "muse/nativeui/workbench/keybindings"
)

func (a *App) anyModalOwnsKeyboard() bool {
	return a.settingsPanel.IsOpen() || a.renameOpen || a.quickOpenOpen() || a.commandPalette.IsOpen() || a.workspaceSearch.IsOpen() ||
		a.imagePreviewOpen() || a.navIconPicker.IsOpen() || a.propertiesPanel.HasTransientUI() || a.busy || a.treeHelpOpen ||
		(a.documents != nil && a.documents.ModalOpen()) || (a.windows != nil && a.windows.HasModal())
}

func (a *App) ensurePanelActions() {
	if a.panelActions != nil {
		return
	}
	dispatcher, _ := commands.NewActionDispatcher(
		commands.PanelAction{ID: commands.ActionFocusPanel, Run: func(ctx commands.ActionContext) error {
			panel, _ := ctx.Values["panel"].(commands.PanelID)
			if panel == "" {
				panel = commands.PanelIDFiles
			}
			a.focusPanel(panel)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionFocusEditor, Run: func(commands.ActionContext) error {
			a.focusPanel(commands.PanelIDEditor)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionFocusDirection, Run: func(ctx commands.ActionContext) error {
			direction, _ := ctx.Values["direction"].(int)
			a.focusPanelDirection(direction)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionFocusNextPanel, Run: func(commands.ActionContext) error { a.cycleFocusPanel(false); return nil }},
		commands.PanelAction{ID: commands.ActionFocusPreviousPanel, Run: func(commands.ActionContext) error { a.cycleFocusPanel(true); return nil }},
		commands.PanelAction{ID: commands.ActionNavigate, Run: func(ctx commands.ActionContext) error {
			a.applyTreeKeyAction(commands.ActionNavigate, ctx.Values)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionSelectRange, Run: func(ctx commands.ActionContext) error {
			a.applyTreeKeyAction(commands.ActionSelectRange, ctx.Values)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionOpen, Run: func(ctx commands.ActionContext) error {
			path := actionPath(ctx, a.fileTreeState.FocusID())
			if path == "" {
				return nil
			}
			if a.treeIsDir(path) {
				a.applyTreeKey(filetree.KeyCommand{Operation: filetree.KeyToggleExpanded, TargetID: path})
			} else {
				a.selectedPath = path
				a.requestOpen(path)
			}
			return nil
		}},
		commands.PanelAction{ID: commands.ActionToggleExpanded, Run: func(ctx commands.ActionContext) error {
			a.applyTreeKeyAction(commands.ActionToggleExpanded, ctx.Values)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionCloseNode, Run: func(commands.ActionContext) error {
			a.applyTreeKeyAction(commands.ActionCloseNode, nil)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionCollapseAll, Run: func(commands.ActionContext) error {
			a.applyTreeKeyAction(commands.ActionCollapseAll, nil)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionSelectAll, Run: func(commands.ActionContext) error {
			a.applyTreeKeyAction(commands.ActionSelectAll, nil)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionToggleSelection, Run: func(commands.ActionContext) error {
			a.applyTreeKeyAction(commands.ActionToggleSelection, nil)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionInvertSelection, Run: func(commands.ActionContext) error {
			a.applyTreeKeyAction(commands.ActionInvertSelection, nil)
			return nil
		}},
		commands.PanelAction{ID: commands.ActionRename, CanRun: func(ctx commands.ActionContext) bool {
			if path, ok := ctx.Values["path"].(string); ok && path != "" {
				return true
			}
			return len(a.fileTreeState.SelectedPaths(a.fileRows(), a.fileTreeState.FocusID())) == 1
		}, Run: func(ctx commands.ActionContext) error {
			path := actionPath(ctx, "")
			if path == "" {
				paths := a.fileTreeState.SelectedPaths(a.fileRows(), a.fileTreeState.FocusID())
				if len(paths) > 0 {
					path = paths[0]
				}
			}
			if path == "" {
				return nil
			}
			a.renamePath, a.renameValue, a.renameOpen = path, filepath.Base(path), true
			return nil
		}},
		commands.PanelAction{ID: commands.ActionCopy, Run: func(commands.ActionContext) error { a.captureTreeClipboard(false); return nil }},
		commands.PanelAction{ID: commands.ActionCut, Run: func(commands.ActionContext) error { a.captureTreeClipboard(true); return nil }},
		commands.PanelAction{ID: commands.ActionPaste, CanRun: func(commands.ActionContext) bool { return len(a.fileTreeState.Clipboard().IDs) > 0 }, Run: func(commands.ActionContext) error { return a.pasteTreeClipboard() }},
		commands.PanelAction{ID: commands.ActionTrash, Run: func(commands.ActionContext) error {
			a.trashTreeItems(a.treeActionPaths())
			return nil
		}},
		commands.PanelAction{ID: commands.ActionCreateFile, Run: func(commands.ActionContext) error { a.newFileIn(a.treeActionTarget()); return nil }},
		commands.PanelAction{ID: commands.ActionCreateFolder, Run: func(commands.ActionContext) error { a.newFolderIn(a.treeActionTarget()); return nil }},
		commands.PanelAction{ID: commands.ActionCopyPath, Run: func(ctx commands.ActionContext) error {
			if path := actionPath(ctx, a.fileTreeState.FocusID()); path != "" {
				relative, ok := ctx.Values["relative"].(bool)
				if !ok {
					relative = true
				}
				a.copyPath(path, relative)
			}
			return nil
		}},
		commands.PanelAction{ID: commands.ActionRevealInFinder, Run: func(ctx commands.ActionContext) error {
			if path := actionPath(ctx, a.fileTreeState.FocusID()); path != "" {
				mygo.Shell.ShowItemInFolder(path)
			}
			return nil
		}},
		commands.PanelAction{ID: commands.ActionRefreshPanel, Run: func(commands.ActionContext) error { a.clearTreeCache(); a.invalidate(); return nil }},
		commands.PanelAction{ID: commands.ActionCancel, Run: func(commands.ActionContext) error { a.focusPanel(commands.PanelIDEditor); return nil }},
		commands.PanelAction{ID: commands.ActionShowHelp, Run: func(commands.ActionContext) error {
			a.notice(locales.FilesTreeKeyHelp())
			return nil
		}},
	)
	a.panelActions = dispatcher
}

func actionPath(ctx commands.ActionContext, fallback string) string {
	if path, ok := ctx.Values["path"].(string); ok && path != "" {
		return path
	}
	return fallback
}

func (a *App) dispatchPanelAction(id commands.ActionID, values map[string]any) bool {
	a.ensurePanelActions()
	a.cancelPanelSequences()
	a.cancelTreeSequences()
	a.lastPanelAction = id
	target := commands.FocusTargetEditor
	panel := commands.PanelIDEditor
	if a.focusedPanel == commands.PanelIDFiles {
		target, panel = commands.FocusTargetTree, commands.PanelIDFiles
	}
	focusID := a.fileTreeState.FocusID()
	selectedPaths := a.fileTreeState.SelectedPaths(a.fileRows(), focusID)
	ctx := commands.ActionContext{Panel: panel, Focus: commands.FocusState{Target: target, HasSelection: len(selectedPaths) > 0, SelectionSize: len(selectedPaths)}, Values: values}
	if err := a.panelActions.Dispatch(id, ctx); err != nil {
		if !errors.Is(err, commands.ErrActionDisabled) {
			a.notice(err.Error())
		}
		return false
	}
	a.invalidate()
	return true
}

func (a *App) dispatchTreeAction(path string, id commands.ActionID, values map[string]any) bool {
	if path != "" {
		a.focusedPanel = commands.PanelIDFiles
		a.applyTreeKey(filetree.KeyCommand{Operation: filetree.KeyFocus, TargetID: path})
		if values == nil {
			values = make(map[string]any)
		}
		values["path"] = path
	}
	return a.dispatchPanelAction(id, values)
}

func (a *App) anyModalOwnsKeyboardShortcut() bool { return a.anyModalOwnsKeyboard() }

// applyTreeKeyAction translates the stable interaction action ID into the
// filetree package's typed, panel-owned keyboard command.
func (a *App) applyTreeKeyAction(id commands.ActionID, values map[string]any) {
	args := filetree.TreeBindingArgs{}
	if values != nil {
		args.Delta, _ = values["delta"].(int)
		args.Direction, _ = values["direction"].(int)
		args.Index, args.HasIndex = values["index"].(int)
		args.TargetID, _ = values["path"].(string)
	}
	command, ok := filetree.KeyCommandForAction(id, args, []string{a.selectedPath, a.documentPath()})
	if !ok {
		return
	}
	a.applyTreeKey(command)
}

func (a *App) applyTreeKey(command filetree.KeyCommand) {
	a.fileTreeState.ApplyKey(command, a.fileRows())
	a.invalidate()
}

func (a *App) cancelPanelSequences() {
	if a.panelKeySequences != nil {
		a.panelKeySequences.Cancel()
	}
}

func (a *App) cancelTreeSequences() {
	if a.treeKeySequences != nil {
		a.treeKeySequences.Cancel()
	}
}

func (a *App) cancelChangedTreeKeymap() {
	keymap := "standard"
	if a.noteEditor() != nil {
		if modal := a.noteEditor().ModalCommands(); modal != nil && modal.Keymap() == "vim" {
			keymap = "vim"
		}
	}
	if a.treeSequenceKeymap != "" && a.treeSequenceKeymap != keymap {
		a.cancelTreeSequences()
		a.cancelPanelSequences()
	}
	a.treeSequenceKeymap = keymap
}

func (a *App) handlePanelShortcuts(c *ui.Context) bool {
	if a.anyModalOwnsKeyboardShortcut() {
		a.cancelPanelSequences()
		a.cancelTreeSequences()
		return false
	}
	a.ensureKeySequenceMatcher()
	if a.panelKeySequences.Pending() {
		result := a.panelKeySequences.Expire()
		if result.ActionID != "" {
			a.dispatchPanelAction(result.ActionID, result.Values)
		}
		if result.Pending {
			c.After(800 * time.Millisecond)
		}
	}
	vimGlobal := a.noteEditor() != nil && a.noteEditor().WantsUnmodifiedShortcuts()
	for _, candidate := range a.panelKeySequences.Candidates() {
		if !a.panelKeySequences.Pending() && candidate.Mods == 0 {
			continue
		}
		isStandardFocus := candidate.Key == "E" && candidate.Mods == keybindings.ModPrimary|keybindings.ModShift
		if !a.panelKeySequences.Pending() && !isStandardFocus && !vimGlobal {
			continue
		}
		mods, ok := uiModifiersFor(candidate.Mods)
		if !ok {
			continue
		}
		key, ok := uiKeyForName(candidate.Key)
		if !ok {
			continue
		}
		if c.Shortcut(mods, key) {
			return a.feedKeySequence(a.panelKeySequences, candidate)
		}
	}
	return false
}

func (a *App) handleTreeKey(mods ui.Modifiers, key ui.Key) bool {
	if a.anyModalOwnsKeyboard() {
		a.cancelTreeSequences()
		return false
	}
	a.ensureKeySequenceMatcher()
	a.cancelChangedTreeKeymap()
	if a.noteEditor() != nil && a.noteEditor().ModalCommands().Keymap() == "vim" {
		if strokeName := keyStrokeName(key); strokeName != "" {
			if a.feedKeySequence(a.treeKeySequences, keybindings.KeyStroke{Key: strokeName, Mods: keybindingModifiers(mods)}) {
				return true
			}
		}
	} else {
		a.cancelTreeSequences()
	}
	binding := filetree.StandardTreeBindings{}.Resolve(mods, key)
	if !binding.Handled && a.noteEditor() != nil && a.noteEditor().ModalCommands().Keymap() == "vim" {
		binding = (filetree.VimTreeBindings{}).Resolve(mods, key)
	}
	if !binding.Handled {
		return false
	}
	if binding.Action != "" {
		a.dispatchPanelAction(binding.Action, binding.Args.Values())
	}
	return true
}

func (a *App) ensureKeySequenceMatcher() {
	if a.treeKeySequences == nil {
		matcher, _ := keybindings.NewKeySequenceMatcher(800*time.Millisecond, nil, filetree.VimTreeBindings{}.Sequences()...)
		a.treeKeySequences = matcher
	}
	if a.panelKeySequences == nil {
		bindings := append(keybindings.StandardPanelBindings(), keybindings.VimPanelBindings()...)
		matcher, _ := keybindings.NewKeySequenceMatcher(800*time.Millisecond, nil, bindings...)
		a.panelKeySequences = matcher
	}
	a.cancelChangedTreeKeymap()
}

func (a *App) feedKeySequence(matcher *keybindings.KeySequenceMatcher, stroke keybindings.KeyStroke) bool {
	for tries := 0; tries < 2; tries++ {
		result := matcher.Match(stroke)
		if result.ActionID != "" {
			a.dispatchPanelAction(result.ActionID, result.Values)
		}
		if result.Reprocess {
			continue
		}
		return result.Handled
	}
	return true
}

func keyStrokeName(key ui.Key) string {
	if key >= ui.KeyA && key <= ui.KeyZ {
		return string(rune('A' + key - ui.KeyA))
	}
	if key >= ui.Key0 && key <= ui.Key9 {
		return string(rune('0' + key - ui.Key0))
	}
	switch key {
	case ui.KeyEscape:
		return "Escape"
	case ui.KeyLeft:
		return "Left"
	case ui.KeyRight:
		return "Right"
	case ui.KeyUp:
		return "Up"
	case ui.KeyDown:
		return "Down"
	}
	return ""
}

func uiModifiersFor(mods uint32) (ui.Modifiers, bool) {
	var value ui.Modifiers
	known := uint32(0)
	if mods&keybindings.ModShift != 0 {
		value |= ui.Shift
		known |= keybindings.ModShift
	}
	if mods&keybindings.ModCtrl != 0 {
		value |= ui.Ctrl
		known |= keybindings.ModCtrl
	}
	if mods&keybindings.ModAlt != 0 {
		value |= ui.Alt
		known |= keybindings.ModAlt
	}
	if mods&keybindings.ModSuper != 0 {
		value |= ui.Super
		known |= keybindings.ModSuper
	}
	if mods&keybindings.ModPrimary != 0 {
		value |= ui.Cmd
		known |= keybindings.ModPrimary
	}
	if mods != known {
		return 0, false
	}
	return value, true
}

func keybindingModifiers(mods ui.Modifiers) uint32 {
	var value uint32
	if mods&ui.Shift != 0 {
		value |= keybindings.ModShift
	}
	if mods&ui.Alt != 0 {
		value |= keybindings.ModAlt
	}
	if mods&ui.Cmd != 0 {
		value |= keybindings.ModPrimary
	}
	// Cmd may alias Ctrl off macOS or Super on macOS. Preserve the physical
	// modifier separately only when it is distinct from the platform primary.
	if ui.Cmd != ui.Ctrl && mods&ui.Ctrl != 0 {
		value |= keybindings.ModCtrl
	}
	if ui.Cmd != ui.Super && mods&ui.Super != 0 {
		value |= keybindings.ModSuper
	}
	return value
}

func uiKeyForName(name string) (ui.Key, bool) {
	switch name {
	case "Escape":
		return ui.KeyEscape, true
	case "Left":
		return ui.KeyLeft, true
	case "Right":
		return ui.KeyRight, true
	case "Up":
		return ui.KeyUp, true
	case "Down":
		return ui.KeyDown, true
	}
	if len(name) == 1 {
		if name[0] >= 'A' && name[0] <= 'Z' {
			return ui.KeyA + ui.Key(name[0]-'A'), true
		}
		if name[0] >= '0' && name[0] <= '9' {
			return ui.Key0 + ui.Key(name[0]-'0'), true
		}
	}
	return 0, false
}

func focusablePanelOrder() []commands.PanelID {
	return []commands.PanelID{commands.PanelIDFiles, commands.PanelIDEditor}
}

func (a *App) focusPanel(panel commands.PanelID) bool {
	switch panel {
	case commands.PanelIDFiles:
		a.showPanel(panelFiles)
		a.setFocusedPanel(commands.PanelIDFiles)
		a.ensureTreeFocus()
		a.treeFocusRequested = true
	case commands.PanelIDEditor:
		a.setFocusedPanel(commands.PanelIDEditor)
		a.treeFocusRequested = false
		a.treeFocusHandle.CancelFocus()
		a.noteEditor().FocusCommands().Focus()
	default:
		return false
	}
	return true
}

// setFocusedPanel is the transition hook for toolkit focus as well as routed
// focus actions. Pointer clicks can move focus without passing through
// focusPanel, so any panel transition must also discard pending sequences.
func (a *App) setFocusedPanel(panel commands.PanelID) {
	if panel == "" || a.focusedPanel == panel {
		return
	}
	a.focusedPanel = panel
	a.cancelPanelSequences()
	a.cancelTreeSequences()
}

func (a *App) focusPanelDirection(direction int) {
	router, err := focus.NewFocusRouter(focusablePanelOrder(), a.focusedPanel)
	if err != nil {
		return
	}
	target, moved := router.Move(direction)
	if !moved {
		return
	}
	a.focusPanel(target)
}

func (a *App) cycleFocusPanel(reverse bool) {
	router, err := focus.NewFocusRouter(focusablePanelOrder(), a.focusedPanel)
	if err != nil {
		return
	}
	target, moved := router.Cycle(reverse)
	if moved {
		a.focusPanel(target)
	}
}

func (a *App) ensureTreeFocus() {
	a.fileTreeState.ApplyKey(filetree.KeyCommand{Operation: filetree.KeyEnsureFocus, PreferredIDs: []string{a.selectedPath, a.documentPath()}}, a.fileRows())
}

func (a *App) treeIsDir(path string) bool {
	return a.catalogIsDirectory(path)
}

func (a *App) treeActionTarget() string {
	return a.vaultFileUseCases().DestinationForPath(a.fileTreeState.FocusID())
}

func (a *App) captureTreeClipboard(cut bool) {
	paths := a.treeActionPaths()
	clipboard := a.fileTreeState.CaptureClipboard(paths, cut)
	if clipboard.Kind == filetree.ClipboardCut {
		a.notice(locales.CommonCutManyStatus(len(paths)))
	} else {
		a.notice(locales.CommonCopiedManyStatus(len(paths)))
	}
}

func (a *App) treeClipboardContains(path string) bool {
	for _, id := range a.fileTreeState.Clipboard().IDs {
		if id == path {
			return true
		}
	}
	return false
}

func (a *App) treeActionPaths() []string {
	paths := a.fileTreeState.SelectedPaths(a.fileRows(), a.fileTreeState.FocusID())
	return a.vaultFileUseCases().NormalizeTargets(paths)
}

func (a *App) pasteTreeClipboard() error {
	destination := a.treeActionTarget()
	if destination == "" {
		destination = a.root
	}
	if destination == "" {
		return nil
	}
	clipboard := a.fileTreeState.Clipboard()
	paths := clipboard.IDs
	mutation, err := a.vaultFileUseCases().PasteFiles(applicationvault.PasteFilesCommand{
		Sources: paths, Destination: destination, Cut: clipboard.Kind == filetree.ClipboardCut,
	})
	if clipboard.Kind == filetree.ClipboardCut {
		if len(mutation.Moves) > 0 {
			a.remapTreePaths(mutation.Moves)
		}
		moved := make(map[string]bool, len(mutation.Moves))
		for _, move := range mutation.Moves {
			moved[move.Source] = true
		}
		movedPaths := make([]string, 0, len(mutation.Moves))
		for path := range moved {
			movedPaths = append(movedPaths, path)
		}
		a.fileTreeState.RetainCutPaths(movedPaths)
	}
	if err != nil {
		a.notice(locales.CommonPasteFailedStatus(err.Error()))
		return err
	}
	a.clearTreeCache()
	a.invalidate()
	return nil
}
