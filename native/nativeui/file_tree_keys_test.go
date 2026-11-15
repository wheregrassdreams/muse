package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/nativeui/interactions"
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
	a := New()
	a.setWorkspace(root)
	a.ed.SetSource("line one\nline two\nline three")
	a.ed.SetKeymap("vim")
	a.ed.Focus()
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	before := a.ed.SelectionRange()

	tt.Key(0, ui.KeyJ)
	tt.Frame()
	if a.focusedPanel != interactions.PanelIDEditor || a.lastPanelAction != "" {
		t.Fatalf("Vim j with editor focus reached tree: panel=%q action=%q", a.focusedPanel, a.lastPanelAction)
	}
	if a.treeFocusPath != "" {
		t.Fatalf("editor Vim navigation changed tree focus: %q", a.treeFocusPath)
	}

	tt.Key(ui.Cmd|ui.Shift, ui.KeyE)
	tt.Frame()
	if !tt.Focused("文件树") || a.focusedPanel != interactions.PanelIDFiles {
		t.Fatalf("focus-panel shortcut did not transfer toolkit focus: treeFocused=%v appPanel=%q", tt.Focused("文件树"), a.focusedPanel)
	}
	rows := a.fileRows()
	if len(rows) < 2 || a.treeFocusPath != rows[0].path {
		t.Fatalf("initial tree focus = %q rows=%v", a.treeFocusPath, rows)
	}
	next := rows[1].path
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.treeFocusPath != next || a.lastPanelAction != interactions.ActionNavigate {
		t.Fatalf("tree Down did not dispatch navigation: focus=%q action=%q", a.treeFocusPath, a.lastPanelAction)
	}
	if got := a.ed.SelectionRange(); got != before || a.ed.Source() != "line one\nline two\nline three" {
		t.Fatalf("tree navigation changed editor selection/source: range=%+v source=%q", got, a.ed.Source())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.path != next || a.lastPanelAction != interactions.ActionOpen {
		t.Fatalf("tree Enter did not open focused file through interactions.ActionOpen: path=%q action=%q", a.path, a.lastPanelAction)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if tt.Focused("文件树") || !a.ed.Focused() || a.focusedPanel != interactions.PanelIDEditor {
		t.Fatalf("Escape did not restore editor toolkit focus: treeFocused=%v editorFocused=%v panel=%q", tt.Focused("文件树"), a.ed.Focused(), a.focusedPanel)
	}
}

func TestFileTreeMultiCopyCutPasteUsesAppClipboard(t *testing.T) {
	root := keyTestWorkspace(t)
	dest := filepath.Join(root, "dest")
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	tt.Key(ui.Cmd|ui.Shift, ui.KeyE)
	tt.Frame()
	a.treeSelection = map[string]bool{filepath.Join(root, "alpha.md"): true, filepath.Join(root, "beta.md"): true}
	a.treeFocusPath = filepath.Join(root, "alpha.md")
	tt.Key(ui.Cmd, ui.KeyC)
	tt.Frame()
	if a.lastPanelAction != interactions.ActionCopy || len(a.treeClipboard) != 2 || a.treeClipboardCut {
		t.Fatalf("Cmd+C did not capture selected files in app clipboard: action=%q clipboard=%+v cut=%v", a.lastPanelAction, a.treeClipboard, a.treeClipboardCut)
	}
	a.treeFocusPath = dest
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	if a.lastPanelAction != interactions.ActionPaste {
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

	a.treeSelection = map[string]bool{filepath.Join(root, "alpha.md"): true, filepath.Join(root, "beta.md"): true}
	a.treeFocusPath = filepath.Join(root, "alpha.md")
	tt.Key(ui.Cmd, ui.KeyX)
	tt.Frame()
	if a.lastPanelAction != interactions.ActionCut || !a.treeClipboardCut || len(a.treeClipboard) != 2 {
		t.Fatalf("Cmd+X did not stage cut selection: %+v cut=%v", a.treeClipboard, a.treeClipboardCut)
	}
	if !tt.HasText("剪切项目提示") {
		t.Fatal("cut selection has no visible row marker")
	}
	a.treeFocusPath = filepath.Join(root, "dest")
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	for _, name := range []string{"alpha.md", "beta.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("cut paste left source %s: err=%v", name, err)
		}
	}
	if len(a.treeClipboard) != 0 || a.treeClipboardCut {
		t.Fatalf("successful cut paste did not clear clipboard: %+v cut=%v", a.treeClipboard, a.treeClipboardCut)
	}
	if tt.HasText("剪切项目提示") {
		t.Fatal("cut marker remained after successful paste")
	}
}

func TestFileTreeModalKeepsShortcutsFromTree(t *testing.T) {
	root := keyTestWorkspace(t)
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	a.settingsOpen = true
	tt.Key(ui.Cmd|ui.Shift, ui.KeyE)
	tt.Frame()
	if a.focusedPanel != interactions.PanelIDEditor || a.lastPanelAction != "" || tt.Focused("文件树") {
		t.Fatalf("modal did not retain keyboard priority: panel=%q action=%q treeFocused=%v", a.focusedPanel, a.lastPanelAction, tt.Focused("文件树"))
	}
}

func TestTreeBindingConsumersMapToSemanticActions(t *testing.T) {
	cases := []struct {
		name string
		got  TreeBinding
		want interactions.ActionID
	}{
		{"down", StandardTreeBindings{}.Resolve(0, ui.KeyDown), interactions.ActionNavigate},
		{"shift-range", StandardTreeBindings{}.Resolve(ui.Shift, ui.KeyDown), interactions.ActionSelectRange},
		{"enter", StandardTreeBindings{}.Resolve(0, ui.KeyEnter), interactions.ActionOpen},
		{"rename", StandardTreeBindings{}.Resolve(0, ui.KeyF2), interactions.ActionRename},
		{"copy", StandardTreeBindings{}.Resolve(ui.Cmd, ui.KeyC), interactions.ActionCopy},
		{"cut", StandardTreeBindings{}.Resolve(ui.Cmd, ui.KeyX), interactions.ActionCut},
		{"paste", StandardTreeBindings{}.Resolve(ui.Cmd, ui.KeyV), interactions.ActionPaste},
		{"delete", StandardTreeBindings{}.Resolve(0, ui.KeyDelete), interactions.ActionTrash},
		{"vim-j", (VimTreeBindings{}).Resolve(0, ui.KeyJ), interactions.ActionNavigate},
		{"vim-range-j", (VimTreeBindings{}).Resolve(ui.Shift, ui.KeyJ), interactions.ActionSelectRange},
		{"vim-delete", (VimTreeBindings{}).Resolve(0, ui.KeyD), interactions.ActionTrash},
		{"vim-close-node", (VimTreeBindings{}).Resolve(ui.Shift, ui.KeyC), interactions.ActionCloseNode},
		{"vim-collapse-all", (VimTreeBindings{}).Resolve(0, ui.KeyZ), interactions.ActionCollapseAll},
		{"vim-toggle-selection", (VimTreeBindings{}).Resolve(0, ui.KeyTab), interactions.ActionToggleSelection},
		{"vim-invert-selection", (VimTreeBindings{}).Resolve(ui.Ctrl|ui.Shift, ui.KeyI), interactions.ActionInvertSelection},
		{"vim-copy-path", (VimTreeBindings{}).Resolve(ui.Shift, ui.KeyY), interactions.ActionCopyPath},
		{"vim-a-file", (VimTreeBindings{}).Resolve(0, ui.KeyA), interactions.ActionCreateFile},
		{"vim-A-folder", (VimTreeBindings{}).Resolve(ui.Shift, ui.KeyA), interactions.ActionCreateFolder},
		{"vim-help", (VimTreeBindings{}).Resolve(ui.Shift, ui.KeySlash), interactions.ActionShowHelp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.got.Handled || tc.got.Action != tc.want {
				t.Fatalf("binding = %+v, want %q", tc.got, tc.want)
			}
		})
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
	a := New()
	a.setWorkspace(root)
	a.ed.SetKeymap("vim")
	a.expanded[dest] = true
	a.expanded[nested] = true
	a.treeFocusPath = nested
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	if !a.focusPanel(interactions.PanelIDFiles) {
		t.Fatal("could not focus tree")
	}
	tt.Frame()

	// C closes the focused directory; from an item it closes its nearest
	// expanded parent and returns tree focus to that directory.
	tt.Key(ui.Shift, ui.KeyC)
	tt.Frame()
	if a.expanded[nested] || a.treeFocusPath != nested || a.lastPanelAction != interactions.ActionCloseNode {
		t.Fatalf("C did not close the focused node: expanded=%v focus=%q action=%q", a.expanded[nested], a.treeFocusPath, a.lastPanelAction)
	}
	a.treeFocusPath = filepath.Join(dest, "note.md")
	a.expanded[dest] = true
	tt.Key(ui.Shift, ui.KeyC)
	tt.Frame()
	if a.expanded[dest] || a.treeFocusPath != dest {
		t.Fatalf("C did not close the nearest expanded parent: expanded=%v focus=%q", a.expanded[dest], a.treeFocusPath)
	}

	// z collapses every directory without changing row order or selection.
	a.expanded[dest], a.expanded[nested] = true, true
	a.treeFocusPath = nested
	tt.Key(0, ui.KeyZ)
	tt.Frame()
	if a.expanded[dest] || a.expanded[nested] || a.lastPanelAction != interactions.ActionCollapseAll {
		t.Fatalf("z did not collapse all nodes: expanded=%v action=%q", a.expanded, a.lastPanelAction)
	}

	// Tab toggles the focused row without opening it; Ctrl+Shift+I inverts
	// only currently visible rows.
	focused := filepath.Join(root, "alpha.md")
	a.treeFocusPath = focused
	beforeOpen := a.path
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if !a.treeSelection[focused] || a.path != beforeOpen || a.lastPanelAction != interactions.ActionToggleSelection {
		t.Fatalf("Tab did not toggle the row without opening it: selected=%v path=%q action=%q", a.treeSelection[focused], a.path, a.lastPanelAction)
	}
	selectionBeforeInvert := make(map[string]bool, len(a.treeSelection))
	for path, selected := range a.treeSelection {
		selectionBeforeInvert[path] = selected
	}
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyI)
	tt.Frame()
	rows := a.fileRows()
	for _, row := range rows {
		if a.treeSelection[row.path] == selectionBeforeInvert[row.path] {
			t.Fatalf("Ctrl+Shift+I failed to invert visible row %q: before=%v after=%v", row.path, selectionBeforeInvert, a.treeSelection)
		}
	}
	if a.lastPanelAction != interactions.ActionInvertSelection {
		t.Fatalf("invert selection action=%q", a.lastPanelAction)
	}

	// Y copies a relative path through the existing generic CopyPath action,
	// while y remains the file-item clipboard action.
	copyPath := clipboardWrite
	copied := ""
	clipboardWrite = func(value string) { copied = value }
	t.Cleanup(func() { clipboardWrite = copyPath })
	a.treeFocusPath = focused
	tt.Key(ui.Shift, ui.KeyY)
	tt.Frame()
	if copied != "alpha.md" || a.lastPanelAction != interactions.ActionCopyPath {
		t.Fatalf("Y did not copy the relative path: copied=%q action=%q", copied, a.lastPanelAction)
	}
}

