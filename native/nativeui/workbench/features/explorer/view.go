// Package filetree renders a caller-projected file tree and translates user
// input into typed actions. It owns transient presentation state; discovery,
// filtering, filesystem operations, and drag/drop policy belong to the host.
package filetree

import (
	"math"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"muse/locales"
)

type EntryKind uint8

const (
	File EntryKind = iota + 1
	Directory
)

// Entry is one already ordered and already visible tree row. ID is opaque to
// this package and should be stable while the row remains the same item.
type Entry struct {
	ID             string
	Label          string
	Detail         string
	Kind           EntryKind
	Depth          int
	CanSelect      bool
	CanExpand      bool
	CanOpen        bool
	CanDrag        bool
	CanDrop        bool
	DragIDs        []string
	DragActive     bool
	ContextActions []Command
	Previewable    bool
	DropHint       bool
	Icon           *ui.SVG
	IconTint       ui.Color
}

// Command describes an explorer menu or toolbar action. The panel renders it
// and returns its typed ID to the host.
type Command struct {
	ID      CommandID
	Label   string
	Icon    *ui.SVG
	Enabled bool
}

// CommandID is an explorer-owned intent returned by a context menu. The host
// maps it to an application use case; it is not a filesystem operation.
type CommandID string

// NativeDropPort is a caller-provided native transfer capability. The panel
// negotiates/detects transfers through MyGo and returns completed drops; it
// never chooses a filesystem operation.
type NativeDropPort struct {
	Formats    []transfer.Format
	Operations transfer.Operation
}

// ViewModel contains an ordered visible-row projection and capabilities. The
// host owns catalog discovery, filtering, operation policy, and file access.
type ViewModel struct {
	Entries            []Entry
	PopoverAnchorID    string
	Commands           []Command
	EmptyMessage       string
	EmptyActionLabel   string
	EmptyActions       []Command
	RootLabel          string
	RootContextActions []Command
	RootCanDrop        bool
	NativeDrop         NativeDropPort
}

// ModelEntryInput carries host-discovered row facts and visual resources into
// the explorer projector. The feature derives selection, drag IDs, affordances,
// context menus, and drop-range highlighting from its own state and policy.
type ModelEntryInput struct {
	Row         Row
	Detail      string
	Icon        *ui.SVG
	IconTint    ui.Color
	Previewable bool
}

// ModelInput contains catalog rows and external capabilities needed to build
// the explorer view. It intentionally has no application object or callbacks.
type ModelInput struct {
	Entries          []ModelEntryInput
	PopoverAnchorID  string
	Commands         []Command
	EmptyMessage     string
	EmptyActionLabel string
	EmptyActions     []Command
	RootLabel        string
	DragActive       bool
	RootCanDrop      bool
	NativeDrop       NativeDropPort
}

type ActionKind uint8

const (
	SelectEntry ActionKind = iota + 1
	NavigateEntry
	SubmitEntry
	ToggleEntry
	OpenEntry
	InvokeCommand
	DropEntries
	InvokeContextCommand
	HoverEntry
	LeaveEntry
	StartDrag
	BeginNativeHandoff
	EndDrag
	DropNative
	ImportFiles
)

// Action contains presentation intent only. In particular, DropEntries does
// not imply move/copy/reorder; the caller decides what the target means.
type Action[T any] struct {
	Kind      ActionKind
	ID        string
	Index     int
	CommandID CommandID
	ItemIDs   []string
	Payload   T
	Modifiers ui.Modifiers
	Offer     transfer.Offer
	Drop      transfer.Drop
	Paths     []string
	Bounds    ui.Rect
}

// DragPort adapts the panel's stable row IDs to a caller-owned MyGo drag
// payload. This lets another panel consume the same concrete payload type
// without filetree importing that feature's package.
type DragPort[T any] struct {
	Make func(entryID string, selectedIDs []string) T
	IDs  func(T) []string
}

type RenderResult struct {
	List          ui.Element
	PopoverAnchor ui.Element
	HoveredID     string
	NativeOffer   *transfer.Offer
}

