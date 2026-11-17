package filetree

import (
	"path/filepath"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/ui/scrollbar"
)

// state owns transient file-tree presentation state. The catalog and all
// filesystem operations remain with the host application.
type state struct {
	expanded             map[string]bool
	selected             map[string]struct{}
	selectionAnchor      string
	focusID              string
	focused              bool
	visibleIDs           []string
	previewID            string
	previewBounds        ui.Rect
	previewLeaveAt       time.Time
	previewLeaveNotified bool
	previewPopupHovered  bool
	dragPreview          DragPreviewState
	pointerPress         PointerPressState
	pointerReleased      bool
	dropHover            DropHoverState
	dropTargetRange      DropTargetRange
	trashTargetActive    bool
	dragOperation        DragOperationDisplay
	clipboard            Clipboard
	list                 ui.ListState
	scroll               ui.ScrollState
	indicator            scrollbar.Indicator
	rows                 []Row
	rowsRoot             string
	rowsDirty            bool
}

// Handle is an opaque capability for transient file-tree presentation state.
// Its embedded implementation remains private to this package.
type Handle struct{ *state }

// NewHandle creates an opaque file-tree state capability for an application
// composition root.
func NewHandle() *Handle { return &Handle{state: newState()} }

// KeyOperation is a panel-owned keyboard intent that changes tree selection,
// focus, or expansion state. Filesystem and other host actions are deliberately
// absent from this command set.
type KeyOperation uint8

const (
	KeyEnsureFocus KeyOperation = iota + 1
	KeyFocus
	KeyNavigate
	KeySelectRange
	KeyToggleExpanded
	KeyCloseNode
	KeyCollapseAll
	KeySelectAll
	KeyToggleSelection
	KeyInvertSelection
)

// KeyCommand carries typed arguments for one panel-owned tree operation.
type KeyCommand struct {
	Operation    KeyOperation
	TargetID     string
	Delta        int
	Direction    int
	Index        int
	HasIndex     bool
	PreferredIDs []string
}

// ClipboardKind controls the presentation of paths held by the host clipboard.
// The paths themselves are identifiers only; this state never reads files.
type ClipboardKind uint8

const (
	ClipboardCopy ClipboardKind = iota + 1
	ClipboardCut
)

type Clipboard struct {
	Kind ClipboardKind
	IDs  []string
}

// DragPreviewState stores only the visual timeline and pointer for the active
// tree drag. The host supplies item labels, icons, and operation policy.
type DragPreviewState struct {
	Active    bool
	StartedAt time.Time
	Frames    int
	Pointer   ui.Point
	ItemID    string
}

// PointerPressState tracks only pointer gesture presentation and threshold
// bookkeeping. The host decides whether a drag action is permitted.
type PointerPressState struct {
	ID        string
	Modifiers ui.Modifiers
	Start     ui.Point
	Applied   bool
}

// DropHoverState captures hover timing used by host-controlled auto-expand.
type DropHoverState struct {
	TargetID     string
	EnteredAt    time.Time
	AutoExpandID string
	AutoExpandAt time.Time
}

// DropTargetRange is a presentation hint over a half-open row interval.
type DropTargetRange struct {
	TargetID string
	StartRow int
	EndRow   int
}

// DragOperationDisplay contains display values only; it never chooses or
// performs a file operation.
type DragOperationDisplay struct {
	Kind        DragOperationKind
	TargetLabel string
}

type DragOperationKind uint8

const (
	DragOperationNone DragOperationKind = iota
	DragOperationMoveToFolder
	DragOperationMoveToTrash
	DragOperationInsertIntoEditor
)

func (d DragOperationDisplay) Label() string {
	switch d.Kind {
	case DragOperationMoveToFolder:
		return locales.FilesMoveItemToFolder(d.TargetLabel)
	case DragOperationMoveToTrash:
		return locales.FilesMoveToTrash()
	case DragOperationInsertIntoEditor:
		return locales.FilesInsertIntoEditor()
	default:
		return ""
	}
}

