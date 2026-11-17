package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	commands "muse/nativeui/workbench/commands"
	filetree "muse/nativeui/workbench/features/explorer"
	settingsui "muse/nativeui/workbench/features/settings"
)

func keyTestWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"alpha.md", "beta.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("# "+name+"\nline two\nline three"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "dest"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestFileTreeFocusNavigationDoesNotMoveEditorSelection(t *testing.T) {
	root := keyTestWorkspace(t)
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	a.loadDoc("line one\nline two\nline three")
	a.noteEditor().ModalCommands().SetKeymap("vim")
	a.noteEditor().FocusCommands().Focus()
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	before := a.noteEditor().NavigationCommands().SelectionRange()

	tt.Key(0, ui.KeyJ)
	tt.Frame()
	if a.focusedPanel != commands.PanelIDEditor || a.lastPanelAction != "" {
		t.Fatalf("Vim j with editor focus reached tree: panel=%q action=%q", a.focusedPanel, a.lastPanelAction)
	}
	if a.fileTreeState.FocusID() != "" {
		t.Fatalf("editor Vim navigation changed tree focus: %q", a.fileTreeState.FocusID())
	}

	tt.Key(ui.Cmd|ui.Shift, ui.KeyE)
	tt.Frame()
	if !tt.Focused("文件树") || a.focusedPanel != commands.PanelIDFiles {
		t.Fatalf("focus-panel shortcut did not transfer toolkit focus: treeFocused=%v appPanel=%q", tt.Focused("文件树"), a.focusedPanel)
	}
	rows := a.fileRows()
	if len(rows) < 2 || a.fileTreeState.FocusID() != rows[0].Path {
		t.Fatalf("initial tree focus = %q rows=%v", a.fileTreeState.FocusID(), rows)
	}
	next := rows[1].Path
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.fileTreeState.FocusID() != next || a.lastPanelAction != commands.ActionNavigate {
		t.Fatalf("tree Down did not dispatch navigation: focus=%q action=%q", a.fileTreeState.FocusID(), a.lastPanelAction)
	}
	if got := a.noteEditor().NavigationCommands().SelectionRange(); got != before || a.noteEditor().Source() != "line one\nline two\nline three" {
		t.Fatalf("tree navigation changed editor selection/source: range=%+v source=%q", got, a.noteEditor().Source())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.documentPath() != next || a.lastPanelAction != commands.ActionOpen {
		t.Fatalf("tree Enter did not open focused file through commands.ActionOpen: path=%q action=%q", a.documentPath(), a.lastPanelAction)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if tt.Focused("文件树") || !a.noteEditor().FocusCommands().Focused() || a.focusedPanel != commands.PanelIDEditor {
		t.Fatalf("Escape did not restore editor toolkit focus: treeFocused=%v editorFocused=%v panel=%q", tt.Focused("文件树"), a.noteEditor().FocusCommands().Focused(), a.focusedPanel)
	}
}

func TestFileTreeMultiCopyCutPasteUsesAppClipboard(t *testing.T) {
	root := keyTestWorkspace(t)
	dest := filepath.Join(root, "dest")
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	tt.Key(ui.Cmd|ui.Shift, ui.KeyE)
	tt.Frame()
	a.fileTreeState.SetSelection([]string{filepath.Join(root, "alpha.md"), filepath.Join(root, "beta.md")})
	a.fileTreeState.SetFocusID(filepath.Join(root, "alpha.md"))
	tt.Key(ui.Cmd, ui.KeyC)
	tt.Frame()
	if a.lastPanelAction != commands.ActionCopy || len(a.fileTreeState.Clipboard().IDs) != 2 || a.fileTreeState.Clipboard().Kind == filetree.ClipboardCut {
		t.Fatalf("Cmd+C did not capture selected files in app clipboard: action=%q clipboard=%+v cut=%v", a.lastPanelAction, a.fileTreeState.Clipboard().IDs, a.fileTreeState.Clipboard().Kind == filetree.ClipboardCut)
	}
	a.fileTreeState.SetFocusID(dest)
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	if a.lastPanelAction != commands.ActionPaste {
		t.Fatalf("Cmd+V action=%q", a.lastPanelAction)
	}
	for _, name := range []string{"alpha.md", "beta.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("copy changed source %s: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Fatalf("copy did not create %s: %v", name, err)
		}
	}
	// A collision gets a unique target while preserving both source and target.
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 4 {
		t.Fatalf("second copy should preserve originals and make unique names: entries=%v err=%v", entries, err)
	}

	a.fileTreeState.SetSelection([]string{filepath.Join(root, "alpha.md"), filepath.Join(root, "beta.md")})
	a.fileTreeState.SetFocusID(filepath.Join(root, "alpha.md"))
	tt.Key(ui.Cmd, ui.KeyX)
	tt.Frame()
	if a.lastPanelAction != commands.ActionCut || a.fileTreeState.Clipboard().Kind != filetree.ClipboardCut || len(a.fileTreeState.Clipboard().IDs) != 2 {
		t.Fatalf("Cmd+X did not stage cut selection: %+v cut=%v", a.fileTreeState.Clipboard().IDs, a.fileTreeState.Clipboard().Kind == filetree.ClipboardCut)
	}
	if !tt.HasText("剪切项目提示") {
		t.Fatal("cut selection has no visible row marker")
	}
	a.fileTreeState.SetFocusID(filepath.Join(root, "dest"))
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	for _, name := range []string{"alpha.md", "beta.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("cut paste left source %s: err=%v", name, err)
		}
	}
	if len(a.fileTreeState.Clipboard().IDs) != 0 || a.fileTreeState.Clipboard().Kind == filetree.ClipboardCut {
		t.Fatalf("successful cut paste did not clear clipboard: %+v cut=%v", a.fileTreeState.Clipboard().IDs, a.fileTreeState.Clipboard().Kind == filetree.ClipboardCut)
	}
	if tt.HasText("剪切项目提示") {
		t.Fatal("cut marker remained after successful paste")
	}
}

