package filetree

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestDragOperationDisplayLabels(t *testing.T) {
	for _, tc := range []struct {
		name    string
		display DragOperationDisplay
		want    string
	}{
		{"folder", DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: "Notes"}, locales.FilesMoveItemToFolder("Notes")},
		{"trash", DragOperationDisplay{Kind: DragOperationMoveToTrash}, locales.FilesMoveToTrash()},
		{"editor", DragOperationDisplay{Kind: DragOperationInsertIntoEditor}, locales.FilesInsertIntoEditor()},
		{"none", DragOperationDisplay{Kind: DragOperationNone}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.display.Label(); got != tc.want {
				t.Fatalf("Label() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRootDropPreviewOwnsVisibleRowRangeAndLabel(t *testing.T) {
	handle := NewHandle()
	root := "/vault"
	rows := []Row{{Path: "/vault/note.md"}, {Path: "/vault/Folder", Directory: true}}
	handle.SetDropTargetRange(root, 0, len(rows))
	handle.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: "vault"})
	if got := handle.DropTargetRange(); got.TargetID != root || got.StartRow != 0 || got.EndRow != len(rows) {
		t.Fatalf("root drop range = %+v, want [%d,%d)", got, 0, len(rows))
	}
	if display := handle.DragOperationDisplay(); display.Kind != DragOperationMoveToFolder || display.TargetLabel != "vault" {
		t.Fatalf("root operation display = %+v", display)
	}
}

func TestHandleProjectsVisibleSelectionAndCreateTarget(t *testing.T) {
	handle := NewHandle()
	rows := []Row{{Path: "root/a.md"}, {Path: "root/folder", Directory: true}, {Path: "root/b.md"}}
	handle.SetSelection([]string{"root/b.md", "root/a.md"})
	if got := handle.SelectedPaths(rows, "root/folder"); len(got) != 2 || got[0] != "root/a.md" || got[1] != "root/b.md" {
		t.Fatalf("SelectedPaths() = %v, want visible row order [root/a.md root/b.md]", got)
	}
	handle.SetSelection(nil)
	if got := handle.SelectedPaths(rows, "root/folder"); len(got) != 1 || got[0] != "root/folder" {
		t.Fatalf("SelectedPaths() fallback = %v, want focused row", got)
	}
	if got := handle.CreateTarget("root", "root/folder", true); got != "root/folder" {
		t.Fatalf("CreateTarget(selected directory) = %q, want selected directory", got)
	}
	if got := handle.CreateTarget("root", "root/a.md", false); got != "root" {
		t.Fatalf("CreateTarget(selected file) = %q, want vault root", got)
	}
}

func TestRemoveSelectedKeepsOtherSelection(t *testing.T) {
	handle := NewHandle()
	handle.SetSelection([]string{"a.md", "b.md", "c.md"})
	handle.RemoveSelected("b.md")
	if got := handle.SelectedIDs(); len(got) != 2 || got[0] != "a.md" || got[1] != "c.md" {
		t.Fatalf("selection after removing b.md = %v, want [a.md c.md]", got)
	}
	handle.RemoveSelected("missing.md")
	if got := handle.SelectedIDs(); len(got) != 2 || got[0] != "a.md" || got[1] != "c.md" {
		t.Fatalf("selection after removing an unselected ID = %v, want unchanged", got)
	}
}

func TestSelectIDMaintainsAnchorFocusAndModifiers(t *testing.T) {
	state := newState()
	entries := []Entry{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}
	state.SetSelection([]string{"b"})

	state.SelectID("d", ui.Shift, entries)
	for _, id := range []string{"b", "c", "d"} {
		if !state.IsSelected(id) {
			t.Errorf("shift selection omitted %q: %v", id, state.SelectedIDs())
		}
	}
	if state.FocusID() != "d" || *state.list.Selected != 3 {
		t.Fatalf("focus/list index = %q/%d, want d/3", state.FocusID(), *state.list.Selected)
	}

	state.SelectID("c", ui.Cmd, entries)
	if state.IsSelected("c") || !state.IsSelected("b") || !state.IsSelected("d") {
		t.Fatalf("Cmd selection = %v, want b and d", state.SelectedIDs())
	}
	if state.FocusID() != "c" {
		t.Fatalf("focus ID = %q, want c", state.FocusID())
	}

	state.SelectID("a", 0, entries)
	if !state.IsSelected("a") || len(state.SelectedIDs()) != 1 || state.FocusID() != "a" {
		t.Fatalf("plain selection = %v focus=%q, want only a", state.SelectedIDs(), state.FocusID())
	}
}

func TestApplyKeyOwnsKeyboardTreeTransitions(t *testing.T) {
	state := newState()
	rows := []Row{
		{Path: "root/folder", Directory: true},
		{Path: "root/folder/note.md", Depth: 1},
		{Path: "root/other.md"},
	}

	if !state.ApplyKey(KeyCommand{Operation: KeyNavigate, Delta: 1}, rows) || state.FocusID() != rows[1].Path {
		t.Fatalf("navigate focus = %q, want %q", state.FocusID(), rows[1].Path)
	}
	if !state.ApplyKey(KeyCommand{Operation: KeySelectRange, Delta: 1}, rows) || !state.IsSelected(rows[1].Path) || !state.IsSelected(rows[2].Path) {
		t.Fatalf("range selection = %v, want visible rows 1 and 2", state.SelectedIDs())
	}
	state.ApplyKey(KeyCommand{Operation: KeyFocus, TargetID: rows[0].Path}, rows)
	state.ApplyKey(KeyCommand{Operation: KeyToggleExpanded, TargetID: rows[0].Path}, rows)
	if !state.ApplyKey(KeyCommand{Operation: KeyToggleExpanded, Direction: -1}, rows) || state.IsExpanded(rows[0].Path) {
		t.Fatalf("left command did not collapse focused folder: expanded=%v focus=%q", state.ExpandedIDs(), state.FocusID())
	}
	if !state.ApplyKey(KeyCommand{Operation: KeySelectAll}, rows) || len(state.SelectedIDs()) != len(rows) {
		t.Fatalf("select all = %v, want all visible rows", state.SelectedIDs())
	}
	if !state.ApplyKey(KeyCommand{Operation: KeyInvertSelection}, rows) || len(state.SelectedIDs()) != 0 {
		t.Fatalf("invert = %v, want empty selection", state.SelectedIDs())
	}
}

func TestRowsProjectionCacheFollowsExpansionAndRootLifecycle(t *testing.T) {
	handle := NewHandle()
	childrenByPath := map[string][]CatalogEntry{
		"root":       {{Path: "notes", Kind: CatalogDirectory}, {Path: "Home.md", Kind: CatalogFile}},
		"root/notes": {{Path: "notes/Child.md", Kind: CatalogFile}},
	}
	catalog := CatalogPort{Children: func(path string) []CatalogEntry { return childrenByPath[path] }}

	first := BuildRows(handle, "root", catalog)
	second := BuildRows(handle, "root", catalog)
	if len(first) != 2 || &first[0] != &second[0] {
		t.Fatalf("unchanged rows were not reused: first=%p second=%p", &first[0], &second[0])
	}
	handle.SetExpanded("root/notes", true)
	expanded := BuildRows(handle, "root", catalog)
	if len(expanded) != 3 || expanded[1].Path != "root/notes/Child.md" || expanded[1].Depth != 1 {
		t.Fatalf("expanded rows = %+v", expanded)
	}
	BuildRows(handle, "", catalog)
	childrenByPath["root"] = []CatalogEntry{{Path: "New.md", Kind: CatalogFile}}
	reopened := BuildRows(handle, "root", catalog)
	if len(reopened) != 1 || reopened[0].Path != "root/New.md" {
		t.Fatalf("rows after closing and reopening same root = %+v", reopened)
	}
}

func TestPreviewCanBeSetAndClearedByHost(t *testing.T) {
	state := newState()
	bounds := ui.Rect{X: 12, Y: 34, W: 56, H: 78}
	state.SetPreview("image.png", bounds)
	if state.PreviewID() != "image.png" || state.PreviewBounds() != bounds {
		t.Fatalf("preview = %q %+v, want image.png %+v", state.PreviewID(), state.PreviewBounds(), bounds)
	}
	state.SetPreview("", bounds)
	if state.PreviewID() != "" || state.PreviewBounds() != (ui.Rect{}) {
		t.Fatalf("empty preview ID did not clear state: %q %+v", state.PreviewID(), state.PreviewBounds())
	}
	state.SetPreview("image.png", bounds)
	state.ClearPreview()
	if state.PreviewID() != "" || state.PreviewBounds() != (ui.Rect{}) {
		t.Fatalf("ClearPreview left state: %q %+v", state.PreviewID(), state.PreviewBounds())
	}
}

func TestPreviewLeaveDeadlineAndPopupHoverLifecycle(t *testing.T) {
	state := newState()
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	state.SetPreview("image.png", ui.Rect{X: 1, Y: 2, W: 3, H: 4})
	state.BeginPreviewLeave(now, 220*time.Millisecond)
	deadline := now.Add(220 * time.Millisecond)
	if got := state.PreviewLeaveAt(); !got.Equal(deadline) {
		t.Fatalf("leave deadline = %v, want %v", got, deadline)
	}
	if state.PreviewLeaveExpired(deadline.Add(-time.Nanosecond)) || !state.PreviewLeaveExpired(deadline) {
		t.Fatal("preview leave expiration did not respect the deadline")
	}

	state.SetPreviewPopupHovered(true)
	if !state.PreviewPopupHovered() || !state.PreviewLeaveAt().IsZero() || state.PreviewLeaveExpired(deadline.Add(time.Second)) {
		t.Fatal("hovering preview popup did not cancel pending leave")
	}
	state.SetPreviewPopupHovered(false)
	state.BeginPreviewLeave(deadline, 220*time.Millisecond)
	if !state.PreviewLeaveExpired(deadline.Add(220 * time.Millisecond)) {
		t.Fatal("preview leave did not expire after leaving popup")
	}

	state.CancelPreviewLeave()
	if state.PreviewLeaveExpired(deadline.Add(time.Hour)) || !state.PreviewLeaveAt().IsZero() {
		t.Fatal("CancelPreviewLeave retained a deadline")
	}
	state.BeginPreviewLeave(now, 220*time.Millisecond)
	state.ClearPreview()
	if state.PreviewID() != "" || !state.PreviewLeaveAt().IsZero() || state.PreviewPopupHovered() {
		t.Fatal("ClearPreview retained leave lifecycle state")
	}
}

func TestLeaveActionLeavesPreviewForHostDeadlinePolicy(t *testing.T) {
	state := newState()
	state.SetPreview("image.png", ui.Rect{X: 1, Y: 2, W: 3, H: 4})
	applyPresentationAction(state, LeaveEntry, "image.png", 0, ui.Rect{}, nil)
	if state.PreviewID() != "image.png" || !state.PreviewLeaveAt().IsZero() {
		t.Fatalf("LeaveEntry changed preview state before host policy: id=%q deadline=%v", state.PreviewID(), state.PreviewLeaveAt())
	}
	state.BeginPreviewLeave(time.Now(), 220*time.Millisecond)
	if state.PreviewLeaveAt().IsZero() {
		t.Fatal("host could not schedule delayed preview leave")
	}
}

func TestRemapIDsPreservesAndDeduplicatesPresentationState(t *testing.T) {
	state := newState()
	entries := []Entry{{ID: "old"}, {ID: "keep"}, {ID: "drop"}}
	state.SetExpanded("old", true)
	state.SetExpanded("drop", true)
	state.SetSelection([]string{"old", "keep", "drop"})
	state.SetFocusID("old")
	state.visibleIDs = entryIDs(entries)
	state.syncListFocus()
	state.SetClipboard(ClipboardCut, []string{"old", "old-copy", "drop"})
	bounds := ui.Rect{X: 10, Y: 20, W: 30, H: 40}
	state.SetPreview("old", bounds)
	state.SetDragPreview(DragPreviewState{Active: true, ItemID: "old"})
	state.SetPointerPress("old", ui.Cmd|ui.Shift, ui.Point{X: 11, Y: 12})
	state.SetPointerPressApplied(true)
	state.SetPointerReleased(true)
	state.SetDropHover(DropHoverState{TargetID: "old", EnteredAt: time.Unix(100, 0), AutoExpandID: "drop", AutoExpandAt: time.Unix(101, 0)})
	state.SetDropTargetRange("old", 2, 5)
	state.SetTrashTargetActive(true)
	state.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: "drop"})

	state.RemapIDs(func(id string) string {
		switch id {
		case "old", "old-copy":
			return "renamed"
		case "drop":
			return ""
		default:
			return id
		}
	})

	if !state.IsExpanded("renamed") || state.IsExpanded("drop") {
		t.Fatalf("expanded IDs = %v, want renamed only", state.ExpandedIDs())
	}
	if got := state.SelectedIDs(); len(got) != 2 || !state.IsSelected("renamed") || !state.IsSelected("keep") || state.IsSelected("drop") {
		t.Fatalf("selected IDs = %v, want deduplicated renamed and keep", got)
	}
	if state.FocusID() != "renamed" || state.selectionAnchor != "renamed" || *state.list.Selected != 0 {
		t.Fatalf("focus/anchor/list = %q/%q/%d, want renamed/renamed/0", state.FocusID(), state.selectionAnchor, *state.list.Selected)
	}
	if clip := state.Clipboard(); clip.Kind != ClipboardCut || len(clip.IDs) != 1 || clip.IDs[0] != "renamed" {
		t.Fatalf("clipboard = %+v, want one renamed cut ID", clip)
	}
	if state.PreviewID() != "renamed" || state.PreviewBounds() != bounds {
		t.Fatalf("preview = %q %+v, want renamed %+v", state.PreviewID(), state.PreviewBounds(), bounds)
	}
	if drag := state.DragPreview(); drag.ItemID != "renamed" || !drag.Active {
		t.Fatalf("drag preview = %+v, want active renamed item", drag)
	}
	if press := state.PointerPress(); press.ID != "renamed" || press.Modifiers != ui.Cmd|ui.Shift || press.Start != (ui.Point{X: 11, Y: 12}) || !press.Applied || !state.PointerReleased() {
		t.Fatalf("pointer press/release = %+v/%v, want remapped applied press and release", press, state.PointerReleased())
	}
	if hover := state.DropHover(); hover.TargetID != "renamed" || hover.AutoExpandID != "" || !hover.AutoExpandAt.IsZero() || !hover.EnteredAt.Equal(time.Unix(100, 0)) {
		t.Fatalf("drop hover = %+v, want remapped target with cleared removed auto-expand ID", hover)
	}
	if dropRange := state.DropTargetRange(); dropRange != (DropTargetRange{TargetID: "renamed", StartRow: 2, EndRow: 5}) {
		t.Fatalf("drop row range = %+v", dropRange)
	}
	if !state.TrashTargetActive() || state.DragOperationDisplay() != (DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: "drop"}) {
		t.Fatalf("trash/operation display = %v/%+v", state.TrashTargetActive(), state.DragOperationDisplay())
	}
}

