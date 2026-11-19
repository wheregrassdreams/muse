package workspaceui

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"
	appworkspace "muse/application/workspace"
	domain "muse/workspace"
)

const MainHost HostID = "main"

// WindowSlots is the narrow portion of the generic hosting manager needed to
// render and manage content in workspace panes.
type WindowSlots interface {
	FocusedPane(HostID) (PaneID, bool)
	RenderPane(*ui.Context, HostID, PaneID)
	Close(Handle) error
	Focus(Handle) bool
}

// PaneRequest asks the root-owned document adapter to create a pane surface.
// Workspace UI passes identity and cursor state through without loading files
// or interpreting document data itself.
type PaneRequest struct {
	TabID        domain.TabID
	PaneID       domain.PaneID
	Path         string
	CursorOffset int
	Focused      bool
}

// PaneOpener is a narrow content-creation capability supplied by the root.
// It must return the handle produced by the generic window host.
type PaneOpener func(PaneRequest) (Handle, error)

// Ports contains only the host-slot operations needed by workspace UI.
type Ports struct {
	Windows  WindowSlots
	OpenPane PaneOpener
	Host     HostID
}

// Host owns the MyGo split projection and the handles occupying workspace
// pane slots. Durable workspace policy remains with application/workspace.
type Host struct {
	ports       Ports
	presenter   *presenter
	paneHandles map[domain.PaneID]Handle
}

func NewHost(ports Ports) *Host {
	if ports.Host == "" {
		ports.Host = MainHost
	}
	return &Host{
		ports: ports, presenter: newPresenter(), paneHandles: make(map[domain.PaneID]Handle),
	}
}

// Render projects the active tab's split tree and emits typed UI intents. The
// root routes focus and split-ratio actions to application/workspace.
func (h *Host) Render(c *ui.Context, state domain.State, dispatch func(Action)) {
	if h == nil || h.presenter == nil || h.ports.Windows == nil {
		return
	}
	if state.ActiveTab == "" {
		// An untitled note is an ephemeral workspace surface and therefore has
		// no durable document path/tab yet. The application restore plan still
		// gives it the canonical default pane identity; render that slot until a
		// successful save promotes it into durable workspace state.
		ui.Box(c).Key("workspace-empty-pane").Fill().MinWidth(0).MinHeight(0).Children(func() {
			h.ports.Windows.RenderPane(c, h.ports.Host, PaneID(appworkspace.DefaultPaneID))
		})
		return
	}
	h.presenter.Render(c, state, h.ports.Host, func(ctx *ui.Context, host HostID, pane PaneID) {
		h.ports.Windows.RenderPane(ctx, host, pane)
	}, dispatch)
}

// RenderSession owns the transient application-session updates produced by
// workspace focus and split gestures, then reports the resulting snapshot to
// the workbench composition root.
func (h *Host) RenderSession(
	c *ui.Context,
	session *appworkspace.Session,
	state domain.State,
	onFocus func(domain.PaneID),
	onUpdate func(domain.State),
) {
	if session == nil {
		h.Render(c, state, nil)
		return
	}
	h.Render(c, state, func(action Action) {
		switch action.Kind {
		case FocusPane:
			if err := session.Focus(action.TabID, action.PaneID); err == nil && onFocus != nil {
				onFocus(action.PaneID)
			}
		case SplitRatioChanged:
			_ = session.SetSplitRatio(action.TabID, action.SplitPath, action.Ratio)
		}
		if onUpdate != nil {
			onUpdate(session.Snapshot())
		}
	})
}

// FocusedPane returns the window host's pane focus, falling back to the
// application-selected pane when no hosted pane has received focus yet.
func (h *Host) FocusedPane(fallback domain.PaneID) domain.PaneID {
	if h != nil && h.ports.Windows != nil {
		if pane, ok := h.ports.Windows.FocusedPane(h.ports.Host); ok && pane != "" {
			return domain.PaneID(pane)
		}
	}
	return fallback
}