func TestFileTreeModalKeepsShortcutsFromTree(t *testing.T) {
	root := keyTestWorkspace(t)
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	a.settingsPanel.OpenAt(settingsui.AppearancePage)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyE)
	tt.Frame()
	if a.focusedPanel != commands.PanelIDEditor || a.lastPanelAction != "" || tt.Focused("文件树") {
		t.Fatalf("modal did not retain keyboard priority: panel=%q action=%q treeFocused=%v", a.focusedPanel, a.lastPanelAction, tt.Focused("文件树"))
	}
}

func TestFileTreeVimNodeSelectionAndRefreshOperations(t *testing.T) {
	root := keyTestWorkspace(t)
	dest := filepath.Join(root, "dest")
	nested := filepath.Join(dest, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "note.md"), []byte("# note"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	a.noteEditor().ModalCommands().SetKeymap("vim")
	a.fileTreeState.SetExpanded(dest, true)
	a.fileTreeState.SetExpanded(nested, true)
	a.fileTreeState.SetFocusID(nested)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	if !a.focusPanel(commands.PanelIDFiles) {
		t.Fatal("could not focus tree")
	}
	tt.Frame()

	// C closes the focused directory; from an item it closes its nearest
	// expanded parent and returns tree focus to that directory.
	tt.Key(ui.Shift, ui.KeyC)
	tt.Frame()
	if a.fileTreeState.IsExpanded(nested) || a.fileTreeState.FocusID() != nested || a.lastPanelAction != commands.ActionCloseNode {
		t.Fatalf("C did not close the focused node: expanded=%v focus=%q action=%q", a.fileTreeState.IsExpanded(nested), a.fileTreeState.FocusID(), a.lastPanelAction)
	}
	a.fileTreeState.SetFocusID(filepath.Join(dest, "note.md"))
	a.fileTreeState.SetExpanded(dest, true)
	tt.Key(ui.Shift, ui.KeyC)
	tt.Frame()
	if a.fileTreeState.IsExpanded(dest) || a.fileTreeState.FocusID() != dest {
		t.Fatalf("C did not close the nearest expanded parent: expanded=%v focus=%q", a.fileTreeState.IsExpanded(dest), a.fileTreeState.FocusID())
	}

	// z collapses every directory without changing row order or selection.
	a.fileTreeState.SetExpanded(dest, true)
	a.fileTreeState.SetExpanded(nested, true)
	a.fileTreeState.SetFocusID(nested)
	tt.Key(0, ui.KeyZ)
	tt.Frame()
	if a.fileTreeState.IsExpanded(dest) || a.fileTreeState.IsExpanded(nested) || a.lastPanelAction != commands.ActionCollapseAll {
		t.Fatalf("z did not collapse all nodes: expanded=%v action=%q", a.fileTreeState.ExpandedIDs(), a.lastPanelAction)
	}

	// Tab toggles the focused row without opening it; Ctrl+Shift+I inverts
	// only currently visible rows.
	focused := filepath.Join(root, "alpha.md")
	a.fileTreeState.SetFocusID(focused)
	beforeOpen := a.documentPath()
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if !a.fileTreeState.IsSelected(focused) || a.documentPath() != beforeOpen || a.lastPanelAction != commands.ActionToggleSelection {
		t.Fatalf("Tab did not toggle the row without opening it: selected=%v path=%q action=%q", a.fileTreeState.IsSelected(focused), a.documentPath(), a.lastPanelAction)
	}
	selectionBeforeInvert := make(map[string]bool, len(a.fileTreeState.SelectedIDs()))
	for _, path := range a.fileTreeState.SelectedIDs() {
		selectionBeforeInvert[path] = true
	}
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyI)
	tt.Frame()
	rows := a.fileRows()
	for _, row := range rows {
		if a.fileTreeState.IsSelected(row.Path) == selectionBeforeInvert[row.Path] {
			t.Fatalf("Ctrl+Shift+I failed to invert visible row %q: before=%v after=%v", row.Path, selectionBeforeInvert, a.fileTreeState.SelectedIDs())
		}
	}
	if a.lastPanelAction != commands.ActionInvertSelection {
		t.Fatalf("invert selection action=%q", a.lastPanelAction)
	}

	// Y copies a relative path through the existing generic CopyPath action,
	// while y remains the file-item clipboard action.
	copyPath := clipboardWrite
	copied := ""
	clipboardWrite = func(value string) { copied = value }
	t.Cleanup(func() { clipboardWrite = copyPath })
	a.fileTreeState.SetFocusID(focused)
	tt.Key(ui.Shift, ui.KeyY)
	tt.Frame()
	if copied != "alpha.md" || a.lastPanelAction != commands.ActionCopyPath {
		t.Fatalf("Y did not copy the relative path: copied=%q action=%q", copied, a.lastPanelAction)
	}
}