func TestRemapIDsClearsReferencesMappedToEmpty(t *testing.T) {
	state := newState()
	state.SetSelection([]string{"gone"})
	state.SetFocusID("gone")
	state.visibleIDs = []string{"gone"}
	state.syncListFocus()
	state.SetClipboard(ClipboardCopy, []string{"gone"})
	state.SetPreview("gone", ui.Rect{X: 5, Y: 6, W: 7, H: 8})
	state.SetDragPreview(DragPreviewState{Active: true, ItemID: "gone"})
	state.SetPointerPress("gone", ui.Cmd, ui.Point{X: 1, Y: 2})
	state.SetPointerPressApplied(true)
	state.SetDropHover(DropHoverState{TargetID: "gone", EnteredAt: time.Now(), AutoExpandAt: time.Now()})
	state.SetDropTargetRange("gone", 0, 1)
	state.RemapIDs(func(string) string { return "" })
	if state.FocusID() != "" || state.PreviewID() != "" || state.PreviewBounds() != (ui.Rect{}) || len(state.SelectedIDs()) != 0 || state.Clipboard().Kind != 0 || state.DragPreview().ItemID != "" || state.PointerPress() != (PointerPressState{}) || state.DropHover() != (DropHoverState{}) || state.DropTargetRange() != (DropTargetRange{}) || *state.list.Selected != -1 {
		t.Fatalf("empty remap retained stale state: selected=%v focus=%q preview=%q drag=%+v press=%+v drop=%+v range=%+v clip=%+v list=%d", state.SelectedIDs(), state.FocusID(), state.PreviewID(), state.DragPreview(), state.PointerPress(), state.DropHover(), state.DropTargetRange(), state.Clipboard(), *state.list.Selected)
	}
}