func newState() *state {
	selected := -1
	return &state{expanded: make(map[string]bool), selected: make(map[string]struct{}), list: ui.ListState{Selected: &selected}}
}

func (s *state) IsExpanded(id string) bool { return s != nil && s.expanded[id] }

func (s *state) ExpandedIDs() []string {
	if s == nil {
		return nil
	}
	ids := make([]string, 0, len(s.expanded))
	for id, expanded := range s.expanded {
		if expanded {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (s *state) CollapseAll() {
	if s != nil {
		if len(s.expanded) > 0 {
			s.InvalidateRows()
		}
		clear(s.expanded)
	}
}

func (s *state) SetExpanded(id string, expanded bool) {
	if s == nil || id == "" {
		return
	}
	if s.expanded == nil {
		s.expanded = make(map[string]bool)
	}
	if s.expanded[id] != expanded {
		s.InvalidateRows()
	}
	s.expanded[id] = expanded
}

func (s *state) ToggleExpanded(id string) bool {
	if s == nil || id == "" {
		return false
	}
	s.SetExpanded(id, !s.IsExpanded(id))
	return s.IsExpanded(id)
}

func (s *state) SelectedIDs() []string {
	if s == nil {
		return nil
	}
	ids := make([]string, 0, len(s.selected))
	for id := range s.selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// SelectedPaths returns the current selection in visible row order, with a
// focused or clicked item as fallback when the selection is empty. Keeping
// this projection with the tree state avoids making the app shell understand
// the panel's selection semantics.
func (h *Handle) SelectedPaths(rows []Row, fallback string) []string {
	if h == nil || h.state == nil {
		return nil
	}
	selected := make([]string, 0, len(h.selected))
	for _, row := range rows {
		if h.IsSelected(row.Path) {
			selected = append(selected, row.Path)
		}
	}
	if len(selected) == 0 && fallback != "" {
		selected = append(selected, fallback)
	}
	return selected
}

// CreateTarget resolves the file-tree selection for a create action. A
// selected directory is the destination; a selected file leaves creation at
// the vault root, matching the tree's New File/New Folder commands.
func (h *Handle) CreateTarget(root, selected string, selectedDirectory bool) string {
	if h != nil && selected != "" && selectedDirectory {
		return selected
	}
	return root
}

func (s *state) IsSelected(id string) bool { return s.owns(id) }

func (s *state) SetSelection(ids []string) {
	if s == nil {
		return
	}
	s.selected = make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			s.selected[id] = struct{}{}
		}
	}
	if len(ids) > 0 {
		s.selectionAnchor = ids[0]
	} else {
		s.selectionAnchor = ""
	}
	s.syncListFocus()
}

// RemoveSelected removes one vanished vault item from the panel's transient
// selection while preserving the remaining selected IDs.
func (h *Handle) RemoveSelected(id string) {
	if h == nil || h.state == nil || id == "" || !h.IsSelected(id) {
		return
	}
	selected := h.SelectedIDs()
	remaining := selected[:0]
	for _, selectedID := range selected {
		if selectedID != id {
			remaining = append(remaining, selectedID)
		}
	}
	h.SetSelection(remaining)
}

// SelectID updates the panel's selection presentation from a visible row
// interaction. Shift selection extends from the last plain selection anchor;
// Cmd toggles the row while preserving the anchor.
func (s *state) SelectID(id string, modifiers ui.Modifiers, visibleEntries []Entry) {
	if s == nil || id == "" {
		return
	}
	s.selectID(id, modifiers, visibleEntries)
	s.focusID = id
	s.visibleIDs = entryIDs(visibleEntries)
	s.syncListFocus()
}

// PrepareDragSelection applies the pointer-down selection rules before a drag
// crosses the toolkit threshold. An unmodified press on an already selected
// row preserves the multi-selection; modifiers otherwise follow SelectID.
func (s *state) PrepareDragSelection(id string, modifiers ui.Modifiers, visibleEntries []Entry) {
	if s == nil || id == "" {
		return
	}
	if modifiers&ui.Shift != 0 || modifiers&ui.Cmd != 0 || !s.owns(id) {
		s.selectID(id, modifiers, visibleEntries)
	} else {
		s.visibleIDs = entryIDs(visibleEntries)
		s.syncListFocus()
	}
	s.focusID = id
	s.syncListFocus()
}

func (s *state) FocusID() string {
	if s == nil {
		return ""
	}
	return s.focusID
}
func (s *state) SetFocusID(id string) {
	if s != nil {
		s.focusID = id
		s.syncListFocus()
	}
}
func (s *state) SetFocused(focused bool) {
	if s != nil {
		s.focused = focused
	}
}
func (s *state) Focused() bool { return s != nil && s.focused }

// EnsureFocus keeps focus on a visible row, preferring the current focus and
// then the supplied workspace/editor fallbacks.
func (s *state) EnsureFocus(rows []Row, preferred ...string) string {
	if s == nil {
		return ""
	}
	if len(rows) == 0 {
		s.SetFocusID("")
		return ""
	}
	candidates := append([]string{s.FocusID()}, preferred...)
	for _, candidate := range candidates {
		for _, row := range rows {
			if candidate != "" && row.Path == candidate {
				s.SetFocusID(candidate)
				return candidate
			}
		}
	}
	s.SetFocusID(rows[0].Path)
	return rows[0].Path
}

// Navigate moves focus by a visible row delta or to an explicit row index.
// Shift movement extends the range from the selection anchor.
func (s *state) Navigate(rows []Row, explicitIndex *int, delta int, extend bool) string {
	if s == nil || len(rows) == 0 {
		if s != nil {
			s.SetFocusID("")
		}
		return ""
	}
	index := 0
	for i, row := range rows {
		if row.Path == s.FocusID() {
			index = i
			break
		}
	}
	mods := ui.Modifiers(0)
	if explicitIndex != nil {
		index = max(0, min(*explicitIndex, len(rows)-1))
	} else {
		index = max(0, min(index+delta, len(rows)-1))
		if extend {
			mods = ui.Shift
		}
	}
	id := rows[index].Path
	entries := entriesForRows(rows)
	s.SelectID(id, mods, entries)
	return id
}

func (s *state) ToggleSelectionID(id string, rows []Row) {
	if s != nil && id != "" {
		s.SelectID(id, ui.Cmd, entriesForRows(rows))
	}
}

func (s *state) SelectAll(rows []Row) {
	if s == nil {
		return
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.Path
	}
	s.SetSelection(ids)
	if len(ids) > 0 {
		s.SetFocusID(ids[0])
	}
}

func (s *state) InvertSelection(rows []Row) {
	if s == nil {
		return
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if !s.IsSelected(row.Path) {
			ids = append(ids, row.Path)
		}
	}
	s.SetSelection(ids)
}

// CloseFocusedNode collapses the focused expanded directory, or the nearest
// expanded ancestor and moves focus to that ancestor.
func (s *state) CloseFocusedNode(rows []Row) bool {
	if s == nil {
		return false
	}
	path := s.FocusID()
	if path == "" {
		return false
	}
	if s.IsExpanded(path) {
		s.SetExpanded(path, false)
		return true
	}
	for parent := filepath.Dir(path); parent != path; parent = filepath.Dir(parent) {
		if !s.IsExpanded(parent) {
			continue
		}
		s.SetExpanded(parent, false)
		s.SelectID(parent, 0, entriesForRows(rows))
		return true
	}
	return false
}

// ToggleHorizontal applies tree-specific left/right behavior using the
// flattened projection supplied by the catalog adapter.
func (s *state) ToggleHorizontal(rows []Row, direction int) (expanded bool) {
	if s == nil {
		return false
	}
	path := s.FocusID()
	if path == "" {
		return false
	}
	if direction > 0 {
		for i, row := range rows {
			if row.Path != path || !row.Directory {
				continue
			}
			if !s.IsExpanded(path) {
				s.SetExpanded(path, true)
				return true
			}
			if i+1 < len(rows) && rows[i+1].Depth > row.Depth {
				s.SelectID(rows[i+1].Path, 0, entriesForRows(rows))
			}
			return false
		}
		return false
	}
	for _, row := range rows {
		if row.Path == path && row.Directory && s.IsExpanded(path) {
			s.SetExpanded(path, false)
			return true
		}
	}
	parent := filepath.Dir(path)
	for _, row := range rows {
		if row.Path == parent {
			s.SelectID(parent, 0, entriesForRows(rows))
			break
		}
	}
	return false
}

// ApplyKey executes one tree-local keyboard command. Rows are the host's
// current visible projection; this package uses them only to navigate and
// update presentation state, never to discover files or perform operations.
func (s *state) ApplyKey(command KeyCommand, rows []Row) bool {
	if s == nil {
		return false
	}
	switch command.Operation {
	case KeyNavigate, KeySelectRange, KeyCloseNode, KeyToggleSelection:
		s.EnsureFocus(rows, command.PreferredIDs...)
	case KeyToggleExpanded:
		if command.TargetID == "" {
			s.EnsureFocus(rows, command.PreferredIDs...)
		}
	}
	switch command.Operation {
	case KeyFocus:
		s.SetFocusID(command.TargetID)
		return true
	case KeyEnsureFocus:
		preferred := append([]string(nil), command.PreferredIDs...)
		before := s.FocusID()
		after := s.EnsureFocus(rows, preferred...)
		return before != after
	case KeyNavigate, KeySelectRange:
		var explicit *int
		if command.HasIndex {
			target := command.Index
			if target < 0 {
				target = len(rows) - 1
			}
			explicit = &target
		}
		beforeFocus := s.FocusID()
		beforeSelection := s.SelectedIDs()
		s.Navigate(rows, explicit, command.Delta, command.Operation == KeySelectRange)
		return beforeFocus != s.FocusID() || !sameIDs(beforeSelection, s.SelectedIDs())
	case KeyToggleExpanded:
		if command.TargetID != "" {
			before := s.IsExpanded(command.TargetID)
			s.ToggleExpanded(command.TargetID)
			return before != s.IsExpanded(command.TargetID)
		}
		beforeFocus := s.FocusID()
		wasExpanded := s.IsExpanded(beforeFocus)
		changed := s.ToggleHorizontal(rows, command.Direction)
		return changed || beforeFocus != s.FocusID() || wasExpanded != s.IsExpanded(beforeFocus)
	case KeyCloseNode:
		return s.CloseFocusedNode(rows)
	case KeyCollapseAll:
		before := len(s.expanded)
		s.CollapseAll()
		return before > 0
	case KeySelectAll:
		before := s.SelectedIDs()
		s.SelectAll(rows)
		return !sameIDs(before, s.SelectedIDs())
	case KeyToggleSelection:
		id := command.TargetID
		if id == "" {
			id = s.FocusID()
		}
		before := s.SelectedIDs()
		s.ToggleSelectionID(id, rows)
		return !sameIDs(before, s.SelectedIDs())
	case KeyInvertSelection:
		before := s.SelectedIDs()
		s.InvertSelection(rows)
		return !sameIDs(before, s.SelectedIDs())
	default:
		return false
	}
}

func sameIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *state) PreviewID() string {
	if s == nil {
		return ""
	}
	return s.previewID
}
func (s *state) PreviewBounds() ui.Rect {
	if s == nil {
		return ui.Rect{}
	}
	return s.previewBounds
}

// SetPreview records the row used to anchor a host-managed delayed preview.
// Passing an empty ID clears the preview.
func (s *state) SetPreview(id string, bounds ui.Rect) {
	if s == nil {
		return
	}
	if id == "" {
		s.ClearPreview()
		return
	}
	if s.previewID != id {
		s.previewPopupHovered = false
	}
	s.previewID, s.previewBounds = id, bounds
	s.CancelPreviewLeave()
}

func (s *state) ClearPreview() {
	if s != nil {
		s.previewID, s.previewBounds = "", ui.Rect{}
		s.previewLeaveAt = time.Time{}
		s.previewLeaveNotified = false
		s.previewPopupHovered = false
	}
}

func (s *state) BeginPreviewLeave(now time.Time, delay time.Duration) {
	if s == nil || s.previewID == "" || s.previewPopupHovered || delay <= 0 {
		return
	}
	s.previewLeaveAt = now.Add(delay)
}

func (s *state) CancelPreviewLeave() {
	if s != nil {
		s.previewLeaveAt = time.Time{}
		s.previewLeaveNotified = false
	}
}

func (s *state) PreviewLeaveAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	return s.previewLeaveAt
}