// Render draws the caller's projected file tree using panel-owned transient
// presentation state, then returns typed intents for host-side handling.
func Render[T any](c *ui.Context, handle *Handle, model ViewModel, drag DragPort[T], dispatch func(Action[T])) RenderResult {
	var state *state
	if handle != nil {
		state = handle.state
	}
	if state == nil {
		state = newState()
	}
	state.initialize()
	entries := append([]Entry(nil), model.Entries...)
	dispatchState := func(action Action[T]) {
		applyPresentationAction(state, action.Kind, action.ID, action.Modifiers, action.Bounds, entries)
		if action.Kind == StartDrag {
			drag := state.DragPreview()
			drag.Active = true
			drag.Frames++
			drag.ItemID = action.ID
			if drag.StartedAt.IsZero() {
				drag.StartedAt = c.Now()
			}
			state.SetDragPreview(drag)
		}
		dispatchAction(dispatch, action)
	}
	t := c.Theme()
	if len(model.Commands) > 0 {
		ui.Row(c).FillWidth().Height(34).Padding(0, 6).Gap(4).AlignItems(ui.Center).Children(func() {
			for _, command := range model.Commands {
				command := command
				button := ui.ButtonBase(c).Label(command.Label).Disabled(!command.Enabled).Height(28).Padding(0, 8)
				if command.Icon != nil {
					button.Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, command.Icon).Size(14, 14).TextColor(t.TextMuted)
							ui.Text(c, command.Label).FontSize(11).TextColor(t.Text)
						})
					})
				}
				if button.Clicked() && command.Enabled {
					dispatchState(Action[T]{Kind: InvokeCommand, CommandID: command.ID})
				}
			}
		})
	}

	if len(model.Entries) == 0 {
		message := model.EmptyMessage
		if message == "" && model.EmptyActionLabel == "" {
			message = locales.FilesNoEditableText()
		}
		area := ui.Column(c).FillWidth().Grow(1).Center().Padding(20).Children(func() {
			if model.EmptyActionLabel != "" && len(model.EmptyActions) > 0 {
				open := ui.Button(c, model.EmptyActionLabel)
				open.Menu(func(m *ui.Menu) {
					for _, command := range model.EmptyActions {
						command := command
						if m.Item(command.Label).Disabled(!command.Enabled).Chosen() && command.Enabled {
							dispatchState(Action[T]{Kind: InvokeCommand, CommandID: command.ID})
						}
					}
				})
			}
			if message != "" {
				ui.Text(c, message).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
		})
		attachContextMenu(area, "", model.RootContextActions, dispatchState)
		if model.RootCanDrop {
			if payload, ok := ui.Drop[T](area); ok {
				dispatchState(Action[T]{Kind: DropEntries, ItemIDs: dragIDs(drag, payload), Payload: payload})
			}
			if model.NativeDrop.Operations != transfer.None && len(model.NativeDrop.Formats) > 0 {
				opts := transfer.DropOptions{Formats: model.NativeDrop.Formats, Operations: model.NativeDrop.Operations}
				if drop, ok := ui.DropData(area, opts); ok {
					dispatchState(Action[T]{Kind: DropNative, Drop: drop})
				}
			}
			if paths := area.DroppedFiles(); len(paths) > 0 {
				dispatchState(Action[T]{Kind: ImportFiles, Paths: append([]string(nil), paths...)})
			}
		}
		if state.previewID != "" && !state.previewLeaveNotified {
			dispatchState(Action[T]{Kind: LeaveEntry, ID: state.previewID})
		}
		return RenderResult{List: area}
	}

	listState := &state.list
	if listState.Selected == nil {
		listState.Selected = new(int)
	}
	state.visibleIDs = entryIDs(entries)
	state.syncListFocus()
	listState.Label = func(i int) string { return entries[i].Label }
	listState.Key = func(i int) any { return entries[i].ID }

	showScrollbar := state.indicator.Visible(c.Now(), state.scroll.X, state.scroll.Y)
	if showScrollbar {
		c.After(state.indicator.Remaining(c.Now()))
	}

	pointerActivated := false
	rowDropHandled := false
	hoveredID := ""
	var hoveredBounds ui.Rect
	var nativeOffer *transfer.Offer
	var popoverAnchor ui.Element
	dropHoverID := ""
	list := ui.List(c, listState, len(entries), func(i int) {
		entry := entries[i]
		selected, focused := state.owns(entry.ID), state.focusID == entry.ID
		row := ui.ButtonBase(c).Label(entry.Label).Height(28).FillWidth().Radius(5).Padding(0, 8, 0, 4+float32(max(entry.Depth, 0))*14).
			Gap(7).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if entry.ID == model.PopoverAnchorID {
			popoverAnchor = row
		}
		if selected || focused || row.Hovered() || entry.DropHint {
			row.Background(t.SurfaceHover)
		}
		dropTarget := row
		row.Children(func() {
			if entry.Kind == Directory {
				label := ""
				if state.IsExpanded(entry.ID) {
					label = "⌄"
				} else {
					label = "›"
				}
				toggle := ui.ButtonBase(c).Label(label).Size(16, 20).Disabled(!entry.CanExpand).Padding(0).TextColor(t.TextMuted)
				if toggle.Clicked() && entry.CanExpand {
					pointerActivated = true
					dispatchState(Action[T]{Kind: ToggleEntry, ID: entry.ID, Modifiers: toggle.ClickModifiers()})
				}
			} else {
				ui.Box(c).Size(16, 20)
			}
			if entry.Icon != nil {
				ui.Icon(c, entry.Icon).Size(15, 15).TextColor(entry.IconTint)
			} else {
				glyph := "▤"
				if entry.Kind == Directory {
					glyph = "▱"
				}
				ui.Text(c, glyph).Width(15).FontSize(13).TextColor(t.TextMuted)
			}
			ui.Column(c).FillWidth().MinWidth(0).Gap(1).Children(func() {
				labelColor := t.Text
				if state.IsCut(entry.ID) {
					labelColor = t.TextMuted
				}
				label := ui.Text(c, entry.Label).SingleLine().Ellipsis("…").FontSize(12).TextColor(labelColor)
				if selected || focused {
					label.FontWeight(600)
				}
				if entry.Detail != "" {
					ui.Text(c, entry.Detail).SingleLine().Ellipsis("…").FontSize(9.5).TextColor(t.TextMuted)
				}
			})
			if state.IsCut(entry.ID) {
				ui.Text(c, locales.FilesCut()).Label(locales.FilesCutHint()).FontSize(9).TextColor(t.TextMuted.Alpha(0.58)).
					Padding(1, 4).Radius(4).Background(t.Surface)
			}
		})
		if entry.CanDrag && drag.Make != nil {
			dragItems := cloneIDs(entry.DragIDs)
			if len(dragItems) == 0 {
				dragItems = []string{entry.ID}
			}
			payload := drag.Make(entry.ID, dragItems)
			handle := ui.Box(c).Key("tree-drag-handle-" + entry.ID).Absolute().Left(0).Top(0).Right(0).Height(28).
				Cursor(ui.CursorPointer).Opacity(0).Label(locales.TreeDragHandleLabel(entry.Label)).Drag(payload)
			itemID := entry.ID
			if model.RootLabel != "" {
				if relative, err := filepath.Rel(model.RootLabel, entry.ID); err == nil {
					itemID = filepath.ToSlash(relative)
				}
			}
			theme := c.Theme()
			cardWidth, cardHeight := float32(260), float32(28)
			cardLeft := float32(0)
			if handle.Dragging() {
				drag := state.DragPreview()
				if !drag.StartedAt.IsZero() && c.Now().Sub(drag.StartedAt) >= 120*time.Millisecond {
					cardWidth, cardHeight = 220, 50
					cardLeft = 20
				} else {
					c.After(60 * time.Millisecond)
				}
			}
			handle.Children(func() {
				card := ui.Row(c).Absolute().Left(cardLeft).Top(0).Key("tree-drag-projection-card-" + itemID).
					Label(locales.TreeDragProjectionLabel(itemID)).Width(cardWidth).MinWidth(0).Height(cardHeight).
					IgnoreInput(true).Radius(8).Background(theme.SurfaceHover).
					Border(1, theme.Border).Padding(0, 8).Gap(8).AlignItems(ui.Center)
				card.Children(func() {
					if entry.Icon != nil {
						ui.Icon(c, entry.Icon).Size(14, 14).TextColor(entry.IconTint)
					}
					ui.Text(c, entry.Label).FontSize(12.5).FontWeight(600).
						TextColor(theme.Text).SingleLine().Ellipsis("…")
				})
			})
			dropTarget = handle
			handle.HandleInput(func(ev ui.InputEvent) bool {
				switch ev.Kind {
				case ui.InputPointerDown:
					state.SetPointerReleased(false)
					state.SetPointerPress(entry.ID, ev.Mods, ui.Point{X: ev.X, Y: ev.Y})
					if ev.Button == 1 {
						state.PrepareDragSelection(entry.ID, 0, entries)
					}
				case ui.InputPointerMove:
					press := state.PointerPress()
					if press.ID == entry.ID && !press.Applied && pointerMovePassedThreshold(press.Start, ui.Point{X: ev.X, Y: ev.Y}) {
						state.PrepareDragSelection(entry.ID, press.Modifiers, entries)
						state.SetPointerPressApplied(true)
					}
				case ui.InputPointerUp:
					state.SetPointerReleased(true)
				}
				return false
			})
			handle.OnClick(func() {
				pointerActivated = true
				mods := handle.ClickModifiers()
				if entry.CanSelect && listState.Selected != nil {
					*listState.Selected = i
				}
				if entry.CanSelect {
					dispatchState(Action[T]{Kind: SelectEntry, ID: entry.ID, Modifiers: mods})
				}
				if mods&(ui.Cmd|ui.Shift) == 0 {
					emitDefaultAction(entry, dispatchState)
				}
			})
			attachContextMenu(handle, entry.ID, entry.ContextActions, dispatchState)
			if handle.Dragging() {
				if !entry.DragActive {
					dispatchState(Action[T]{Kind: StartDrag, ID: entry.ID, ItemIDs: dragItems, Payload: payload})
				} else {
					dragState := state.DragPreview()
					dragState.Active = true
					dragState.Frames++
					dragState.ItemID = entry.ID
					state.SetDragPreview(dragState)
				}
			}
		}
		if entry.Previewable && row.Hovered() {
			hoveredID, hoveredBounds = entry.ID, row.Bounds()
		}
		if entry.CanDrop {
			if payload, ok := ui.Drop[T](dropTarget); ok {
				rowDropHandled = true
				dispatchState(Action[T]{Kind: DropEntries, ID: entry.ID, ItemIDs: dragIDs(drag, payload), Payload: payload})
			}
			if _, over := ui.DragOver[T](dropTarget); over {
				rowDropHandled = true
				dropHoverID = entry.ID
				updateDropHover(c, state, entry, i)
				row.Background(t.SurfaceHover)
			}
			if model.NativeDrop.Operations != transfer.None && len(model.NativeDrop.Formats) > 0 {
				opts := transfer.DropOptions{Formats: model.NativeDrop.Formats, Operations: model.NativeDrop.Operations}
				if offer, over := ui.DataDragOver(dropTarget, opts); over {
					rowDropHandled = true
					dropHoverID = entry.ID
					updateDropHover(c, state, entry, i)
					nativeOffer = &offer
					row.Background(t.SurfaceHover)
				}
				if drop, ok := ui.DropData(dropTarget, opts); ok {
					rowDropHandled = true
					dispatchState(Action[T]{Kind: DropNative, ID: entry.ID, Drop: drop})
				}
			}
			if dropTarget.FileDragOver() {
				rowDropHandled = true
				row.Background(t.SurfaceHover)
			}
			if paths := dropTarget.DroppedFiles(); len(paths) > 0 {
				rowDropHandled = true
				dispatchState(Action[T]{Kind: ImportFiles, ID: entry.ID, Paths: append([]string(nil), paths...)})
			}
		}
		attachContextMenu(row, entry.ID, entry.ContextActions, dispatchState)
		if (entry.CanSelect || entry.CanExpand || entry.CanOpen) && (!entry.CanDrag || drag.Make == nil) {
			row.OnClick(func() {
				pointerActivated = true
				mods := row.ClickModifiers()
				if entry.CanSelect && listState.Selected != nil {
					*listState.Selected = i
				}
				if entry.CanSelect {
					dispatchState(Action[T]{Kind: SelectEntry, ID: entry.ID, Modifiers: mods})
				}
				if mods&(ui.Cmd|ui.Shift) != 0 {
					return
				}
				emitDefaultAction(entry, dispatch)
			})
		}
	})
	list.FillWidth().Grow(1).MinHeight(0).Padding(0, 6).Gap(1).TrackScroll(&state.scroll).Scrollbars(showScrollbar).Focusable()
	attachContextMenu(list, "", model.RootContextActions, dispatchState)
	if model.RootCanDrop && !rowDropHandled {
		if _, over := ui.DragOver[T](list); over {
			dropHoverID = model.RootLabel
			state.ClearDropHover()
			state.SetDropTargetRange(model.RootLabel, 0, len(entries))
			state.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: filepath.Base(model.RootLabel)})
		}
		if payload, ok := ui.Drop[T](list); ok {
			dispatchState(Action[T]{Kind: DropEntries, ItemIDs: dragIDs(drag, payload), Payload: payload})
		}
		if model.NativeDrop.Operations != transfer.None && len(model.NativeDrop.Formats) > 0 {
			opts := transfer.DropOptions{Formats: model.NativeDrop.Formats, Operations: model.NativeDrop.Operations}
			if offer, over := ui.DataDragOver(list, opts); over {
				dropHoverID = model.RootLabel
				state.ClearDropHover()
				state.SetDropTargetRange(model.RootLabel, 0, len(entries))
				state.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: filepath.Base(model.RootLabel)})
				if nativeOffer == nil {
					nativeOffer = &offer
				}
			}
			if drop, ok := ui.DropData(list, opts); ok {
				dispatchState(Action[T]{Kind: DropNative, Drop: drop})
			}
		}
		if paths := list.DroppedFiles(); len(paths) > 0 {
			dispatchState(Action[T]{Kind: ImportFiles, Paths: append([]string(nil), paths...)})
		}
	}
	if dropHoverID == "" {
		state.ClearDropHover()
		if !state.DragPreview().Active {
			state.ClearDropTargetRange()
		}
	}
	if hoveredID != state.previewID {
		if state.previewID != "" && !state.previewLeaveNotified {
			dispatchState(Action[T]{Kind: LeaveEntry, ID: state.previewID})
		}
		if hoveredID != "" {
			dispatchState(Action[T]{Kind: HoverEntry, ID: hoveredID, Bounds: hoveredBounds})
		}
	}
	if list.Changed() && !pointerActivated && state.focused && listState.Selected != nil {
		if i := *listState.Selected; i >= 0 && i < len(entries) {
			entry := entries[i]
			if entry.CanSelect {
				dispatchState(Action[T]{Kind: NavigateEntry, ID: entry.ID, Index: i, Modifiers: c.Modifiers()})
			}
		}
	}
	if !pointerActivated && state.focused && list.Submitted() && listState.Selected != nil {
		if i := *listState.Selected; i >= 0 && i < len(entries) {
			entry := entries[i]
			if entry.CanOpen || entry.CanExpand {
				dispatchState(Action[T]{Kind: SubmitEntry, ID: entry.ID, Index: i})
			}
		}
	}
	return RenderResult{List: list, PopoverAnchor: popoverAnchor, HoveredID: hoveredID, NativeOffer: nativeOffer}
}