func TestVimTreeBindingsDoNotRunOutsideVimTreeFocus(t *testing.T) {
	root := keyTestWorkspace(t)
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	a.noteEditor().ModalCommands().SetKeymap("default")
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	before := len(a.fileTreeState.SelectedIDs())
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if a.lastPanelAction == commands.ActionToggleSelection || len(a.fileTreeState.SelectedIDs()) != before {
		t.Fatalf("Standard mode captured Vim Tab: action=%q selection=%v", a.lastPanelAction, a.fileTreeState.SelectedIDs())
	}
	a.noteEditor().ModalCommands().SetKeymap("vim")
	a.focusPanel(commands.PanelIDEditor)
	tt.Frame()
	path := filepath.Join(root, "alpha.md")
	a.fileTreeState.SetFocusID(path)
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if a.lastPanelAction == commands.ActionToggleSelection || a.fileTreeState.IsSelected(path) {
		t.Fatalf("editor focus captured Vim Tab: action=%q selection=%v", a.lastPanelAction, a.fileTreeState.SelectedIDs())
	}
	a.focusPanel(commands.PanelIDFiles)
	tt.Frame()
	a.settingsPanel.OpenAt(settingsui.AppearancePage)
	tt.Key(0, ui.KeyZ)
	tt.Frame()
	if a.lastPanelAction == commands.ActionCollapseAll {
		t.Fatal("modal allowed Vim tree operation")
	}
}