func (s *state) SetPreviewPopupHovered(hovered bool) {
	if s == nil {
		return
	}
	s.previewPopupHovered = hovered
	if hovered {
		s.CancelPreviewLeave()
	}
}

func (s *state) PreviewPopupHovered() bool {
	return s != nil && s.previewPopupHovered
}

func (s *state) PreviewLeaveExpired(now time.Time) bool {
	return s != nil && !s.previewLeaveAt.IsZero() && !s.previewPopupHovered && !now.Before(s.previewLeaveAt)
}

func (s *state) SetDragPreview(preview DragPreviewState) {
	if s != nil {
		s.dragPreview = preview
	}
}

func (s *state) DragPreview() DragPreviewState {
	if s == nil {
		return DragPreviewState{}
	}
	return s.dragPreview
}

func (s *state) SetPointerPress(id string, modifiers ui.Modifiers, start ui.Point) {
	if s == nil {
		return
	}
	if id == "" {
		s.ClearPointerPress()
		return
	}
	s.pointerPress = PointerPressState{ID: id, Modifiers: modifiers, Start: start}
}

func (s *state) PointerPress() PointerPressState {
	if s == nil {
		return PointerPressState{}
	}
	return s.pointerPress
}

func (s *state) SetPointerPressApplied(applied bool) {
	if s != nil {
		s.pointerPress.Applied = applied
	}
}

