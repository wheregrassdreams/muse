// Package window hosts logical panes and overlays inside a native app window.
// All manager operations and Content callbacks belong to the UI-owner thread.
package window

import (
	"errors"

	"github.com/egoist/mygo/ui"
)

type HostID string
type PaneID string

type Layer uint8

const (
	PaneContent Layer = iota
	Overlay
	ModalOverlay
)

type Placement struct {
	Host  HostID
	Pane  PaneID
	Layer Layer
}

type Handle struct{ id uint64 }

func (h Handle) IsZero() bool { return h.id == 0 }

type Content interface {
	Render(*ui.Context)
	Close()
}

type Controls interface {
	CloseSelf()
	FocusSelf()
}

type Bindable interface{ Bind(Controls) }

type FocusLifecycle interface {
	Focused()
	Blurred()
}

var (
	ErrModalAlreadyOpen = errors.New("window: a modal surface is already open")
	ErrInvalidContent   = errors.New("window: content is required")
	ErrInvalidPlacement = errors.New("window: host is required")
)

type entry struct {
	handle    Handle
	content   Content
	placement Placement
	restore   Handle
	closing   bool
}

type paneKey struct {
	host HostID
	pane PaneID
}

// Manager controls logical placement, focus and lifecycle without knowing a
// surface's private state or feature-specific actions.
type Manager struct {
	nextID        uint64
	entries       map[Handle]*entry
	order         []Handle
	focused       Handle
	focusedPanes  map[HostID]PaneID
	recentPanes   map[HostID][]PaneID
	activeContent map[paneKey]Handle
	modal         Handle
	fallbackFocus func()
	renderDepth   int
	deferred      []func()
}

func NewManager(fallbackFocus func()) *Manager {
	return &Manager{
		entries: make(map[Handle]*entry), focusedPanes: make(map[HostID]PaneID),
		recentPanes:   make(map[HostID][]PaneID),
		activeContent: make(map[paneKey]Handle), fallbackFocus: fallbackFocus,
	}
}

func (m *Manager) Open(content Content, placement Placement) (Handle, error) {
	if m == nil || content == nil {
		return Handle{}, ErrInvalidContent
	}
	if placement.Host == "" {
		return Handle{}, ErrInvalidPlacement
	}
	if placement.Layer == ModalOverlay && !m.modal.IsZero() {
		return Handle{}, ErrModalAlreadyOpen
	}
	m.nextID++
	handle := Handle{id: m.nextID}
	current := m.focused
	e := &entry{handle: handle, content: content, placement: placement, restore: current}
	m.entries[handle] = e
	m.order = append(m.order, handle)
	if placement.Layer == ModalOverlay {
		if e.placement.Layer != ModalOverlay {
			e.restore = m.focused
		}
		m.modal = handle
	}
	attach := func() {
		if m.entries[handle] != e {
			return
		}
		if bindable, ok := content.(Bindable); ok {
			bindable.Bind(controls{close: func() { _ = m.Close(handle) }, focus: func() { m.Focus(handle) }})
		}
		m.Focus(handle)
	}
	m.mutate(attach)
	return handle, nil
}

func (m *Manager) Close(handle Handle) error {
	if m == nil || handle.IsZero() {
		return nil
	}
	e := m.entries[handle]
	if e == nil || e.closing {
		return nil
	}
	e.closing = true
	delete(m.entries, handle)
	m.removeFromOrder(handle)
	if m.modal == handle {
		m.modal = Handle{}
	}
	var paneFallback Handle
	if e.placement.Layer == PaneContent {
		m.repairPane(e.placement.Host, e.placement.Pane)
		if pane := m.focusedPanes[e.placement.Host]; pane != "" {
			paneFallback = m.activeContent[paneKey{host: e.placement.Host, pane: pane}]
		}
	}
	wasFocused := m.focused == handle
	if wasFocused {
		m.focused = Handle{}
	}
	m.mutate(func() {
		if wasFocused {
			if lifecycle, ok := e.content.(FocusLifecycle); ok {
				lifecycle.Blurred()
			}
		}
		e.content.Close()
		if wasFocused && m.focused.IsZero() {
			if m.Contains(paneFallback) {
				m.Focus(paneFallback)
			} else if m.Contains(e.restore) {
				m.Focus(e.restore)
			} else if m.fallbackFocus != nil {
				m.fallbackFocus()
			}
		}
	})
	return nil
}