func TestFileTreeVimSequencesAreSplitByFocusContext(t *testing.T) {
	root := keyTestWorkspace(t)
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	a.noteEditor().ModalCommands().SetKeymap("vim")
	a.noteEditor().FocusCommands().Focus()
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	rows := a.fileRows()
	if len(rows) < 2 {
		t.Fatalf("fixture rows = %v", rows)
	}
	a.fileTreeState.SetFocusID(rows[len(rows)-1].Path)
	before := a.fileTreeState.FocusID()
	tt.Key(0, ui.KeyG)
	tt.Frame()
	tt.Key(0, ui.KeyG)
	tt.Frame()
	if a.fileTreeState.FocusID() != before || a.lastPanelAction == commands.ActionNavigate {
		t.Fatalf("editor Vim gg was captured by tree consumer: before=%q after=%q action=%q", before, a.fileTreeState.FocusID(), a.lastPanelAction)
	}
	// Ctrl+W h/l routes through the panel matcher, then actual toolkit focus.
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	if a.panelKeySequences == nil || !a.panelKeySequences.Pending() {
		t.Fatal("Vim Ctrl+W prefix was not retained by the panel matcher")
	}
	tt.Key(0, ui.KeyH)
	tt.Frame()
	if a.focusedPanel != commands.PanelIDFiles || !tt.Focused("文件树") {
		t.Fatalf("Ctrl+W h did not focus file tree: panel=%q tree=%v", a.focusedPanel, tt.Focused("文件树"))
	}
	// gg is now consumed by the tree matcher, not the global panel matcher.
	tt.Key(0, ui.KeyG)
	tt.Frame()
	if !a.treeKeySequences.Pending() {
		t.Fatal("tree gg prefix was not retained by the tree matcher")
	}
	tt.Key(0, ui.KeyG)
	tt.Frame()
	if a.fileTreeState.FocusID() != rows[0].Path || a.lastPanelAction != commands.ActionNavigate {
		t.Fatalf("tree gg did not navigate to first visible row: path=%q action=%q", a.fileTreeState.FocusID(), a.lastPanelAction)
	}
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	tt.Key(0, ui.KeyL)
	tt.Frame()
	if a.focusedPanel != commands.PanelIDEditor || !a.noteEditor().FocusCommands().Focused() || tt.Focused("文件树") {
		t.Fatalf("Ctrl+W l did not restore editor focus: panel=%q ed=%v tree=%v", a.focusedPanel, a.noteEditor().FocusCommands().Focused(), tt.Focused("文件树"))
	}
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	tt.Key(0, ui.KeyW)
	tt.Frame()
	if a.lastPanelAction != commands.ActionFocusNextPanel || a.focusedPanel != commands.PanelIDFiles || !tt.Focused("文件树") {
		t.Fatalf("Ctrl+W w did not cycle forward: action=%q panel=%q tree=%v", a.lastPanelAction, a.focusedPanel, tt.Focused("文件树"))
	}
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyW)
	tt.Frame()
	if a.lastPanelAction != commands.ActionFocusPreviousPanel || a.focusedPanel != commands.PanelIDEditor || !a.noteEditor().FocusCommands().Focused() {
		t.Fatalf("Ctrl+W W did not cycle backward: action=%q panel=%q editor=%v", a.lastPanelAction, a.focusedPanel, a.noteEditor().FocusCommands().Focused())
	}
	// A standard-mode tree never consumes Vim's gg tree-only sequence.
	a.noteEditor().ModalCommands().SetKeymap("default")
	tt.Frame()
	if !a.focusPanel(commands.PanelIDFiles) {
		t.Fatal("could not restore tree focus")
	}
	tt.Frame()
	standardTreeFocus := a.fileTreeState.FocusID()
	tt.Key(0, ui.KeyG)
	tt.Frame()
	tt.Key(0, ui.KeyG)
	tt.Frame()
	if a.fileTreeState.FocusID() != standardTreeFocus || a.lastPanelAction == commands.ActionNavigate {
		t.Fatalf("standard mode captured Vim gg: before=%q after=%q action=%q", standardTreeFocus, a.fileTreeState.FocusID(), a.lastPanelAction)
	}
	a.noteEditor().ModalCommands().SetKeymap("vim")
	tt.Frame()
	if !a.focusPanel(commands.PanelIDEditor) {
		t.Fatal("could not restore editor focus")
	}
	tt.Frame()
	// The prefix is inactive in Vim Insert mode.
	tt.Type("i")
	tt.Frame()
	if !a.noteEditor().ModalCommands().ModalAcceptsText() {
		t.Fatal("fixture did not enter Vim Insert mode")
	}
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	if a.panelKeySequences.Pending() || a.focusedPanel != commands.PanelIDEditor {
		t.Fatalf("Ctrl+W was intercepted in Vim Insert mode: pending=%v panel=%q", a.panelKeySequences.Pending(), a.focusedPanel)
	}
}