func updateDropHover(c *ui.Context, state *state, entry Entry, rowIndex int) {
	if state == nil || entry.ID == "" {
		return
	}
	now := c.Now()
	hover := state.DropHover()
	if hover.TargetID != entry.ID {
		hover = DropHoverState{TargetID: entry.ID, EnteredAt: now}
		if entry.Kind == Directory && !state.IsExpanded(entry.ID) {
			hover.AutoExpandID = entry.ID
			hover.AutoExpandAt = now.Add(600 * time.Millisecond)
		}
		state.SetDropHover(hover)
	}
	state.SetDropTargetRange(entry.ID, rowIndex, rowIndex+1)
	targetLabel := filepath.Base(entry.ID)
	if entry.Kind != Directory {
		targetLabel = filepath.Base(filepath.Dir(entry.ID))
	}
	state.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: targetLabel})
	if hover.AutoExpandID != "" {
		if state.DropAutoExpandExpired(now) {
			state.SetExpanded(hover.AutoExpandID, true)
			hover.AutoExpandID = ""
			hover.AutoExpandAt = time.Time{}
			state.SetDropHover(hover)
		} else if !hover.AutoExpandAt.IsZero() {
			c.After(hover.AutoExpandAt.Sub(now))
		}
	}
}

func pointerMovePassedThreshold(start, current ui.Point) bool {
	dx, dy := float64(current.X-start.X), float64(current.Y-start.Y)
	return math.Hypot(dx, dy) >= 4
}