func (m *Manager) Focus(handle Handle) bool {
	if m == nil || m.entries[handle] == nil {
		return false
	}
	if !m.modal.IsZero() && handle != m.modal {
		return false
	}
	if m.focused == handle {
		return true
	}
	previous := m.entries[m.focused]
	m.focused = handle
	next := m.entries[handle]
	if next.placement.Layer == PaneContent && next.placement.Pane != "" {
		m.rememberPaneFocus(next.placement.Host, next.placement.Pane)
		m.activeContent[paneKey{host: next.placement.Host, pane: next.placement.Pane}] = handle
	}
	m.mutate(func() {
		if previous != nil {
			if lifecycle, ok := previous.content.(FocusLifecycle); ok {
				lifecycle.Blurred()
			}
		}
		if m.entries[handle] == next && next != nil {
			if lifecycle, ok := next.content.(FocusLifecycle); ok {
				lifecycle.Focused()
			}
		}
	})
	return true
}

func (m *Manager) Place(handle Handle, placement Placement) error {
	if m == nil || m.entries[handle] == nil {
		return nil
	}
	if placement.Host == "" {
		return ErrInvalidPlacement
	}
	e := m.entries[handle]
	previousPlacement := e.placement
	if placement.Layer == ModalOverlay && e.placement.Layer != ModalOverlay && !m.modal.IsZero() {
		return ErrModalAlreadyOpen
	}
	if e.placement.Layer == ModalOverlay && placement.Layer != ModalOverlay && m.modal == handle {
		m.modal = Handle{}
	}
	if placement.Layer == ModalOverlay {
		m.modal = handle
	}
	e.placement = placement
	if previousPlacement.Layer == PaneContent && (placement.Layer != PaneContent || previousPlacement.Host != placement.Host || previousPlacement.Pane != placement.Pane) {
		m.repairPane(previousPlacement.Host, previousPlacement.Pane)
	}
	if placement.Layer == ModalOverlay {
		m.Focus(handle)
	} else if placement.Layer == PaneContent && m.focused == handle && placement.Pane != "" {
		m.rememberPaneFocus(placement.Host, placement.Pane)
		m.activeContent[paneKey{host: placement.Host, pane: placement.Pane}] = handle
	}
	return nil
}

// FocusedPane returns the most recently focused pane for a host. Focusing an
// overlay does not erase the pane context underneath it, so application
// commands can resolve the active pane without inspecting feature state.
func (m *Manager) FocusedPane(host HostID) (PaneID, bool) {
	if m == nil || host == "" {
		return "", false
	}
	pane := m.focusedPanes[host]
	return pane, pane != ""
}

func (m *Manager) Render(c *ui.Context, host HostID, layer Layer) {
	m.render(c, host, layer, "", false)
}

// RenderPane renders the active generic content in one pane. Calling Focus
// on another content handle in that pane activates it, which is the runtime
// seam a tab presenter can use without teaching the manager about documents.
func (m *Manager) RenderPane(c *ui.Context, host HostID, pane PaneID) {
	if host == "" || pane == "" {
		return
	}
	m.render(c, host, PaneContent, pane, true)
}

func (m *Manager) render(c *ui.Context, host HostID, layer Layer, pane PaneID, onlyPane bool) {
	if m == nil {
		return
	}
	type renderEntry struct {
		handle    Handle
		entry     *entry
		placement Placement
	}
	snapshot := make([]renderEntry, 0, len(m.order))
	if onlyPane {
		handle := m.activeContent[paneKey{host: host, pane: pane}]
		if e := m.entries[handle]; e != nil && e.placement.Host == host && e.placement.Pane == pane && e.placement.Layer == PaneContent {
			snapshot = append(snapshot, renderEntry{handle: handle, entry: e, placement: e.placement})
		}
	} else {
		for _, handle := range m.order {
			if e := m.entries[handle]; e != nil && e.placement.Host == host && e.placement.Layer == layer {
				snapshot = append(snapshot, renderEntry{handle: handle, entry: e, placement: e.placement})
			}
		}
	}
	m.renderDepth++
	defer func() {
		m.renderDepth--
		if m.renderDepth == 0 {
			m.flushDeferred()
		}
	}()
	for _, item := range snapshot {
		e := m.entries[item.handle]
		if e == nil || e != item.entry || e.closing || e.placement != item.placement {
			continue
		}
		e.content.Render(c)
	}
}

// HandleEscape dismisses the sole modal and consumes Escape when one exists.
func (m *Manager) HandleEscape() bool {
	if m == nil || m.modal.IsZero() {
		return false
	}
	_ = m.Close(m.modal)
	return true
}

func (m *Manager) HasModal() bool { return m != nil && !m.modal.IsZero() }

