package nativeui

import (
	"errors"
	"muse/locales"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"muse/nativeui/interactions"
)

type treeClipboardItem struct{ path string }

func (a *App) anyModalOwnsKeyboard() bool {
	return a.settingsOpen || a.renameOpen || a.quickOpenOpen() || a.cmdPalette || a.workspaceSearch ||
		a.imagePreviewOpen() || a.navIconPickerOpen || a.propertyEditKey != "" || a.propertySourceMode ||
		a.propertyHiddenOpen || a.propertyAddOpen || a.busy || a.treeHelpOpen ||
		(a.documents != nil && a.documents.AnyOpen()) || (a.windows != nil && a.windows.HasModal())
}

func (a *App) ensurePanelActions() {
	if a.panelActions != nil {
		return
	}
	dispatcher, _ := interactions.NewActionDispatcher(
		interactions.PanelAction{ID: interactions.ActionFocusPanel, Run: func(ctx interactions.ActionContext) error {
			panel, _ := ctx.Values["panel"].(interactions.PanelID)
			if panel == "" {
				panel = interactions.PanelIDFiles
			}
			a.focusPanel(panel)
			return nil
		}},
		interactions.PanelAction{ID: interactions.ActionFocusEditor, Run: func(interactions.ActionContext) error {
			a.focusPanel(interactions.PanelIDEditor)
			return nil
		}},
		interactions.PanelAction{ID: interactions.ActionFocusDirection, Run: func(ctx interactions.ActionContext) error {
			direction, _ := ctx.Values["direction"].(int)
			a.focusPanelDirection(direction)
			return nil
		}},
		interactions.PanelAction{ID: interactions.ActionFocusNextPanel, Run: func(interactions.ActionContext) error { a.cycleFocusPanel(false); return nil }},
		interactions.PanelAction{ID: interactions.ActionFocusPreviousPanel, Run: func(interactions.ActionContext) error { a.cycleFocusPanel(true); return nil }},
		interactions.PanelAction{ID: interactions.ActionNavigate, Run: func(ctx interactions.ActionContext) error { return a.navigateTree(ctx, false) }},
		interactions.PanelAction{ID: interactions.ActionSelectRange, Run: func(ctx interactions.ActionContext) error { return a.navigateTree(ctx, true) }},
		interactions.PanelAction{ID: interactions.ActionOpen, Run: func(ctx interactions.ActionContext) error {
			path := actionPath(ctx, a.treeFocusPath)
			if path == "" {
				return nil
			}
			if a.treeIsDir(path) {
				a.expanded[path] = !a.expanded[path]
				a.fileRowsDirty = true
				a.invalidate()
			} else {
				a.selectedPath = path
				a.requestOpen(path)
			}
			return nil
		}},
		interactions.PanelAction{ID: interactions.ActionToggleExpanded, Run: func(ctx interactions.ActionContext) error { return a.toggleTreeHorizontal(ctx) }},
		interactions.PanelAction{ID: interactions.ActionCloseNode, Run: func(interactions.ActionContext) error { return a.closeTreeNode() }},
		interactions.PanelAction{ID: interactions.ActionCollapseAll, Run: func(interactions.ActionContext) error { a.collapseAll(); return nil }},
		interactions.PanelAction{ID: interactions.ActionSelectAll, Run: func(interactions.ActionContext) error {
			if a.treeSelection == nil {
				a.treeSelection = make(map[string]bool)
			}
			clear(a.treeSelection)
			rows := a.fileRows()
			for _, row := range rows {
				a.treeSelection[row.path] = true
			}
			if len(rows) > 0 {
				a.treeSelectionAnchor = rows[0].path
				a.treeFocusPath = rows[0].path
			}
			a.invalidate()
			return nil
		}},
		interactions.PanelAction{ID: interactions.ActionToggleSelection, Run: func(interactions.ActionContext) error {
			a.toggleTreeSelection(a.treeFocusPath)
			return nil
		}},
		interactions.PanelAction{ID: interactions.ActionInvertSelection, Run: func(interactions.ActionContext) error {
			a.invertTreeSelection()
			return nil
		}},
		interactions.PanelAction{ID: interactions.ActionRename, CanRun: func(ctx interactions.ActionContext) bool {
			if path, ok := ctx.Values["path"].(string); ok && path != "" {
				return true
			}
			return len(a.selectedTreePaths(a.treeFocusPath)) == 1
		}, Run: func(ctx interactions.ActionContext) error {
			path := actionPath(ctx, "")
			if path == "" {
				paths := a.selectedTreePaths(a.treeFocusPath)
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
		interactions.PanelAction{ID: interactions.ActionCopy, Run: func(interactions.ActionContext) error { a.captureTreeClipboard(false); return nil }},
		interactions.PanelAction{ID: interactions.ActionCut, Run: func(interactions.ActionContext) error { a.captureTreeClipboard(true); return nil }},
		interactions.PanelAction{ID: interactions.ActionPaste, CanRun: func(interactions.ActionContext) bool { return len(a.treeClipboard) > 0 }, Run: func(interactions.ActionContext) error { return a.pasteTreeClipboard() }},
		interactions.PanelAction{ID: interactions.ActionTrash, Run: func(interactions.ActionContext) error {
			a.trashTreeItems(a.treeActionPaths())
			return nil
		}},
		interactions.PanelAction{ID: interactions.ActionCreateFile, Run: func(interactions.ActionContext) error { a.newFileIn(a.treeActionTarget()); return nil }},
		interactions.PanelAction{ID: interactions.ActionCreateFolder, Run: func(interactions.ActionContext) error { a.newFolderIn(a.treeActionTarget()); return nil }},
		interactions.PanelAction{ID: interactions.ActionCopyPath, Run: func(ctx interactions.ActionContext) error {
			if path := actionPath(ctx, a.treeFocusPath); path != "" {
				relative, ok := ctx.Values["relative"].(bool)
				if !ok {
					relative = true
				}
				a.copyPath(path, relative)
			}
			return nil
		}},
		interactions.PanelAction{ID: interactions.ActionRevealInFinder, Run: func(ctx interactions.ActionContext) error {
			if path := actionPath(ctx, a.treeFocusPath); path != "" {
				mygo.Shell.ShowItemInFolder(path)
			}
			return nil
		}},
		interactions.PanelAction{ID: interactions.ActionRefreshPanel, Run: func(interactions.ActionContext) error { a.clearTreeCache(); a.invalidate(); return nil }},
		interactions.PanelAction{ID: interactions.ActionCancel, Run: func(interactions.ActionContext) error { a.focusPanel(interactions.PanelIDEditor); return nil }},
		interactions.PanelAction{ID: interactions.ActionShowHelp, Run: func(interactions.ActionContext) error {
			a.notice(locales.FilesTreeKeyHelp())
			return nil
		}},
	)
	a.panelActions = dispatcher
}

func actionPath(ctx interactions.ActionContext, fallback string) string {
	if path, ok := ctx.Values["path"].(string); ok && path != "" {
		return path
	}
	return fallback
}

func (a *App) dispatchPanelAction(id interactions.ActionID, values map[string]any) bool {
	a.ensurePanelActions()
	a.cancelPanelSequences()
	a.cancelTreeSequences()
	a.lastPanelAction = id
	target := interactions.FocusTargetEditor
	panel := interactions.PanelIDEditor
	if a.focusedPanel == interactions.PanelIDFiles {
		target, panel = interactions.FocusTargetTree, interactions.PanelIDFiles
	}
	ctx := interactions.ActionContext{Panel: panel, Focus: interactions.FocusState{Target: target, HasSelection: len(a.selectedTreePaths(a.treeFocusPath)) > 0, SelectionSize: len(a.selectedTreePaths(a.treeFocusPath))}, Values: values}
	if err := a.panelActions.Dispatch(id, ctx); err != nil {
		if !errors.Is(err, interactions.ErrActionDisabled) {
			a.notice(err.Error())
		}
		return false
	}
	a.invalidate()
	return true
}

func (a *App) dispatchTreeAction(path string, id interactions.ActionID, values map[string]any) bool {
	if path != "" {
		a.focusedPanel = interactions.PanelIDFiles
		a.treeFocusPath = path
		if values == nil {
			values = make(map[string]any)
		}
		values["path"] = path
	}
	return a.dispatchPanelAction(id, values)
}

func (a *App) anyModalOwnsKeyboardShortcut() bool { return a.anyModalOwnsKeyboard() }

type TreeBinding struct {
	Action  interactions.ActionID
	Values  map[string]any
	Handled bool
}

// StandardTreeBindings maps desktop file-browser shortcuts to semantic actions.
type StandardTreeBindings struct{}

func (StandardTreeBindings) Sequences() []interactions.KeyBinding {
	return []interactions.KeyBinding{{Sequence: []interactions.KeyStroke{{Key: "E", Mods: uint32(ui.Cmd | ui.Shift)}}, ActionID: interactions.ActionFocusPanel, Values: map[string]any{"panel": interactions.PanelIDFiles}}}
}

func (StandardTreeBindings) Resolve(mods ui.Modifiers, key ui.Key) TreeBinding {
	if mods == ui.Cmd {
		switch key {
		case ui.KeyA:
			return TreeBinding{Action: interactions.ActionSelectAll, Handled: true}
		case ui.KeyC:
			return TreeBinding{Action: interactions.ActionCopy, Handled: true}
		case ui.KeyX:
			return TreeBinding{Action: interactions.ActionCut, Handled: true}
		case ui.KeyV:
			return TreeBinding{Action: interactions.ActionPaste, Handled: true}
		}
	}
	if mods == 0 {
		switch key {
		case ui.KeyEscape:
			return TreeBinding{Action: interactions.ActionFocusEditor, Handled: true}
		case ui.KeyF2:
			return TreeBinding{Action: interactions.ActionRename, Handled: true}
		case ui.KeyDelete, ui.KeyBackspace:
			return TreeBinding{Action: interactions.ActionTrash, Handled: true}
		case ui.KeyDown:
			return TreeBinding{Action: interactions.ActionNavigate, Values: map[string]any{"delta": 1}, Handled: true}
		case ui.KeyUp:
			return TreeBinding{Action: interactions.ActionNavigate, Values: map[string]any{"delta": -1}, Handled: true}
		case ui.KeyLeft:
			return TreeBinding{Action: interactions.ActionToggleExpanded, Values: map[string]any{"direction": -1}, Handled: true}
		case ui.KeyRight:
			return TreeBinding{Action: interactions.ActionToggleExpanded, Values: map[string]any{"direction": 1}, Handled: true}
		case ui.KeyEnter:
			return TreeBinding{Action: interactions.ActionOpen, Handled: true}
		}
	}
	if mods == ui.Shift {
		switch key {
		case ui.KeyDown:
			return TreeBinding{Action: interactions.ActionSelectRange, Values: map[string]any{"delta": 1}, Handled: true}
		case ui.KeyUp:
			return TreeBinding{Action: interactions.ActionSelectRange, Values: map[string]any{"delta": -1}, Handled: true}
		}
	}
	return TreeBinding{}
}

// VimTreeBindings is a separate consumer; it emits the same semantic actions
// and keeps Vim sequence state in the mapping layer, not in action handlers.
type VimTreeBindings struct{}

func (VimTreeBindings) Sequences() []interactions.KeyBinding {
	return []interactions.KeyBinding{{Sequence: []interactions.KeyStroke{{Key: "G"}, {Key: "G"}}, ActionID: interactions.ActionNavigate, Values: map[string]any{"index": 0}}}
}

func (VimTreeBindings) PanelSequences() []interactions.KeyBinding {
	return []interactions.KeyBinding{
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "H"}}, ActionID: interactions.ActionFocusDirection, Values: map[string]any{"direction": -1}},
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "Left"}}, ActionID: interactions.ActionFocusDirection, Values: map[string]any{"direction": -1}},
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "L"}}, ActionID: interactions.ActionFocusDirection, Values: map[string]any{"direction": 1}},
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "Right"}}, ActionID: interactions.ActionFocusDirection, Values: map[string]any{"direction": 1}},
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "W"}}, ActionID: interactions.ActionFocusNextPanel},
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "W", Mods: uint32(ui.Shift)}}, ActionID: interactions.ActionFocusPreviousPanel},
	}
}