func (s *state) ClearPointerPress() {
	if s != nil {
		s.pointerPress = PointerPressState{}
	}
}

func (s *state) SetPointerReleased(released bool) {
	if s != nil {
		s.pointerReleased = released
	}
}

func (s *state) PointerReleased() bool { return s != nil && s.pointerReleased }

func (s *state) SetDropHover(state DropHoverState) {
	if s == nil {
		return
	}
	if state.TargetID == "" {
		s.ClearDropHover()
		return
	}
	if state.AutoExpandID == "" {
		state.AutoExpandID = state.TargetID
	}
	s.dropHover = state
}

func (s *state) DropHover() DropHoverState {
	if s == nil {
		return DropHoverState{}
	}
	return s.dropHover
}

func (s *state) ClearDropHover() {
	if s != nil {
		s.dropHover = DropHoverState{}
	}
}

func (s *state) DropAutoExpandExpired(now time.Time) bool {
	return s != nil && s.dropHover.TargetID != "" && s.dropHover.AutoExpandID != "" &&
		!s.dropHover.AutoExpandAt.IsZero() && !now.Before(s.dropHover.AutoExpandAt)
}

func (s *state) SetDropTargetRange(targetID string, startRow, endRow int) {
	if s == nil {
		return
	}
	if targetID == "" || startRow < 0 || endRow <= startRow {
		s.dropTargetRange = DropTargetRange{}
		return
	}
	s.dropTargetRange = DropTargetRange{TargetID: targetID, StartRow: startRow, EndRow: endRow}
}