func TestVimTreeBindingsDoNotRunOutsideVimTreeFocus(t *testing.T) {
	root := keyTestWorkspace(t)
	a := New()
	a.setWorkspace(root)
	a.ed.SetKeymap("default")
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	before := len(a.treeSelection)
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if a.lastPanelAction == interactions.ActionToggleSelection || len(a.treeSelection) != before {
		t.Fatalf("Standard mode captured Vim Tab: action=%q selection=%v", a.lastPanelAction, a.treeSelection)
	}
	a.ed.SetKeymap("vim")
	a.focusPanel(interactions.PanelIDEditor)
	tt.Frame()
	path := filepath.Join(root, "alpha.md")
	a.treeFocusPath = path
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if a.lastPanelAction == interactions.ActionToggleSelection || a.treeSelection[path] {
		t.Fatalf("editor focus captured Vim Tab: action=%q selection=%v", a.lastPanelAction, a.treeSelection)
	}
	a.focusPanel(interactions.PanelIDFiles)
	tt.Frame()
	a.settingsOpen = true
	tt.Key(0, ui.KeyZ)
	tt.Frame()
	if a.lastPanelAction == interactions.ActionCollapseAll {
		t.Fatal("modal allowed Vim tree operation")
	}
}

func TestFileTreeVimSequencesAreSplitByFocusContext(t *testing.T) {
	root := keyTestWorkspace(t)
	a := New()
	a.setWorkspace(root)
	a.ed.SetKeymap("vim")
	a.ed.Focus()
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	rows := a.fileRows()
	if len(rows) < 2 {
		t.Fatalf("fixture rows = %v", rows)
	}
	a.treeFocusPath = rows[len(rows)-1].path
	before := a.treeFocusPath
	tt.Key(0, ui.KeyG)
	tt.Frame()
	tt.Key(0, ui.KeyG)
	tt.Frame()
	if a.treeFocusPath != before || a.lastPanelAction == interactions.ActionNavigate {
		t.Fatalf("editor Vim gg was captured by tree consumer: before=%q after=%q action=%q", before, a.treeFocusPath, a.lastPanelAction)
	}
	// Ctrl+W h/l routes through the panel matcher, then actual toolkit focus.
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	if a.panelKeySequences == nil || !a.panelKeySequences.Pending() {
		t.Fatal("Vim Ctrl+W prefix was not retained by the panel matcher")
	}
	tt.Key(0, ui.KeyH)
	tt.Frame()
	if a.focusedPanel != interactions.PanelIDFiles || !tt.Focused("文件树") {
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
	if a.treeFocusPath != rows[0].path || a.lastPanelAction != interactions.ActionNavigate {
		t.Fatalf("tree gg did not navigate to first visible row: path=%q action=%q", a.treeFocusPath, a.lastPanelAction)
	}
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	tt.Key(0, ui.KeyL)
	tt.Frame()
	if a.focusedPanel != interactions.PanelIDEditor || !a.ed.Focused() || tt.Focused("文件树") {
		t.Fatalf("Ctrl+W l did not restore editor focus: panel=%q ed=%v tree=%v", a.focusedPanel, a.ed.Focused(), tt.Focused("文件树"))
	}
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	tt.Key(0, ui.KeyW)
	tt.Frame()
	if a.lastPanelAction != interactions.ActionFocusNextPanel || a.focusedPanel != interactions.PanelIDFiles || !tt.Focused("文件树") {
		t.Fatalf("Ctrl+W w did not cycle forward: action=%q panel=%q tree=%v", a.lastPanelAction, a.focusedPanel, tt.Focused("文件树"))
	}
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyW)
	tt.Frame()
	if a.lastPanelAction != interactions.ActionFocusPreviousPanel || a.focusedPanel != interactions.PanelIDEditor || !a.ed.Focused() {
		t.Fatalf("Ctrl+W W did not cycle backward: action=%q panel=%q editor=%v", a.lastPanelAction, a.focusedPanel, a.ed.Focused())
	}
	// A standard-mode tree never consumes Vim's gg tree-only sequence.
	a.ed.SetKeymap("default")
	tt.Frame()
	if !a.focusPanel(interactions.PanelIDFiles) {
		t.Fatal("could not restore tree focus")
	}
	tt.Frame()
	standardTreeFocus := a.treeFocusPath
	tt.Key(0, ui.KeyG)
	tt.Frame()
	tt.Key(0, ui.KeyG)
	tt.Frame()
	if a.treeFocusPath != standardTreeFocus || a.lastPanelAction == interactions.ActionNavigate {
		t.Fatalf("standard mode captured Vim gg: before=%q after=%q action=%q", standardTreeFocus, a.treeFocusPath, a.lastPanelAction)
	}
	a.ed.SetKeymap("vim")
	tt.Frame()
	if !a.focusPanel(interactions.PanelIDEditor) {
		t.Fatal("could not restore editor focus")
	}
	tt.Frame()
	// The prefix is inactive in Vim Insert mode.
	tt.Type("i")
	tt.Frame()
	if !a.ed.ModalAcceptsText() {
		t.Fatal("fixture did not enter Vim Insert mode")
	}
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	if a.panelKeySequences.Pending() || a.focusedPanel != interactions.PanelIDEditor {
		t.Fatalf("Ctrl+W was intercepted in Vim Insert mode: pending=%v panel=%q", a.panelKeySequences.Pending(), a.focusedPanel)
	}
}