func (VimTreeBindings) Resolve(mods ui.Modifiers, key ui.Key) TreeBinding {
	if mods == ui.Ctrl|ui.Shift && key == ui.KeyI {
		return TreeBinding{Action: interactions.ActionInvertSelection, Handled: true}
	}
	if mods == ui.Shift {
		switch key {
		case ui.KeySlash:
			return TreeBinding{Action: interactions.ActionShowHelp, Handled: true}
		case ui.KeyG:
			return TreeBinding{Action: interactions.ActionNavigate, Values: map[string]any{"index": -1}, Handled: true}
		case ui.KeyA:
			return TreeBinding{Action: interactions.ActionCreateFolder, Handled: true}
		case ui.KeyJ:
			return TreeBinding{Action: interactions.ActionSelectRange, Values: map[string]any{"delta": 1}, Handled: true}
		case ui.KeyK:
			return TreeBinding{Action: interactions.ActionSelectRange, Values: map[string]any{"delta": -1}, Handled: true}
		case ui.KeyC:
			return TreeBinding{Action: interactions.ActionCloseNode, Handled: true}
		case ui.KeyY:
			return TreeBinding{Action: interactions.ActionCopyPath, Values: map[string]any{"relative": true}, Handled: true}
		}
	}
	if mods != 0 {
		return TreeBinding{}
	}
	switch key {
	case ui.KeyJ:
		return TreeBinding{Action: interactions.ActionNavigate, Values: map[string]any{"delta": 1}, Handled: true}
	case ui.KeyK:
		return TreeBinding{Action: interactions.ActionNavigate, Values: map[string]any{"delta": -1}, Handled: true}
	case ui.KeyH:
		return TreeBinding{Action: interactions.ActionToggleExpanded, Values: map[string]any{"direction": -1}, Handled: true}
	case ui.KeyL:
		return TreeBinding{Action: interactions.ActionToggleExpanded, Values: map[string]any{"direction": 1}, Handled: true}
	case ui.KeyR:
		return TreeBinding{Action: interactions.ActionRename, Handled: true}
	case ui.KeyY:
		return TreeBinding{Action: interactions.ActionCopy, Handled: true}
	case ui.KeyX:
		return TreeBinding{Action: interactions.ActionCut, Handled: true}
	case ui.KeyP:
		return TreeBinding{Action: interactions.ActionPaste, Handled: true}
	case ui.KeyD:
		return TreeBinding{Action: interactions.ActionTrash, Handled: true}
	case ui.KeyA:
		return TreeBinding{Action: interactions.ActionCreateFile, Handled: true}
	case ui.KeyZ:
		return TreeBinding{Action: interactions.ActionCollapseAll, Handled: true}
	case ui.KeyTab:
		return TreeBinding{Action: interactions.ActionToggleSelection, Handled: true}
	}
	return TreeBinding{}
}