func (s *state) DropTargetRange() DropTargetRange {
	if s == nil {
		return DropTargetRange{}
	}
	return s.dropTargetRange
}

func (s *state) ClearDropTargetRange() {
	if s != nil {
		s.dropTargetRange = DropTargetRange{}
	}
}

func (s *state) SetTrashTargetActive(active bool) {
	if s != nil {
		s.trashTargetActive = active
	}
}

func (s *state) TrashTargetActive() bool { return s != nil && s.trashTargetActive }

func (s *state) SetDragOperationDisplay(display DragOperationDisplay) {
	if s != nil {
		s.dragOperation = display
	}
}

func (s *state) DragOperationDisplay() DragOperationDisplay {
	if s == nil {
		return DragOperationDisplay{}
	}
	return s.dragOperation
}

func (s *state) SetClipboard(kind ClipboardKind, ids []string) {
	if s == nil {
		return
	}
	s.clipboard = Clipboard{Kind: kind, IDs: cloneIDs(ids)}
}

// CaptureClipboard stores the currently selected paths as copied or cut and
// returns a detached snapshot for the caller's status message.
func (s *state) CaptureClipboard(ids []string, cut bool) Clipboard {
	kind := ClipboardCopy
	if cut {
		kind = ClipboardCut
	}
	s.SetClipboard(kind, ids)
	return s.Clipboard()
}

