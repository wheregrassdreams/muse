// Package filetree renders a caller-projected file tree and translates user
// input into typed actions. Discovery, ordering, filtering, file operations,
// and drag/drop policy belong to the caller.
package filetree

import (
	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/ui/scrollbar"
)

type EntryKind uint8

const (
	File EntryKind = iota + 1
	Directory
)

// Entry is one already ordered and already visible tree row. ID is opaque to
// this package and should be stable while the row remains the same item.
type Entry struct {
	ID        string
	Label     string
	Detail    string
	Kind      EntryKind
	Depth     int
	Expanded  bool
	Selected  bool
	Focused   bool
	CanSelect bool
	CanExpand bool
	CanOpen   bool
	CanDrag   bool
	CanDrop   bool
	DragIDs   []string
	DropHint  bool
	Icon      *ui.SVG
	IconTint  ui.Color
}

// Command describes a caller-provided toolbar action such as create, search,
// or collapse-all. The panel renders it and returns its opaque ID.
type Command struct {
	ID      string
	Label   string
	Icon    *ui.SVG
	Enabled bool
}

type ScrollPort struct {
	List      *ui.ListState
	State     *ui.ScrollState
	Indicator *scrollbar.Indicator
}

// ViewModel contains presentation rows and capabilities. The caller owns the
// file catalog, selection policy, expanded-node state, and list/scroll state.
type ViewModel struct {
	Entries      []Entry
	Commands     []Command
	EmptyMessage string
	Focused      bool
	Scroll       ScrollPort
}

type ActionKind uint8

const (
	SelectEntry ActionKind = iota + 1
	ToggleEntry
	OpenEntry
	InvokeCommand
	DropEntries
)

// Action contains presentation intent only. In particular, DropEntries does
// not imply move/copy/reorder; the caller decides what the target means.
type Action struct {
	Kind      ActionKind
	ID        string
	CommandID string
	ItemIDs   []string
	Modifiers ui.Modifiers
}

type dragPayload struct{ IDs []string }

func Render(c *ui.Context, model ViewModel, dispatch func(Action)) {
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
					dispatchAction(dispatch, Action{Kind: InvokeCommand, CommandID: command.ID})
				}
			}
		})
	}

	if len(model.Entries) == 0 {
		message := model.EmptyMessage
		if message == "" {
			message = locales.FilesNoEditableText()
		}
		ui.Column(c).FillWidth().Grow(1).Center().Padding(20).Children(func() {
			ui.Text(c, message).FontSize(12).TextColor(t.TextMuted).Wrap()
		})
		return
	}

	var localScroll ui.ScrollState
	scroll := model.Scroll.State
	if scroll == nil {
		scroll = &localScroll
	}
	localSelected := selectedEntry(model.Entries)
	listState := model.Scroll.List
	if listState == nil {
		listState = &ui.ListState{Selected: &localSelected}
	}
	if listState.Selected == nil {
		listState.Selected = &localSelected
	}
	listState.Label = func(i int) string { return model.Entries[i].Label }
	listState.Key = func(i int) any { return model.Entries[i].ID }

	showScrollbar := model.Scroll.State != nil && model.Scroll.Indicator != nil &&
		model.Scroll.Indicator.Visible(c.Now(), model.Scroll.State.X, model.Scroll.State.Y)
	if showScrollbar {
		c.After(model.Scroll.Indicator.Remaining(c.Now()))
	}

	pointerActivated := false
	list := ui.List(c, listState, len(model.Entries), func(i int) {
		entry := model.Entries[i]
		row := ui.ButtonBase(c).Label(entry.Label).Height(28).FillWidth().Radius(5).Padding(0, 8, 0, 4+float32(max(entry.Depth, 0))*14).
			Gap(7).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if entry.Selected || entry.Focused || row.Hovered() || entry.DropHint {
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			if entry.Kind == Directory {
				label := ""
				if entry.Expanded {
					label = "⌄"
				} else {
					label = "›"
				}
				toggle := ui.ButtonBase(c).Label(label).Size(16, 20).Disabled(!entry.CanExpand).Padding(0).TextColor(t.TextMuted)
				if toggle.Clicked() && entry.CanExpand {
					pointerActivated = true
					dispatchAction(dispatch, Action{Kind: ToggleEntry, ID: entry.ID, Modifiers: toggle.ClickModifiers()})
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
				label := ui.Text(c, entry.Label).SingleLine().Ellipsis("…").FontSize(12).TextColor(t.Text)
				if entry.Selected || entry.Focused {
					label.FontWeight(600)
				}
				if entry.Detail != "" {
					ui.Text(c, entry.Detail).SingleLine().Ellipsis("…").FontSize(9.5).TextColor(t.TextMuted)
				}
			})
		})
		if entry.CanDrag {
			row.Drag(dragPayload{IDs: cloneIDs(entry.DragIDs)})
		}
		if entry.CanDrop {
			if payload, ok := ui.Drop[dragPayload](row); ok {
				dispatchAction(dispatch, Action{Kind: DropEntries, ID: entry.ID, ItemIDs: cloneIDs(payload.IDs)})
			}
			if _, over := ui.DragOver[dragPayload](row); over {
				row.Background(t.SurfaceHover)
			}
		}
		if entry.CanSelect || entry.CanExpand || entry.CanOpen {
			row.OnClick(func() {
				pointerActivated = true
				mods := row.ClickModifiers()
				if entry.CanSelect && listState.Selected != nil {
					*listState.Selected = i
				}
				if entry.CanSelect {
					dispatchAction(dispatch, Action{Kind: SelectEntry, ID: entry.ID, Modifiers: mods})
				}
				if mods&(ui.Cmd|ui.Shift) != 0 {
					return
				}
				emitDefaultAction(entry, dispatch)
			})
		}
	})
	list.FillWidth().Grow(1).MinHeight(0).Padding(0, 6).Gap(1).TrackScroll(scroll).Scrollbars(showScrollbar).Focusable()
	if list.Changed() && !pointerActivated && model.Focused && listState.Selected != nil {
		if i := *listState.Selected; i >= 0 && i < len(model.Entries) {
			entry := model.Entries[i]
			if entry.CanSelect {
				dispatchAction(dispatch, Action{Kind: SelectEntry, ID: entry.ID, Modifiers: c.Modifiers()})
			}
		}
	}
	if !pointerActivated && model.Focused && list.Submitted() && listState.Selected != nil {
		if i := *listState.Selected; i >= 0 && i < len(model.Entries) {
			emitDefaultAction(model.Entries[i], dispatch)
		}
	}
}

func selectedEntry(entries []Entry) int {
	for i, entry := range entries {
		if entry.Selected {
			return i
		}
	}
	return -1
}

func emitDefaultAction(entry Entry, dispatch func(Action)) {
	if entry.Kind == Directory && entry.CanExpand {
		dispatchAction(dispatch, Action{Kind: ToggleEntry, ID: entry.ID})
	} else if entry.CanOpen {
		dispatchAction(dispatch, Action{Kind: OpenEntry, ID: entry.ID})
	}
}

func dispatchAction(dispatch func(Action), action Action) {
	if dispatch != nil {
		dispatch(action)
	}
}

func cloneIDs(ids []string) []string {
	return append([]string(nil), ids...)
}