// closeTreeNode closes the focused directory when it is expanded. Otherwise
// it closes the nearest expanded ancestor and moves tree focus to that node.
func (a *App) closeTreeNode() error {
	path := a.treeFocusPath
	if path == "" {
		return nil
	}
	rows := a.fileRows()
	if a.expanded[path] {
		a.expanded[path] = false
		a.fileRowsDirty = true
		a.invalidate()
		return nil
	}
	for parent := filepath.Dir(path); parent != path; parent = filepath.Dir(parent) {
		if !a.expanded[parent] {
			continue
		}
		a.expanded[parent] = false
		a.treeFocusPath = parent
		a.selectTreeRow(parent, 0, rows)
		a.fileRowsDirty = true
		a.invalidate()
		return nil
	}
	return nil
}

func (a *App) toggleTreeSelection(path string) {
	if path == "" {
		return
	}
	if a.treeSelection == nil {
		a.treeSelection = make(map[string]bool)
	}
	if a.treeSelection[path] {
		delete(a.treeSelection, path)
	} else {
		a.treeSelection[path] = true
	}
	if len(a.treeSelection) == 0 {
		a.treeSelection = nil
	}
	if a.treeSelectionAnchor == "" {
		a.treeSelectionAnchor = path
	}
	a.invalidate()
}