func TestGesturePresentationStateSnapshotsAndDropDeadline(t *testing.T) {
	state := newState()
	start := ui.Point{X: 4, Y: 9}
	state.SetPointerPress("item.md", ui.Shift, start)
	if got := state.PointerPress(); got.ID != "item.md" || got.Modifiers != ui.Shift || got.Start != start || got.Applied {
		t.Fatalf("pointer press = %+v", got)
	}
	state.SetPointerPressApplied(true)
	state.SetPointerReleased(true)
	if !state.PointerPress().Applied || !state.PointerReleased() {
		t.Fatal("pointer press/release state was not updated")
	}

	entered := time.Unix(200, 0)
	deadline := entered.Add(300 * time.Millisecond)
	state.SetDropHover(DropHoverState{TargetID: "folder", EnteredAt: entered, AutoExpandAt: deadline})
	hover := state.DropHover()
	if hover.TargetID != "folder" || hover.AutoExpandID != "folder" || !hover.EnteredAt.Equal(entered) || !hover.AutoExpandAt.Equal(deadline) {
		t.Fatalf("drop hover snapshot = %+v", hover)
	}
	if state.DropAutoExpandExpired(deadline.Add(-time.Nanosecond)) || !state.DropAutoExpandExpired(deadline) {
		t.Fatal("drop auto-expand expiry did not respect its deadline")
	}

	state.SetDropTargetRange("folder", 2, 6)
	if got := state.DropTargetRange(); got != (DropTargetRange{TargetID: "folder", StartRow: 2, EndRow: 6}) {
		t.Fatalf("drop range = %+v", got)
	}
	state.SetDropTargetRange("", 0, 0)
	state.SetTrashTargetActive(true)
	state.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToTrash, TargetLabel: "Trash"})
	if state.DropTargetRange() != (DropTargetRange{}) || !state.TrashTargetActive() || state.DragOperationDisplay() != (DragOperationDisplay{Kind: DragOperationMoveToTrash, TargetLabel: "Trash"}) {
		t.Fatalf("gesture presentation state mismatch: range=%+v trash=%v operation=%+v", state.DropTargetRange(), state.TrashTargetActive(), state.DragOperationDisplay())
	}

	state.ClearDropHover()
	state.ClearPointerPress()
	state.SetPointerReleased(false)
	state.SetTrashTargetActive(false)
	state.SetDragOperationDisplay(DragOperationDisplay{})
	if state.DropHover() != (DropHoverState{}) || state.PointerPress() != (PointerPressState{}) || state.PointerReleased() || state.TrashTargetActive() || state.DragOperationDisplay() != (DragOperationDisplay{}) {
		t.Fatal("gesture presentation clear operations retained state")
	}
}