func TestTreeSequencePrefixesAreCanceledByPointerFocusTransitions(t *testing.T) {
	root := keyTestWorkspace(t)
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	a.noteEditor().ModalCommands().SetKeymap("vim")
	a.loadDoc("line one\n\nline two")
	a.noteEditor().FocusCommands().Focus()
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()

	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	if a.panelKeySequences == nil || !a.panelKeySequences.Pending() {
		t.Fatal("Ctrl+W did not start a pending panel prefix")
	}
	clickTreeRow(t, tt, "dest")
	tt.Frame()
	if a.focusedPanel != commands.PanelIDFiles || a.panelKeySequences.Pending() {
		t.Fatalf("tree pointer focus did not cancel Ctrl+W: panel=%q pending=%v handle=%v requested=%v focusPath=%q", a.focusedPanel, a.panelKeySequences.Pending(), tt.Focused("文件树"), a.treeFocusRequested, a.fileTreeState.FocusID())
	}
	tt.Key(0, ui.KeyH)
	tt.Frame()
	if a.focusedPanel != commands.PanelIDFiles || a.lastPanelAction != commands.ActionToggleExpanded {
		t.Fatalf("h after pointer focus completed stale Ctrl+W prefix: panel=%q action=%q", a.focusedPanel, a.lastPanelAction)
	}

	tt.Key(0, ui.KeyG)
	tt.Frame()
	if a.treeKeySequences == nil || !a.treeKeySequences.Pending() {
		t.Fatal("tree g did not start a pending gg prefix")
	}
	line, ok := tt.Find("文档")
	if !ok {
		t.Fatal("editor content target missing")
	}
	tt.ClickAt(line.X+line.W/2, line.Y+line.H/2)
	tt.Frame()
	if a.focusedPanel != commands.PanelIDEditor || a.treeKeySequences.Pending() || !a.noteEditor().FocusCommands().Focused() {
		t.Fatalf("editor pointer focus did not cancel gg: panel=%q pending=%v editor=%v rect=%+v focusedText=%v texts=%v", a.focusedPanel, a.treeKeySequences.Pending(), a.noteEditor().FocusCommands().Focused(), line, tt.Focused("文档"), tt.Texts())
	}
	clickTreeRow(t, tt, "dest")
	tt.Frame()
	if a.focusedPanel != commands.PanelIDFiles {
		t.Fatalf("pointer did not return focus to tree: %q", a.focusedPanel)
	}
	tt.Key(0, ui.KeyG)
	tt.Frame()
	if !a.treeKeySequences.Pending() || a.lastPanelAction == commands.ActionNavigate {
		t.Fatalf("g after return completed stale gg prefix: pending=%v action=%q path=%q", a.treeKeySequences.Pending(), a.lastPanelAction, a.fileTreeState.FocusID())
	}
}

