// Package workspacepresenter maps durable workspace tab/split state onto
// MyGo split elements and asks the window host to render each pane's content.
package workspacepresenter

import (
	"github.com/egoist/mygo/ui"

	"muse/nativeui/workbench/hosting"
	"muse/workspace"
)

type ActionKind uint8

const (
	FocusPane ActionKind = iota
	SplitRatioChanged
)

type Action struct {
	Kind      ActionKind
	TabID     workspace.TabID
	PaneID    workspace.PaneID
	SplitPath string
	Ratio     float64
}

type Presenter struct{}

func New() *Presenter { return &Presenter{} }

// Render shows the active tab's full split tree. Each leaf delegates its
// content to renderPane; the presenter never inspects the content state.
func (p *Presenter) Render(
	c *ui.Context,
	state workspace.State,
	host hosting.HostID,
	renderPane func(*ui.Context, hosting.HostID, hosting.PaneID),
	dispatch func(Action),
) {
	if p == nil || renderPane == nil || state.ActiveTab == "" {
		return
	}
	var active *workspace.Tab
	for i := range state.Tabs {
		if state.Tabs[i].ID == state.ActiveTab {
			active = &state.Tabs[i]
			break
		}
	}
	if active == nil {
		return
	}
	p.renderNode(c, state.ActiveTab, "root", active.Layout, host, renderPane, dispatch)
}

func (p *Presenter) renderNode(
	c *ui.Context,
	tabID workspace.TabID,
	path string,
	node workspace.LayoutNode,
	host hosting.HostID,
	renderPane func(*ui.Context, hosting.HostID, hosting.PaneID),
	dispatch func(Action),
) {
	if node.Pane != nil {
		paneID := node.Pane.ID
		paneContext := c.Key(paneKey{tab: tabID, pane: paneID})
		ui.Box(paneContext).Grow(1).Fill().MinWidth(0).MinHeight(0).HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputPointerDown && dispatch != nil {
				dispatch(Action{Kind: FocusPane, TabID: tabID, PaneID: paneID})
			}
			return false
		}).Children(func() { renderPane(c, host, hosting.PaneID(paneID)) })
		return
	}
	split := node.Split
	if split == nil {
		return
	}
	// The durable workspace model is authoritative. The toolkit mutates this
	// frame-local value while dragging and the typed action writes it back.
	percent := float32(split.Ratio * 100)
	firstPath, nextPath := path+"/first", path+"/next"
	first := func() { p.renderNode(c, tabID, firstPath, split.First, host, renderPane, dispatch) }
	next := func() { p.renderNode(c, tabID, nextPath, split.Next, host, renderPane, dispatch) }
	splitContext := c.Key(splitKey{tab: tabID, path: path})
	var element ui.Element
	if split.Axis == workspace.Vertical {
		element = ui.SplitVerticalPercent(splitContext, &percent, first, next)
	} else {
		element = ui.SplitPercent(splitContext, &percent, first, next)
	}
	element.Grow(1).MinWidth(0).MinHeight(0)
	if element.Changed() && dispatch != nil {
		dispatch(Action{Kind: SplitRatioChanged, TabID: tabID, SplitPath: path, Ratio: float64(percent) / 100})
	}
}

type paneKey struct {
	tab  workspace.TabID
	pane workspace.PaneID
}

type splitKey struct {
	tab  workspace.TabID
	path string
}