func (s *state) ClearClipboard() {
	if s != nil {
		s.clipboard = Clipboard{}
	}
}
func (s *state) Clipboard() Clipboard {
	if s == nil {
		return Clipboard{}
	}
	return Clipboard{Kind: s.clipboard.Kind, IDs: cloneIDs(s.clipboard.IDs)}
}

func (s *state) IsCut(id string) bool {
	if s == nil || s.clipboard.Kind != ClipboardCut {
		return false
	}
	for _, candidate := range s.clipboard.IDs {
		if candidate == id {
			return true
		}
	}
	return false
}

// RetainCutPaths removes paths successfully moved by a paste operation from
// the cut clipboard, keeping any paths that could not be moved.
func (s *state) RetainCutPaths(moved []string) {
	if s == nil || s.clipboard.Kind != ClipboardCut {
		return
	}
	movedSet := make(map[string]struct{}, len(moved))
	for _, path := range moved {
		movedSet[path] = struct{}{}
	}
	remaining := make([]string, 0, len(s.clipboard.IDs))
	for _, path := range s.clipboard.IDs {
		if _, ok := movedSet[path]; !ok {
			remaining = append(remaining, path)
		}
	}
	s.SetClipboard(ClipboardCut, remaining)
}

func (s *state) ScrollOffset() ui.Point {
	if s == nil {
		return ui.Point{}
	}
	return ui.Point{X: s.scroll.X, Y: s.scroll.Y}
}
func (s *state) SetScrollOffset(offset ui.Point) {
	if s != nil {
		s.scroll.X, s.scroll.Y = offset.X, offset.Y
	}
}

// RemapIDs updates identifiers after a host-side rename or move. It preserves
// expanded and clipboard state, removes empty destinations, and merges key
// collisions without retaining duplicate selected or clipboard IDs.
func (s *state) RemapIDs(mapper func(string) string) {
	if s == nil || mapper == nil {
		return
	}
	s.InvalidateRows()
	remapped := make(map[string]string)
	remap := func(id string) string {
		if id == "" {
			return ""
		}
		if next, ok := remapped[id]; ok {
			return next
		}
		next := mapper(id)
		remapped[id] = next
		return next
	}
	expanded := make(map[string]bool, len(s.expanded))
	for id, isExpanded := range s.expanded {
		if next := remap(id); next != "" && isExpanded {
			expanded[next] = true
		}
	}
	s.expanded = expanded

	selected := make(map[string]struct{}, len(s.selected))
	for id := range s.selected {
		if next := remap(id); next != "" {
			selected[next] = struct{}{}
		}
	}
	s.selected = selected
	s.selectionAnchor = remap(s.selectionAnchor)
	s.focusID = remap(s.focusID)
	s.previewID = remap(s.previewID)
	if s.previewID == "" {
		s.ClearPreview()
	}
	s.dragPreview.ItemID = remap(s.dragPreview.ItemID)
	s.pointerPress.ID = remap(s.pointerPress.ID)
	if s.pointerPress.ID == "" {
		s.ClearPointerPress()
	}
	s.dropHover.TargetID = remap(s.dropHover.TargetID)
	s.dropHover.AutoExpandID = remap(s.dropHover.AutoExpandID)
	s.dropTargetRange.TargetID = remap(s.dropTargetRange.TargetID)
	if s.dropHover.TargetID == "" {
		s.ClearDropHover()
	}
	if s.dropHover.AutoExpandID == "" {
		s.dropHover.AutoExpandAt = time.Time{}
	}
	if s.dropTargetRange.TargetID == "" {
		s.ClearDropTargetRange()
	}

	clipboardIDs := make([]string, 0, len(s.clipboard.IDs))
	seen := make(map[string]struct{}, len(s.clipboard.IDs))
	for _, id := range s.clipboard.IDs {
		next := remap(id)
		if next == "" {
			continue
		}
		if _, duplicate := seen[next]; duplicate {
			continue
		}
		seen[next] = struct{}{}
		clipboardIDs = append(clipboardIDs, next)
	}
	s.clipboard.IDs = clipboardIDs
	if len(clipboardIDs) == 0 {
		s.clipboard = Clipboard{}
	}

	visible := make([]string, 0, len(s.visibleIDs))
	seen = make(map[string]struct{}, len(s.visibleIDs))
	for _, id := range s.visibleIDs {
		next := remap(id)
		if next == "" {
			continue
		}
		if _, duplicate := seen[next]; duplicate {
			continue
		}
		seen[next] = struct{}{}
		visible = append(visible, next)
	}
	s.visibleIDs = visible
	s.syncListFocus()
}

