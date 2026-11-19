package workspace

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"

	domain "muse/workspace"
)

var (
	ErrSessionClosed = errors.New("application/workspace: session is closed")
	ErrUnknownTab    = errors.New("application/workspace: tab does not exist")
	ErrUnknownPane   = errors.New("application/workspace: pane does not exist")
	ErrUnknownSplit  = errors.New("application/workspace: split does not exist")
)

// Session coordinates the durable workspace snapshot and ephemeral focus
// history. It never owns editor buffers or rendered pane contents.
type Session struct {
	mu          sync.RWMutex
	repository  domain.Repository
	state       domain.State
	recentFocus map[domain.TabID][]domain.PaneID
	recentTabs  []domain.TabID
	closed      bool
}

func NewSession(repository domain.Repository, initial domain.State) (*Session, error) {
	if initial.Version == 0 {
		initial.Version = 1
	}
	if err := initial.Validate(); err != nil {
		return nil, err
	}
	root, err := cleanAbsolutePath(initial.VaultPath)
	if err != nil {
		return nil, err
	}
	initial.VaultPath = root
	if err := validateOwnedDocuments(initial); err != nil {
		return nil, err
	}
	s := &Session{repository: repository, state: cloneState(initial), recentFocus: make(map[domain.TabID][]domain.PaneID)}
	for _, tab := range initial.Tabs {
		if tab.ActivePaneID != "" {
			s.rememberFocusLocked(tab.ID, tab.ActivePaneID)
		}
	}
	if initial.ActiveTab != "" {
		s.rememberTabLocked(initial.ActiveTab)
	}
	return s, nil
}