func TestTreeContextMenuClipboardActionsUseSelectionAndFocusedTarget(t *testing.T) {
	root := keyTestWorkspace(t)
	dest := filepath.Join(root, "dest")
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	a.loadDoc("editor text remains untouched")
	tt := ui.NewTester(a.View, 1000, 720)
	tt.SetClipboard("system clipboard stays unchanged")
	tt.Frame()
	rightClickTreeRow(t, tt, "alpha.md")
	if contains(tt.Menu(), "粘贴项目") {
		t.Fatal("Paste should be absent when the internal file clipboard is empty")
	}
	tt.CloseMenu()
	clickTreeRow(t, tt, "alpha.md")
	if err := tt.ClickWith(ui.Cmd, treeDragHandleLabel("beta.md")); err != nil {
		t.Fatal(err)
	}
	rightClickTreeRow(t, tt, "beta.md")
	editorSource := a.noteEditor().Source()
	if !contains(tt.Menu(), "复制项目") {
		t.Fatalf("context menu omitted copy: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("复制项目"); err != nil {
		t.Fatal(err)
	}
	if a.lastPanelAction != commands.ActionCopy || len(a.fileTreeState.Clipboard().IDs) != 2 || a.fileTreeState.Clipboard().Kind == filetree.ClipboardCut {
		t.Fatalf("context Copy did not dispatch selected group: action=%q clipboard=%+v cut=%v", a.lastPanelAction, a.fileTreeState.Clipboard().IDs, a.fileTreeState.Clipboard().Kind == filetree.ClipboardCut)
	}
	if a.noteEditor().Source() != editorSource {
		t.Fatal("tree item Copy wrote into editor text")
	}
	if tt.Clipboard() != "system clipboard stays unchanged" {
		t.Fatalf("tree item Copy changed the text clipboard: %q", tt.Clipboard())
	}

	rightClickTreeRow(t, tt, "dest")
	if !contains(tt.Menu(), "粘贴项目") {
		t.Fatalf("context menu omitted internal-clipboard Paste: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("粘贴项目"); err != nil {
		t.Fatal(err)
	}
	if a.lastPanelAction != commands.ActionPaste {
		t.Fatalf("context Paste action=%q", a.lastPanelAction)
	}
	for _, name := range []string{"alpha.md", "beta.md"} {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Fatalf("Paste to focused directory missed %s: %v", name, err)
		}
	}

	// Context Cut retains a selected group and Paste into a folder moves it.
	clickTreeRow(t, tt, "alpha.md")
	if err := tt.ClickWith(ui.Cmd, treeDragHandleLabel("beta.md")); err != nil {
		t.Fatal(err)
	}
	rightClickTreeRow(t, tt, "beta.md")
	if err := tt.ChooseMenuItem("剪切项目"); err != nil {
		t.Fatal(err)
	}
	if a.lastPanelAction != commands.ActionCut || a.fileTreeState.Clipboard().Kind != filetree.ClipboardCut || len(a.fileTreeState.Clipboard().IDs) != 2 || !tt.HasText("剪切项目提示") {
		t.Fatalf("context Cut did not stage/mark selected group: action=%q clipboard=%+v cut=%v", a.lastPanelAction, a.fileTreeState.Clipboard().IDs, a.fileTreeState.Clipboard().Kind == filetree.ClipboardCut)
	}
	if tt.Clipboard() != "system clipboard stays unchanged" {
		t.Fatalf("tree item Cut changed the text clipboard: %q", tt.Clipboard())
	}
	rightClickTreeRow(t, tt, "dest")
	if err := tt.ChooseMenuItem("粘贴项目"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha.md", "beta.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("context Cut/Paste left source %s: %v", name, err)
		}
	}
	if len(a.fileTreeState.Clipboard().IDs) != 0 || a.fileTreeState.Clipboard().Kind == filetree.ClipboardCut || tt.HasText("剪切项目提示") {
		t.Fatalf("successful context Paste did not clear cut state: clipboard=%+v cut=%v texts=%v", a.fileTreeState.Clipboard().IDs, a.fileTreeState.Clipboard().Kind == filetree.ClipboardCut, tt.Texts())
	}
}

func TestTreeContextPasteIntoFileUsesParent(t *testing.T) {
	root := keyTestWorkspace(t)
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	a.fileTreeState.SetSelection([]string{filepath.Join(root, "alpha.md"), filepath.Join(root, "beta.md")})
	a.fileTreeState.SetFocusID(filepath.Join(root, "alpha.md"))
	if !a.dispatchPanelAction(commands.ActionCopy, nil) {
		t.Fatal("failed to populate internal clipboard")
	}
	rightClickTreeRow(t, tt, "alpha.md")
	if err := tt.ChooseMenuItem("粘贴项目"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha 2.md", "beta 2.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("paste to file parent did not create %s: %v", name, err)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