func (m *Manager) Contains(handle Handle) bool {
	return m != nil && !handle.IsZero() && m.entries[handle] != nil
}

func (m *Manager) CloseAll() {
	if m == nil {
		return
	}
	if len(m.entries) == 0 && m.focused.IsZero() && m.modal.IsZero() {
		return
	}
	closing := make([]*entry, 0, len(m.order))
	focused := m.entries[m.focused]
	for i := len(m.order) - 1; i >= 0; i-- {
		handle := m.order[i]
		if e := m.entries[handle]; e != nil && !e.closing {
			e.closing = true
			closing = append(closing, e)
		}
		delete(m.entries, handle)
	}
	m.order = nil
	m.focused, m.modal = Handle{}, Handle{}
	clear(m.focusedPanes)
	clear(m.recentPanes)
	clear(m.activeContent)
	m.mutate(func() {
		if focused != nil {
			if lifecycle, ok := focused.content.(FocusLifecycle); ok {
				lifecycle.Blurred()
			}
		}
		for _, e := range closing {
			e.content.Close()
		}
		if m.fallbackFocus != nil {
			m.fallbackFocus()
		}
	})
}

func (m *Manager) repairPane(host HostID, pane PaneID) {
	if m == nil || host == "" || pane == "" {
		return
	}
	key := paneKey{host: host, pane: pane}
	if active := m.entries[m.activeContent[key]]; active == nil || active.placement.Host != host || active.placement.Pane != pane || active.placement.Layer != PaneContent {
		delete(m.activeContent, key)
		for i := len(m.order) - 1; i >= 0; i-- {
			handle := m.order[i]
			e := m.entries[handle]
			if e != nil && e.placement.Layer == PaneContent && e.placement.Host == host && e.placement.Pane == pane {
				m.activeContent[key] = handle
				break
			}
		}
	}
	if m.focusedPanes[host] != pane {
		return
	}
	for i := len(m.order) - 1; i >= 0; i-- {
		e := m.entries[m.order[i]]
		if e != nil && e.placement.Layer == PaneContent && e.placement.Host == host && e.placement.Pane == pane {
			return
		}
	}
	for i := len(m.recentPanes[host]) - 1; i >= 0; i-- {
		candidate := m.recentPanes[host][i]
		if candidate != pane && m.hasPaneContent(host, candidate) {
			m.rememberPaneFocus(host, candidate)
			return
		}
	}
	for i := len(m.order) - 1; i >= 0; i-- {
		e := m.entries[m.order[i]]
		if e != nil && e.placement.Layer == PaneContent && e.placement.Host == host && e.placement.Pane != "" {
			m.rememberPaneFocus(host, e.placement.Pane)
			return
		}
	}
	delete(m.focusedPanes, host)
	delete(m.recentPanes, host)
}

func (m *Manager) hasPaneContent(host HostID, pane PaneID) bool {
	for _, handle := range m.order {
		if e := m.entries[handle]; e != nil && e.placement.Layer == PaneContent && e.placement.Host == host && e.placement.Pane == pane {
			return true
		}
	}
	return false
}

func (m *Manager) rememberPaneFocus(host HostID, pane PaneID) {
	if host == "" || pane == "" {
		return
	}
	recent := m.recentPanes[host]
	for i, existing := range recent {
		if existing == pane {
			copy(recent[i:], recent[i+1:])
			recent = recent[:len(recent)-1]
			break
		}
	}
	m.recentPanes[host] = append(recent, pane)
	m.focusedPanes[host] = pane
}

// mutate defers lifecycle callbacks until the outermost Render has finished.
// Registry changes happen synchronously so handles and modal exclusivity stay
// deterministic while the render snapshot remains stable.
func (m *Manager) mutate(fn func()) {
	if m.renderDepth > 0 {
		m.deferred = append(m.deferred, fn)
		return
	}
	fn()
}

func (m *Manager) flushDeferred() {
	for len(m.deferred) > 0 {
		batch := m.deferred
		m.deferred = nil
		for _, fn := range batch {
			fn()
		}
	}
}

func (m *Manager) removeFromOrder(handle Handle) {
	for i, current := range m.order {
		if current == handle {
			copy(m.order[i:], m.order[i+1:])
			m.order = m.order[:len(m.order)-1]
			return
		}
	}
}

type controls struct {
	close func()
	focus func()
}

func (c controls) CloseSelf() {
	if c.close != nil {
		c.close()
	}
}

func (c controls) FocusSelf() {
	if c.focus != nil {
		c.focus()
	}
}
