package workspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	domain "muse/workspace"
)

var (
	ErrInvalidWorkspacePath = errors.New("application/workspace: invalid vault path")
	ErrDocumentOutsideVault = errors.New("application/workspace: document is outside its vault")
)

const (
	DefaultTabID  domain.TabID  = "main"
	DefaultPaneID domain.PaneID = "editor"
)

// DocumentRestore is one validated document descriptor in stable tab and
// depth-first pane order. The host can create each pane without reinterpreting
// persisted paths or deciding which cursor/focus should win.
type DocumentRestore struct {
	TabID   domain.TabID
	PaneID  domain.PaneID
	Path    string
	Cursor  int
	Focused bool
}

// RestorePlan is the application decision for rebuilding a workspace. State
// has passed structural validation and every document belongs to VaultPath.
// OpenDefaultDocument asks the host to create its normal blank document when
// no persisted document survives validation.
type RestorePlan struct {
	State      domain.State
	VaultPath  string
	ActiveTab  domain.TabID
	ActivePane domain.PaneID
	Documents  []DocumentRestore
	DefaultTab DefaultTabPlan
}

type DefaultTabDecision uint8

const (
	UseRestoredTabs DefaultTabDecision = iota
	CreateBlankTab
)

// DefaultTabPlan makes the empty-workspace fallback explicit and stable for a
// host without letting the host invent tab/pane identities.
type DefaultTabPlan struct {
	Decision DefaultTabDecision
	TabID    domain.TabID
	PaneID   domain.PaneID
}

// VaultSwitchPlan describes the state replacement required when changing
// workspace identity. It never opens panes or touches live host handles.
type VaultSwitchPlan struct {
	PreviousVaultPath  string
	VaultPath          string
	ActiveDocumentPath string
	VaultChanged       bool
	ResetWorkspace     bool
	Cause              VaultResetCause
	State              domain.State
	DefaultTab         DefaultTabPlan
}

type VaultResetCause uint8

const (
	VaultResetNone VaultResetCause = iota
	VaultResetRootChanged
	VaultResetDocumentOutsideTarget
	VaultResetRootAndDocumentOutsideTarget
)

// ActiveDocumentSnapshot is the small durable portion the presentation host
// knows at the transition boundary. PaneID is an opaque domain pane identity
// reported by the current host; an empty value uses the session's active pane.
type ActiveDocumentSnapshot struct {
	Path         string
	CursorOffset int
	PaneID       domain.PaneID
}

// ActiveDocumentPlan is the application result after recording a cursor and
// choosing the tab/pane that owns the active document.
type ActiveDocumentPlan struct {
	State             domain.State
	TabID             domain.TabID
	PaneID            domain.PaneID
	CreatedDefaultTab bool
}

func makeRestorePlan(state domain.State) (RestorePlan, error) {
	if state.Version == 0 {
		if len(state.Tabs) != 0 {
			return RestorePlan{}, fmt.Errorf("%w: version is required for tabbed state", domain.ErrInvalidState)
		}
		state.Version = 1
	}
	if err := state.Validate(); err != nil {
		return RestorePlan{}, err
	}
	root, err := cleanAbsolutePath(state.VaultPath)
	if err != nil {
		return RestorePlan{}, err
	}

	cleaned := domain.State{Version: state.Version, VaultPath: root}
	for _, original := range state.Tabs {
		layout, ok := keepOwnedLayout(root, original.Layout)
		if !ok {
			continue
		}
		tab := cloneTab(original)
		tab.Layout = layout
		if !containsPane(layout, tab.ActivePaneID) {
			tab.ActivePaneID, _ = firstPane(layout)
		}
		cleaned.Tabs = append(cleaned.Tabs, tab)
	}
	if len(cleaned.Tabs) > 0 {
		if tabIndex(cleaned.Tabs, state.ActiveTab) >= 0 {
			cleaned.ActiveTab = state.ActiveTab
		} else {
			cleaned.ActiveTab = cleaned.Tabs[0].ID
		}
	}
	if err := cleaned.Validate(); err != nil {
		return RestorePlan{}, err
	}

	plan := RestorePlan{
		State: cleaned, VaultPath: root, ActiveTab: cleaned.ActiveTab,
	}
	if len(cleaned.Tabs) == 0 {
		plan.DefaultTab = DefaultTabPlan{Decision: CreateBlankTab, TabID: DefaultTabID, PaneID: DefaultPaneID}
	}
	for _, tab := range cleaned.Tabs {
		for _, pane := range tab.Panes() {
			focused := tab.ID == cleaned.ActiveTab && pane.ID == tab.ActivePaneID
			plan.Documents = append(plan.Documents, DocumentRestore{
				TabID: tab.ID, PaneID: pane.ID, Path: pane.Document.Path,
				Cursor: pane.Document.CursorOffset, Focused: focused,
			})
			if focused {
				plan.ActivePane = pane.ID
			}
		}
	}
	return plan, nil
}

func keepOwnedLayout(root string, node domain.LayoutNode) (domain.LayoutNode, bool) {
	if node.Pane != nil {
		path := node.Pane.Document.Path
		if !isOwnedDocumentPath(root, path) {
			return domain.LayoutNode{}, false
		}
		pane := *node.Pane
		pane.Document.Path = filepath.Clean(path)
		return domain.Leaf(pane), true
	}
	first, firstOK := keepOwnedLayout(root, node.Split.First)
	second, secondOK := keepOwnedLayout(root, node.Split.Next)
	switch {
	case firstOK && secondOK:
		return domain.Branch(node.Split.Axis, node.Split.Ratio, first, second), true
	case firstOK:
		return first, true
	case secondOK:
		return second, true
	default:
		return domain.LayoutNode{}, false
	}
}

func cleanAbsolutePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidWorkspacePath, err)
	}
	return filepath.Clean(absolute), nil
}

func isOwnedDocumentPath(root, path string) bool {
	if root == "" || !filepath.IsAbs(path) || strings.TrimSpace(path) == "" {
		return false
	}
	clean := filepath.Clean(path)
	relative, err := filepath.Rel(root, clean)
	return err == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func stateForPersistence(state domain.State) (domain.State, error) {
	plan, err := makeRestorePlan(state)
	if err != nil {
		return domain.State{}, err
	}
	if len(plan.Documents) != documentCount(state) {
		return domain.State{}, ErrDocumentOutsideVault
	}
	return plan.State, nil
}

func validateOwnedDocuments(state domain.State) error {
	for _, tab := range state.Tabs {
		if err := validateOwnedTab(state.VaultPath, tab); err != nil {
			return err
		}
	}
	return nil
}

func validateOwnedTab(root string, tab domain.Tab) error {
	return validateOwnedLayout(root, tab.Layout)
}

func validateOwnedLayout(root string, node domain.LayoutNode) error {
	if node.Pane != nil {
		return validateOwnedDocument(root, node.Pane.Document.Path)
	}
	if err := validateOwnedLayout(root, node.Split.First); err != nil {
		return err
	}
	return validateOwnedLayout(root, node.Split.Next)
}

func validateOwnedDocument(root, path string) error {
	if root == "" {
		return nil
	}
	cleanRoot, err := cleanAbsolutePath(root)
	if err != nil {
		return err
	}
	if !isOwnedDocumentPath(cleanRoot, path) {
		return ErrDocumentOutsideVault
	}
	return nil
}

func documentCount(state domain.State) int {
	count := 0
	for _, tab := range state.Tabs {
		count += len(tab.Panes())
	}
	return count
}