func TestTreeSequencePrefixesAreCanceledByPointerFocusTransitions(t *testing.T) {
	root := keyTestWorkspace(t)
	a := New()
	a.setWorkspace(root)
	a.ed.SetKeymap("vim")
	a.ed.SetSource("line one\n\nline two")
	a.ed.Focus()
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()

	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	if a.panelKeySequences == nil || !a.panelKeySequences.Pending() {
		t.Fatal("Ctrl+W did not start a pending panel prefix")
	}
	clickTreeRow(t, tt, "dest")
	tt.Frame()
	if a.focusedPanel != interactions.PanelIDFiles || a.panelKeySequences.Pending() {
		t.Fatalf("tree pointer focus did not cancel Ctrl+W: panel=%q pending=%v handle=%v requested=%v focusPath=%q", a.focusedPanel, a.panelKeySequences.Pending(), tt.Focused("文件树"), a.treeFocusRequested, a.treeFocusPath)
	}
	tt.Key(0, ui.KeyH)
	tt.Frame()
	if a.focusedPanel != interactions.PanelIDFiles || a.lastPanelAction != interactions.ActionToggleExpanded {
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
	if a.focusedPanel != interactions.PanelIDEditor || a.treeKeySequences.Pending() || !a.ed.Focused() {
		t.Fatalf("editor pointer focus did not cancel gg: panel=%q pending=%v editor=%v rect=%+v focusedText=%v texts=%v", a.focusedPanel, a.treeKeySequences.Pending(), a.ed.Focused(), line, tt.Focused("文档"), tt.Texts())
	}
	clickTreeRow(t, tt, "dest")
	tt.Frame()
	if a.focusedPanel != interactions.PanelIDFiles {
		t.Fatalf("pointer did not return focus to tree: %q", a.focusedPanel)
	}
	tt.Key(0, ui.KeyG)
	tt.Frame()
	if !a.treeKeySequences.Pending() || a.lastPanelAction == interactions.ActionNavigate {
		t.Fatalf("g after return completed stale gg prefix: pending=%v action=%q path=%q", a.treeKeySequences.Pending(), a.lastPanelAction, a.treeFocusPath)
	}
}

func TestTreeContextMenuClipboardActionsUseSelectionAndFocusedTarget(t *testing.T) {
	root := keyTestWorkspace(t)
	dest := filepath.Join(root, "dest")
	a := New()
	a.setWorkspace(root)
	a.ed.SetSource("editor text remains untouched")
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
	editorSource := a.ed.Source()
	if !contains(tt.Menu(), "复制项目") {
		t.Fatalf("context menu omitted copy: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("复制项目"); err != nil {
		t.Fatal(err)
	}
	if a.lastPanelAction != interactions.ActionCopy || len(a.treeClipboard) != 2 || a.treeClipboardCut {
		t.Fatalf("context Copy did not dispatch selected group: action=%q clipboard=%+v cut=%v", a.lastPanelAction, a.treeClipboard, a.treeClipboardCut)
	}
	if a.ed.Source() != editorSource {
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
	if a.lastPanelAction != interactions.ActionPaste {
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
	if a.lastPanelAction != interactions.ActionCut || !a.treeClipboardCut || len(a.treeClipboard) != 2 || !tt.HasText("剪切项目提示") {
		t.Fatalf("context Cut did not stage/mark selected group: action=%q clipboard=%+v cut=%v", a.lastPanelAction, a.treeClipboard, a.treeClipboardCut)
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
	if len(a.treeClipboard) != 0 || a.treeClipboardCut || tt.HasText("剪切项目提示") {
		t.Fatalf("successful context Paste did not clear cut state: clipboard=%+v cut=%v texts=%v", a.treeClipboard, a.treeClipboardCut, tt.Texts())
	}
}

func TestTreeContextPasteIntoFileUsesParent(t *testing.T) {
	root := keyTestWorkspace(t)
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	a.treeSelection = map[string]bool{filepath.Join(root, "alpha.md"): true, filepath.Join(root, "beta.md"): true}
	a.treeFocusPath = filepath.Join(root, "alpha.md")
	if !a.dispatchPanelAction(interactions.ActionCopy, nil) {
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