func (s *state) owns(id string) bool {
	if s == nil {
		return false
	}
	_, ok := s.selected[id]
	return ok
}

func (s *state) initialize() {
	if s.expanded == nil {
		s.expanded = make(map[string]bool)
	}
	if s.selected == nil {
		s.selected = make(map[string]struct{})
	}
	if s.list.Selected == nil {
		selected := -1
		s.list.Selected = &selected
	}
}

func (s *state) syncListFocus() {
	if s == nil || s.list.Selected == nil {
		return
	}
	*s.list.Selected = -1
	for i, id := range s.visibleIDs {
		if id == s.focusID && id != "" {
			*s.list.Selected = i
			return
		}
	}
	for i, id := range s.visibleIDs {
		if _, selected := s.selected[id]; selected {
			*s.list.Selected = i
			return
		}
	}
}

func entryIDs(entries []Entry) []string {
	ids := make([]string, len(entries))
	for i, entry := range entries {
		ids[i] = entry.ID
	}
	return ids
}

func entriesForRows(rows []Row) []Entry {
	entries := make([]Entry, len(rows))
	for i, row := range rows {
		entries[i] = Entry{ID: row.Path}
	}
	return entries
}

func applyPresentationAction(s *state, kind ActionKind, id string, modifiers ui.Modifiers, bounds ui.Rect, entries []Entry) {
	if s == nil {
		return
	}
	switch kind {
	case ToggleEntry:
		s.ToggleExpanded(id)
	case SelectEntry:
		s.SelectID(id, modifiers, entries)
	case OpenEntry:
		s.focusID = id
		s.syncListFocus()
	case HoverEntry:
		s.SetPreview(id, bounds)
	case LeaveEntry:
		// The host applies its delayed leave policy through BeginPreviewLeave.
		s.previewLeaveNotified = true
	}
}

func (s *state) selectID(id string, modifiers ui.Modifiers, entries []Entry) {
	if s.selected == nil {
		s.selected = make(map[string]struct{})
	}
	switch {
	case modifiers&ui.Shift != 0:
		anchor := -1
		for i, entry := range entries {
			if entry.ID == s.selectionAnchor {
				anchor = i
				break
			}
		}
		current := -1
		for i, entry := range entries {
			if entry.ID == id {
				current = i
				break
			}
		}
		if anchor < 0 {
			anchor = current
		}
		if current >= 0 {
			s.selected = make(map[string]struct{})
			for i := min(anchor, current); i <= max(anchor, current); i++ {
				s.selected[entries[i].ID] = struct{}{}
			}
		}
	case modifiers&ui.Cmd != 0:
		if _, ok := s.selected[id]; ok {
			delete(s.selected, id)
		} else {
			s.selected[id] = struct{}{}
		}
	default:
		s.selected = map[string]struct{}{id: {}}
		s.selectionAnchor = id
	}
	s.visibleIDs = entryIDs(entries)
	s.syncListFocus()
}