// OpenRestorePlan replaces current pane slots with the active tab's documents
// from an application-produced restore plan. Inactive tabs remain durable in
// the session and are opened when the root activates them.
func (h *Host) OpenRestorePlan(plan appworkspace.RestorePlan) error {
	if h == nil {
		return errors.New("workspaceui: host is unavailable")
	}
	h.ClosePaneSlots()
	if h.ports.OpenPane == nil {
		return errors.New("workspaceui: pane opener is unavailable")
	}
	requests := make([]PaneRequest, 0, len(plan.Documents))
	if plan.DefaultTab.Decision == appworkspace.CreateBlankTab {
		requests = append(requests, PaneRequest{TabID: plan.DefaultTab.TabID, PaneID: plan.DefaultTab.PaneID, Focused: true})
	} else {
		for _, doc := range plan.Documents {
			if doc.TabID == plan.ActiveTab {
				requests = append(requests, PaneRequest{
					TabID: doc.TabID, PaneID: doc.PaneID, Path: doc.Path,
					CursorOffset: doc.Cursor, Focused: doc.Focused,
				})
			}
		}
	}
	var failures []error
	for _, request := range requests {
		handle, err := h.ports.OpenPane(request)
		if err != nil {
			failures = append(failures, fmt.Errorf("workspaceui: open pane %q: %w", request.PaneID, err))
			continue
		}
		if handle.IsZero() {
			failures = append(failures, fmt.Errorf("workspaceui: pane opener returned empty handle for %q", request.PaneID))
			continue
		}
		h.RegisterPane(request.PaneID, handle)
		if request.Focused && h.ports.Windows != nil {
			h.ports.Windows.Focus(handle)
		}
	}
	return errors.Join(failures...)
}

// OpenPane replaces the live surface for one existing workspace pane. The
// caller supplies the descriptor selected by application/workspace; this host
// only delegates content creation and maintains the generic handle slot.
func (h *Host) OpenPane(request PaneRequest) (Handle, error) {
	if h == nil {
		return Handle{}, errors.New("workspaceui: host is unavailable")
	}
	if request.PaneID == "" || h.ports.OpenPane == nil {
		return Handle{}, errors.New("workspaceui: pane request is incomplete")
	}
	handle, err := h.ports.OpenPane(request)
	if err != nil {
		return Handle{}, err
	}
	if handle.IsZero() {
		return Handle{}, fmt.Errorf("workspaceui: pane opener returned empty handle for %q", request.PaneID)
	}
	h.RegisterPane(request.PaneID, handle)
	if request.Focused && h.ports.Windows != nil {
		h.ports.Windows.Focus(handle)
	}
	return handle, nil
}

// RegisterPane tracks a newly created or replaced content slot. When a slot
// already contains a different handle, the old surface is closed first.
func (h *Host) RegisterPane(pane domain.PaneID, handle Handle) {
	if h == nil || pane == "" || handle.IsZero() {
		return
	}
	if h.paneHandles == nil {
		h.paneHandles = make(map[domain.PaneID]Handle)
	}
	if previous := h.paneHandles[pane]; !previous.IsZero() && previous != handle && h.ports.Windows != nil {
		_ = h.ports.Windows.Close(previous)
	}
	h.paneHandles[pane] = handle
}

// ReleasePane forgets a slot whose surface was closed by the root or window
// manager. It does not close the handle a second time.
func (h *Host) ReleasePane(pane domain.PaneID) {
	if h == nil {
		return
	}
	delete(h.paneHandles, pane)
}

// ClosePaneSlot closes and forgets one pane surface. Closing a pane in the
// durable tab/split model remains an application/workspace transition.
func (h *Host) ClosePaneSlot(pane domain.PaneID) error {
	if h == nil {
		return nil
	}
	handle := h.paneHandles[pane]
	delete(h.paneHandles, pane)
	if handle.IsZero() || h.ports.Windows == nil {
		return nil
	}
	return h.ports.Windows.Close(handle)
}

func (h *Host) PaneHandle(pane domain.PaneID) Handle {
	if h == nil {
		return Handle{}
	}
	return h.paneHandles[pane]
}

// ClosePaneSlots closes every content surface currently registered to this
// workspace host and clears the slot map.
func (h *Host) ClosePaneSlots() {
	if h == nil {
		return
	}
	if h.ports.Windows != nil {
		for _, handle := range h.paneHandles {
			if !handle.IsZero() {
				_ = h.ports.Windows.Close(handle)
			}
		}
	}
	h.paneHandles = make(map[domain.PaneID]Handle)
}