func (s *Session) Snapshot() domain.State {
	if s == nil {
		return domain.State{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
}

// SwitchVault replaces the portable workspace state when its vault identity
// changes or its current document lies outside the target vault. The plan
// identifies why surfaces must reset and which default tab the host should use.
func (s *Session) SwitchVault(path, activeDocumentPath string) (VaultSwitchPlan, error) {
	if s == nil {
		return VaultSwitchPlan{}, ErrSessionClosed
	}
	target, err := cleanAbsolutePath(path)
	if err != nil {
		return VaultSwitchPlan{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return VaultSwitchPlan{}, ErrSessionClosed
	}
	previous, err := cleanAbsolutePath(s.state.VaultPath)
	if err != nil {
		return VaultSwitchPlan{}, err
	}
	activePath, err := cleanAbsolutePath(activeDocumentPath)
	if err != nil {
		return VaultSwitchPlan{}, err
	}
	vaultChanged := previous != target
	documentOutside := activePath != "" && !isOwnedDocumentPath(target, activePath)
	cause := VaultResetNone
	if vaultChanged && documentOutside {
		cause = VaultResetRootAndDocumentOutsideTarget
	} else if vaultChanged {
		cause = VaultResetRootChanged
	} else if documentOutside {
		cause = VaultResetDocumentOutsideTarget
	}
	reset := cause != VaultResetNone
	if !reset {
		plan, err := makeRestorePlan(s.state)
		if err != nil {
			return VaultSwitchPlan{}, err
		}
		s.state = plan.State
		return VaultSwitchPlan{
			PreviousVaultPath: previous, VaultPath: target, ActiveDocumentPath: activePath,
			State: plan.State, DefaultTab: plan.DefaultTab,
		}, nil
	}
	state := domain.State{Version: 1, VaultPath: target}
	s.state = state
	s.recentFocus = make(map[domain.TabID][]domain.PaneID)
	s.recentTabs = nil
	return VaultSwitchPlan{
		PreviousVaultPath: previous, VaultPath: target, ActiveDocumentPath: activePath,
		VaultChanged: vaultChanged, ResetWorkspace: true, Cause: cause,
		State: state, DefaultTab: DefaultTabPlan{Decision: CreateBlankTab, TabID: DefaultTabID, PaneID: DefaultPaneID},
	}, nil
}

// RecordActiveDocument atomically updates the active pane's document/cursor
// and focus. If the workspace is empty, the application creates its canonical
// default tab and pane instead of asking the presentation layer to invent IDs.
func (s *Session) RecordActiveDocument(snapshot ActiveDocumentSnapshot) (ActiveDocumentPlan, error) {
	if s == nil {
		return ActiveDocumentPlan{}, ErrSessionClosed
	}
	if !filepath.IsAbs(snapshot.Path) || strings.TrimSpace(snapshot.Path) == "" || snapshot.CursorOffset < 0 {
		return ActiveDocumentPlan{}, fmt.Errorf("application/workspace: active document path and cursor are invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ActiveDocumentPlan{}, ErrSessionClosed
	}
	path := filepath.Clean(snapshot.Path)
	if err := validateOwnedDocument(s.state.VaultPath, path); err != nil {
		return ActiveDocumentPlan{}, err
	}
	candidate := cloneState(s.state)
	createdDefault := len(candidate.Tabs) == 0
	var tab *domain.Tab
	if createdDefault {
		doc := domain.Document{Path: path, CursorOffset: snapshot.CursorOffset}
		candidate.Tabs = []domain.Tab{{
			ID: DefaultTabID, ActivePaneID: DefaultPaneID,
			Layout: domain.Leaf(domain.Pane{ID: DefaultPaneID, Document: doc}),
		}}
		candidate.ActiveTab = DefaultTabID
		tab = &candidate.Tabs[0]
	} else {
		i := tabIndex(candidate.Tabs, candidate.ActiveTab)
		if i < 0 {
			return ActiveDocumentPlan{}, fmt.Errorf("%w: active tab %q", ErrUnknownTab, candidate.ActiveTab)
		}
		tab = &candidate.Tabs[i]
		pane := tab.ActivePaneID
		if snapshot.PaneID != "" {
			if !containsPane(tab.Layout, snapshot.PaneID) {
				return ActiveDocumentPlan{}, ErrUnknownPane
			}
			pane = snapshot.PaneID
		}
		if !containsPane(tab.Layout, pane) {
			pane, _ = firstPane(tab.Layout)
		}
		if !updatePane(&tab.Layout, pane, func(p *domain.Pane) {
			p.Document = domain.Document{Path: path, CursorOffset: snapshot.CursorOffset}
		}) {
			return ActiveDocumentPlan{}, ErrUnknownPane
		}
		tab.ActivePaneID = pane
		candidate.ActiveTab = tab.ID
	}
	if err := candidate.Validate(); err != nil {
		return ActiveDocumentPlan{}, err
	}
	s.state = candidate
	s.rememberFocusLocked(tab.ID, tab.ActivePaneID)
	s.rememberTabLocked(tab.ID)
	return ActiveDocumentPlan{State: cloneState(candidate), TabID: tab.ID, PaneID: tab.ActivePaneID, CreatedDefaultTab: createdDefault}, nil
}

// Restore atomically applies a validated, vault-confined state and returns
// the exact document/focus plan for the presentation host.
func (s *Session) Restore(state domain.State) (RestorePlan, error) {
	if s == nil {
		return RestorePlan{}, ErrSessionClosed
	}
	plan, err := makeRestorePlan(state)
	if err != nil {
		return RestorePlan{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return RestorePlan{}, ErrSessionClosed
	}
	s.state = cloneState(plan.State)
	s.recentFocus = make(map[domain.TabID][]domain.PaneID)
	s.recentTabs = nil
	for _, tab := range plan.State.Tabs {
		if tab.ActivePaneID != "" {
			s.rememberFocusLocked(tab.ID, tab.ActivePaneID)
		}
	}
	if plan.State.ActiveTab != "" {
		s.rememberTabLocked(plan.State.ActiveTab)
	}
	return plan, nil
}

func (s *Session) Focus(tabID domain.TabID, paneID domain.PaneID) error {
	if s == nil {
		return ErrSessionClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	i := tabIndex(s.state.Tabs, tabID)
	if i < 0 {
		return ErrUnknownTab
	}
	if !containsPane(s.state.Tabs[i].Layout, paneID) {
		return ErrUnknownPane
	}
	s.state.ActiveTab = tabID
	s.state.Tabs[i].ActivePaneID = paneID
	s.rememberFocusLocked(tabID, paneID)
	s.rememberTabLocked(tabID)
	return nil
}

// ActivateTab restores the tab's last active pane, or its first leaf when it
// has never had a focused pane.
func (s *Session) ActivateTab(tabID domain.TabID) (domain.PaneID, error) {
	if s == nil {
		return "", ErrSessionClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrSessionClosed
	}
	i := tabIndex(s.state.Tabs, tabID)
	if i < 0 {
		return "", ErrUnknownTab
	}
	tab := &s.state.Tabs[i]
	paneID := tab.ActivePaneID
	if !containsPane(tab.Layout, paneID) {
		paneID, _ = firstPane(tab.Layout)
		tab.ActivePaneID = paneID
	}
	s.state.ActiveTab = tabID
	s.rememberFocusLocked(tabID, paneID)
	s.rememberTabLocked(tabID)
	return paneID, nil
}

// CreateTab adds a validated durable tab. When activate is true the new tab
// becomes current; otherwise the existing active tab remains unchanged.
func (s *Session) CreateTab(tab domain.Tab, activate bool) error {
	if s == nil {
		return ErrSessionClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	if tab.ID == "" || tab.ActivePaneID == "" || tab.ID == s.state.ActiveTab || tabIndex(s.state.Tabs, tab.ID) >= 0 {
		return fmt.Errorf("%w: tab identity or active pane is invalid", domain.ErrInvalidState)
	}
	if err := validateOwnedTab(s.state.VaultPath, tab); err != nil {
		return err
	}
	candidate := cloneState(s.state)
	candidate.Tabs = append(candidate.Tabs, cloneTab(tab))
	if activate || candidate.ActiveTab == "" {
		candidate.ActiveTab = tab.ID
	}
	if err := candidate.Validate(); err != nil {
		return err
	}
	s.state = candidate
	s.rememberFocusLocked(tab.ID, tab.ActivePaneID)
	if candidate.ActiveTab == tab.ID {
		s.rememberTabLocked(tab.ID)
	}
	return nil
}

// CloseTab removes a tab and selects the most recently active surviving tab,
// falling back to the first tab when no runtime focus history remains.
func (s *Session) CloseTab(tabID domain.TabID) (domain.TabID, error) {
	if s == nil {
		return "", ErrSessionClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrSessionClosed
	}
	index := tabIndex(s.state.Tabs, tabID)
	if index < 0 {
		return "", ErrUnknownTab
	}
	candidate := cloneState(s.state)
	candidate.Tabs = append(candidate.Tabs[:index], candidate.Tabs[index+1:]...)
	if candidate.ActiveTab == tabID {
		candidate.ActiveTab = focusTabFallback(s.recentTabs, candidate.Tabs)
	}
	if err := candidate.Validate(); err != nil {
		return "", err
	}
	s.state = candidate
	delete(s.recentFocus, tabID)
	s.pruneTabsLocked()
	return candidate.ActiveTab, nil
}

// MoveTab reorders a tab to the given zero-based position without changing
// active-tab identity. An invalid target index leaves the snapshot untouched.
func (s *Session) MoveTab(tabID domain.TabID, index int) error {
	if s == nil {
		return ErrSessionClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	from := tabIndex(s.state.Tabs, tabID)
	if from < 0 {
		return ErrUnknownTab
	}
	if index < 0 || index >= len(s.state.Tabs) {
		return fmt.Errorf("%w: tab index %d is out of range", domain.ErrInvalidState, index)
	}
	candidate := cloneState(s.state)
	tab := candidate.Tabs[from]
	candidate.Tabs = append(candidate.Tabs[:from], candidate.Tabs[from+1:]...)
	candidate.Tabs = append(candidate.Tabs, domain.Tab{})
	copy(candidate.Tabs[index+1:], candidate.Tabs[index:])
	candidate.Tabs[index] = tab
	if err := candidate.Validate(); err != nil {
		return err
	}
	s.state = candidate
	return nil
}

func (s *Session) UpdateDocument(tabID domain.TabID, paneID domain.PaneID, doc domain.Document) error {
	if s == nil {
		return ErrSessionClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	i := tabIndex(s.state.Tabs, tabID)
	if i < 0 {
		return ErrUnknownTab
	}
	if strings.TrimSpace(doc.Path) == "" || doc.CursorOffset < 0 {
		return fmt.Errorf("application/workspace: document path and cursor offset are invalid")
	}
	if err := validateOwnedDocument(s.state.VaultPath, doc.Path); err != nil {
		return err
	}
	if !updatePane(&s.state.Tabs[i].Layout, paneID, func(p *domain.Pane) { p.Document = doc }) {
		return ErrUnknownPane
	}
	return nil
}

// SetLayout replaces one tab's complete split tree. Existing active focus is
// retained when its pane survives; otherwise the first pane becomes active.
func (s *Session) SetLayout(tabID domain.TabID, layout domain.LayoutNode) error {
	if s == nil {
		return ErrSessionClosed
	}
	if err := (domain.State{Version: 1, ActiveTab: tabID, Tabs: []domain.Tab{{ID: tabID, Layout: layout}}}).Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	if err := validateOwnedLayout(s.state.VaultPath, layout); err != nil {
		return err
	}
	i := tabIndex(s.state.Tabs, tabID)
	if i < 0 {
		return ErrUnknownTab
	}
	s.state.Tabs[i].Layout = cloneNode(layout)
	if !containsPane(layout, s.state.Tabs[i].ActivePaneID) {
		pane, _ := firstPane(layout)
		s.state.Tabs[i].ActivePaneID = pane
	}
	return nil
}

// SetSplitRatio updates a split identified by its stable tree path ("root",
// "root/first", ...). The ratio remains a domain value, not a UI width.
func (s *Session) SetSplitRatio(tabID domain.TabID, nodePath string, ratio float64) error {
	if s == nil {
		return ErrSessionClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	i := tabIndex(s.state.Tabs, tabID)
	if i < 0 {
		return ErrUnknownTab
	}
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 0 || ratio >= 1 {
		return fmt.Errorf("%w: split ratio must be between 0 and 1", domain.ErrInvalidState)
	}
	candidate := cloneState(s.state)
	if !setSplitRatio(&candidate.Tabs[i].Layout, nodePath, ratio) {
		return fmt.Errorf("%w: %q", ErrUnknownSplit, nodePath)
	}
	if err := candidate.Validate(); err != nil {
		return err
	}
	s.state = candidate
	return nil
}

// ClosePane removes the leaf and collapses its parent split. When the focused
// pane closes, focus returns to the most recently focused surviving pane.
func (s *Session) ClosePane(tabID domain.TabID, paneID domain.PaneID) (domain.PaneID, error) {
	if s == nil {
		return "", ErrSessionClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrSessionClosed
	}
	i := tabIndex(s.state.Tabs, tabID)
	if i < 0 {
		return "", ErrUnknownTab
	}
	candidate := cloneState(s.state)
	tab := &candidate.Tabs[i]
	if !containsPane(tab.Layout, paneID) {
		return "", ErrUnknownPane
	}
	if paneCount(tab.Layout) == 1 {
		return "", ErrUnknownPane
	}
	removePane(&tab.Layout, paneID)
	if tab.ActivePaneID == paneID {
		tab.ActivePaneID = focusFallback(s.recentFocus[tabID], tab.Layout)
	}
	if err := candidate.Validate(); err != nil {
		return "", err
	}
	s.state = candidate
	s.pruneFocusLocked(tabID, tab.Layout)
	if candidate.ActiveTab == tabID {
		s.rememberFocusLocked(tabID, tab.ActivePaneID)
	}
	return tab.ActivePaneID, nil
}

func (s *Session) FocusHistory(tabID domain.TabID) []domain.PaneID {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.PaneID(nil), s.recentFocus[tabID]...)
}

func (s *Session) Persist() error {
	if s == nil {
		return ErrSessionClosed
	}
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrSessionClosed
	}
	repository, state := s.repository, cloneState(s.state)
	s.mu.RUnlock()
	if repository == nil {
		return ErrUnavailable
	}
	state, err := stateForPersistence(state)
	if err != nil {
		return err
	}
	return repository.Save(state)
}

func (s *Session) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

func (s *Session) rememberFocusLocked(tab domain.TabID, pane domain.PaneID) {
	if pane == "" {
		return
	}
	history := s.recentFocus[tab]
	filtered := make([]domain.PaneID, 0, len(history)+1)
	filtered = append(filtered, pane)
	for _, prior := range history {
		if prior != pane {
			filtered = append(filtered, prior)
		}
	}
	s.recentFocus[tab] = filtered
}

func (s *Session) rememberTabLocked(tab domain.TabID) {
	if tab == "" {
		return
	}
	history := make([]domain.TabID, 0, len(s.recentTabs)+1)
	history = append(history, tab)
	for _, prior := range s.recentTabs {
		if prior != tab {
			history = append(history, prior)
		}
	}
	s.recentTabs = history
}

func (s *Session) pruneTabsLocked() {
	kept := make([]domain.TabID, 0, len(s.recentTabs))
	for _, id := range s.recentTabs {
		if tabIndex(s.state.Tabs, id) >= 0 {
			kept = append(kept, id)
		}
	}
	s.recentTabs = kept
}

func focusTabFallback(history []domain.TabID, tabs []domain.Tab) domain.TabID {
	for _, id := range history {
		if tabIndex(tabs, id) >= 0 {
			return id
		}
	}
	if len(tabs) == 0 {
		return ""
	}
	return tabs[0].ID
}

func (s *Session) pruneFocusLocked(tab domain.TabID, layout domain.LayoutNode) {
	history := s.recentFocus[tab]
	kept := make([]domain.PaneID, 0, len(history))
	for _, pane := range history {
		if containsPane(layout, pane) {
			kept = append(kept, pane)
		}
	}
	s.recentFocus[tab] = kept
}

func focusFallback(history []domain.PaneID, layout domain.LayoutNode) domain.PaneID {
	for _, pane := range history {
		if containsPane(layout, pane) {
			return pane
		}
	}
	pane, _ := firstPane(layout)
	return pane
}

func tabIndex(tabs []domain.Tab, id domain.TabID) int {
	for i := range tabs {
		if tabs[i].ID == id {
			return i
		}
	}
	return -1
}

func containsPane(node domain.LayoutNode, id domain.PaneID) bool {
	if id == "" {
		return false
	}
	if node.Pane != nil {
		return node.Pane.ID == id
	}
	return containsPane(node.Split.First, id) || containsPane(node.Split.Next, id)
}

func firstPane(node domain.LayoutNode) (domain.PaneID, bool) {
	if node.Pane != nil {
		return node.Pane.ID, true
	}
	if pane, ok := firstPane(node.Split.First); ok {
		return pane, true
	}
	return firstPane(node.Split.Next)
}

func paneCount(node domain.LayoutNode) int {
	if node.Pane != nil {
		return 1
	}
	return paneCount(node.Split.First) + paneCount(node.Split.Next)
}

func updatePane(node *domain.LayoutNode, id domain.PaneID, update func(*domain.Pane)) bool {
	if node.Pane != nil {
		if node.Pane.ID != id {
			return false
		}
		update(node.Pane)
		return true
	}
	return updatePane(&node.Split.First, id, update) || updatePane(&node.Split.Next, id, update)
}

func setSplitRatio(node *domain.LayoutNode, nodePath string, ratio float64) bool {
	if node.Split == nil {
		return false
	}
	if nodePath == "root" {
		node.Split.Ratio = ratio
		return true
	}
	if len(nodePath) <= 5 || nodePath[:5] != "root/" {
		return false
	}
	remaining := nodePath[5:]
	if remaining == "first" {
		return setSplitRatio(&node.Split.First, "root", ratio)
	}
	if remaining == "next" {
		return setSplitRatio(&node.Split.Next, "root", ratio)
	}
	if len(remaining) > 6 && remaining[:6] == "first/" {
		return setSplitRatio(&node.Split.First, "root/"+remaining[6:], ratio)
	}
	if len(remaining) > 5 && remaining[:5] == "next/" {
		return setSplitRatio(&node.Split.Next, "root/"+remaining[5:], ratio)
	}
	return false
}

func removePane(node *domain.LayoutNode, id domain.PaneID) bool {
	if node.Split == nil {
		if node.Pane != nil && node.Pane.ID == id {
			*node = domain.LayoutNode{}
			return true
		}
		return false
	}
	if removePane(&node.Split.First, id) {
		if node.Split.First.Pane == nil && node.Split.First.Split == nil {
			*node = cloneNode(node.Split.Next)
		}
		return true
	}
	if removePane(&node.Split.Next, id) {
		if node.Split.Next.Pane == nil && node.Split.Next.Split == nil {
			*node = cloneNode(node.Split.First)
		}
		return true
	}
	return false
}

func cloneState(state domain.State) domain.State {
	copy := state
	copy.Tabs = append([]domain.Tab(nil), state.Tabs...)
	for i := range copy.Tabs {
		copy.Tabs[i].Layout = cloneNode(state.Tabs[i].Layout)
	}
	return copy
}

func cloneTab(tab domain.Tab) domain.Tab {
	tab.Layout = cloneNode(tab.Layout)
	return tab
}

func cloneNode(node domain.LayoutNode) domain.LayoutNode {
	if node.Pane != nil {
		pane := *node.Pane
		return domain.Leaf(pane)
	}
	if node.Split == nil {
		return domain.LayoutNode{}
	}
	return domain.Branch(node.Split.Axis, node.Split.Ratio, cloneNode(node.Split.First), cloneNode(node.Split.Next))
}