func (a *App) invertTreeSelection() {
	rows := a.fileRows()
	if a.treeSelection == nil {
		a.treeSelection = make(map[string]bool)
	}
	for _, row := range rows {
		if a.treeSelection[row.path] {
			delete(a.treeSelection, row.path)
		} else {
			a.treeSelection[row.path] = true
		}
	}
	if len(a.treeSelection) == 0 {
		a.treeSelection = nil
	}
	if a.treeFocusPath != "" && a.treeSelection[a.treeFocusPath] {
		a.treeSelectionAnchor = a.treeFocusPath
	}
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
	if a.ed != nil && a.ed.Keymap() == "vim" {
		keymap = "vim"
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
	vimGlobal := a.ed != nil && a.ed.Keymap() == "vim" && !a.ed.ModalAcceptsText()
	for _, candidate := range a.panelKeySequences.Candidates() {
		if !a.panelKeySequences.Pending() && candidate.Mods == 0 {
			continue
		}
		isStandardFocus := candidate.Key == "E" && candidate.Mods == uint32(ui.Cmd|ui.Shift)
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
	if a.ed != nil && a.ed.Keymap() == "vim" {
		if strokeName := keyStrokeName(key); strokeName != "" {
			if a.feedKeySequence(a.treeKeySequences, interactions.KeyStroke{Key: strokeName, Mods: uint32(mods)}) {
				return true
			}
		}
	} else {
		a.cancelTreeSequences()
	}
	binding := StandardTreeBindings{}.Resolve(mods, key)
	if !binding.Handled && a.ed != nil && a.ed.Keymap() == "vim" {
		binding = (VimTreeBindings{}).Resolve(mods, key)
	}
	if !binding.Handled {
		return false
	}
	if binding.Action != "" {
		a.dispatchPanelAction(binding.Action, binding.Values)
	}
	return true
}

func (a *App) ensureKeySequenceMatcher() {
	if a.treeKeySequences == nil {
		matcher, _ := interactions.NewKeySequenceMatcher(800*time.Millisecond, nil, VimTreeBindings{}.Sequences()...)
		a.treeKeySequences = matcher
	}
	if a.panelKeySequences == nil {
		bindings := append(StandardTreeBindings{}.Sequences(), VimTreeBindings{}.PanelSequences()...)
		matcher, _ := interactions.NewKeySequenceMatcher(800*time.Millisecond, nil, bindings...)
		a.panelKeySequences = matcher
	}
	a.cancelChangedTreeKeymap()
}

func (a *App) feedKeySequence(matcher *interactions.KeySequenceMatcher, stroke interactions.KeyStroke) bool {
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
	value := ui.Modifiers(mods)
	if value & ^(ui.Cmd|ui.Ctrl|ui.Shift|ui.Alt|ui.Super) != 0 {
		return 0, false
	}
	return value, true
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

func focusablePanelOrder() []interactions.PanelID {
	return []interactions.PanelID{interactions.PanelIDFiles, interactions.PanelIDEditor}
}

func (a *App) focusPanel(panel interactions.PanelID) bool {
	switch panel {
	case interactions.PanelIDFiles:
		a.showPanel(panelFiles)
		a.setFocusedPanel(interactions.PanelIDFiles)
		a.ensureTreeFocus()
		a.treeFocusRequested = true
	case interactions.PanelIDEditor:
		a.setFocusedPanel(interactions.PanelIDEditor)
		a.treeFocusRequested = false
		a.treeFocusHandle.CancelFocus()
		a.ed.Focus()
	default:
		return false
	}
	return true
}

// setFocusedPanel is the transition hook for toolkit focus as well as routed
// focus actions. Pointer clicks can move focus without passing through
// focusPanel, so any panel transition must also discard pending sequences.
func (a *App) setFocusedPanel(panel interactions.PanelID) {
	if panel == "" || a.focusedPanel == panel {
		return
	}
	a.focusedPanel = panel
	a.cancelPanelSequences()
	a.cancelTreeSequences()
}

func (a *App) focusPanelDirection(direction int) {
	router, err := interactions.NewFocusRouter(focusablePanelOrder(), a.focusedPanel)
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
	router, err := interactions.NewFocusRouter(focusablePanelOrder(), a.focusedPanel)
	if err != nil {
		return
	}
	target, moved := router.Cycle(reverse)
	if moved {
		a.focusPanel(target)
	}
}

func (a *App) ensureTreeFocus() {
	rows := a.fileRows()
	if len(rows) == 0 {
		a.treeFocusPath = ""
		return
	}
	for _, candidate := range []string{a.treeFocusPath, a.selectedPath, a.path} {
		for _, row := range rows {
			if candidate != "" && row.path == candidate {
				a.treeFocusPath = candidate
				return
			}
		}
	}
	a.treeFocusPath = rows[0].path
}

func (a *App) navigateTree(ctx interactions.ActionContext, extend bool) error {
	rows := a.fileRows()
	if len(rows) == 0 {
		a.treeFocusPath = ""
		return nil
	}
	a.ensureTreeFocus()
	index := 0
	for i, row := range rows {
		if row.path == a.treeFocusPath {
			index = i
			break
		}
	}
	if target, ok := ctx.Values["index"].(int); ok {
		if target < 0 {
			target = len(rows) - 1
		}
		if target >= len(rows) {
			target = len(rows) - 1
		}
		index = target
		a.treeFocusPath = rows[index].path
		a.selectTreeRow(a.treeFocusPath, 0, rows)
		return nil
	}
	delta, _ := ctx.Values["delta"].(int)
	next := index + delta
	if next < 0 {
		next = 0
	}
	if next >= len(rows) {
		next = len(rows) - 1
	}
	a.treeFocusPath = rows[next].path
	mods := ui.Modifiers(0)
	if extend {
		mods = ui.Shift
	}
	a.selectTreeRow(a.treeFocusPath, mods, rows)
	return nil
}

func (a *App) toggleTreeHorizontal(ctx interactions.ActionContext) error {
	a.ensureTreeFocus()
	path := a.treeFocusPath
	if path == "" {
		return nil
	}
	direction, _ := ctx.Values["direction"].(int)
	rows := a.fileRows()
	if direction > 0 {
		if a.treeIsDir(path) {
			if !a.expanded[path] {
				a.expanded[path] = true
				a.fileRowsDirty = true
				a.invalidate()
				return nil
			}
			for i, row := range rows {
				if row.path == path && i+1 < len(rows) && rows[i+1].depth > row.depth {
					a.treeFocusPath = rows[i+1].path
					a.selectTreeRow(a.treeFocusPath, 0, rows)
					return nil
				}
			}
		}
		return nil
	}
	if a.treeIsDir(path) && a.expanded[path] {
		a.expanded[path] = false
		a.fileRowsDirty = true
		a.invalidate()
		return nil
	}
	parent := filepath.Dir(path)
	for _, row := range rows {
		if row.path == parent {
			a.treeFocusPath = parent
			a.selectTreeRow(parent, 0, rows)
			break
		}
	}
	return nil
}

func (a *App) treeIsDir(path string) bool {
	a.dirMu.Lock()
	defer a.dirMu.Unlock()
	return a.isDir[path]
}

func (a *App) treeActionTarget() string {
	path := a.treeFocusPath
	if path != "" {
		if a.catalogIsDirectory(path) {
			return path
		}
		return filepath.Dir(path)
	}
	return a.root
}

func (a *App) captureTreeClipboard(cut bool) {
	paths := a.treeActionPaths()
	a.treeClipboard = a.treeClipboard[:0]
	for _, path := range paths {
		a.treeClipboard = append(a.treeClipboard, treeClipboardItem{path: path})
	}
	a.treeClipboardCut = cut
	if cut {
		a.notice(locales.CommonCutManyStatus(len(paths)))
	} else {
		a.notice(locales.CommonCopiedManyStatus(len(paths)))
	}
}

func (a *App) treeClipboardContains(path string) bool {
	for _, entry := range a.treeClipboard {
		if entry.path == path {
			return true
		}
	}
	return false
}

func (a *App) treeActionPaths() []string {
	paths := a.selectedTreePaths(a.treeFocusPath)
	selected := make(map[string]bool, len(paths))
	for _, path := range paths {
		selected[path] = true
	}
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		covered := false
		for parent := filepath.Dir(path); parent != path; {
			if selected[parent] && a.treeIsDir(parent) {
				covered = true
				break
			}
			next := filepath.Dir(parent)
			if next == parent {
				break
			}
			parent = next
		}
		if !covered {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func (a *App) pasteTreeClipboard() error {
	destination := a.treeActionTarget()
	if destination == "" {
		destination = a.root
	}
	if destination == "" {
		return nil
	}
	if a.fileOperations == nil {
		return errors.New("vault file operations are not configured")
	}
	if !a.treeIsDir(destination) {
		if !a.catalogIsDirectory(destination) {
			destination = filepath.Dir(destination)
		}
	}
	paths := make([]string, 0, len(a.treeClipboard))
	for _, entry := range a.treeClipboard {
		paths = append(paths, entry.path)
	}
	if a.treeClipboardCut {
		moves, err := a.fileOperations.Move(a.root, paths, destination)
		if len(moves) > 0 {
			a.remapTreePaths(moves)
		}
		moved := make(map[string]bool, len(moves))
		for _, move := range moves {
			moved[move.Source] = true
		}
		remaining := make([]treeClipboardItem, 0, len(paths)-len(moves))
		for _, path := range paths {
			if !moved[path] {
				remaining = append(remaining, treeClipboardItem{path: path})
			}
		}
		a.treeClipboard = remaining
		if len(remaining) == 0 {
			a.treeClipboard = nil
			a.treeClipboardCut = false
		}
		if err != nil {
			a.notice(locales.CommonPasteFailedStatus(err.Error()))
			return err
		}
	} else {
		_, err := a.fileOperations.Copy(a.root, paths, destination)
		if err != nil {
			a.notice(locales.CommonPasteFailedStatus(err.Error()))
			return err
		}
	}
	a.clearTreeCache()
	a.invalidate()
	return nil
}