func attachContextMenu[T any](element ui.Element, targetID string, actions []Command, dispatch func(Action[T])) {
	if len(actions) == 0 {
		return
	}
	element.ContextMenu(func(m *ui.Menu) {
		for _, command := range actions {
			command := command
			if m.Item(command.Label).Disabled(!command.Enabled).Chosen() && command.Enabled {
				dispatchAction(dispatch, Action[T]{Kind: InvokeContextCommand, ID: targetID, CommandID: command.ID})
			}
		}
	})
}

func emitDefaultAction[T any](entry Entry, dispatch func(Action[T])) {
	if entry.Kind == Directory && entry.CanExpand {
		dispatchAction(dispatch, Action[T]{Kind: ToggleEntry, ID: entry.ID})
	} else if entry.CanOpen {
		dispatchAction(dispatch, Action[T]{Kind: OpenEntry, ID: entry.ID})
	}
}

func dispatchAction[T any](dispatch func(Action[T]), action Action[T]) {
	if dispatch != nil {
		dispatch(action)
	}
}

func dragIDs[T any](drag DragPort[T], payload T) []string {
	if drag.IDs == nil {
		return nil
	}
	return cloneIDs(drag.IDs(payload))
}

func cloneIDs(ids []string) []string {
	return append([]string(nil), ids...)
}
