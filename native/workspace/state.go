// Package workspace owns the portable editing-session snapshot. It records
// which documents are open and how their panes are arranged; document source,
// editor buffers, widgets, and rendered resources remain owned elsewhere.
package workspace

import (
	"errors"
	"fmt"
	"math"
)

type TabID string
type PaneID string

type Axis string

const (
	Horizontal Axis = "horizontal"
	Vertical   Axis = "vertical"
)

type Document struct {
	Path         string `json:"path"`
	CursorOffset int    `json:"cursorOffset,omitempty"`
}

type Pane struct {
	ID       PaneID   `json:"id"`
	Document Document `json:"document"`
}

// LayoutNode is a tagged union: a leaf has Pane set and a branch has Split
// set. Constructors and Validate enforce that exactly one variant is present.
type LayoutNode struct {
	Pane  *Pane  `json:"pane,omitempty"`
	Split *Split `json:"split,omitempty"`
}

type Split struct {
	Axis  Axis       `json:"axis"`
	Ratio float64    `json:"ratio"`
	First LayoutNode `json:"first"`
	Next  LayoutNode `json:"next"`
}

type Tab struct {
	ID           TabID      `json:"id"`
	Layout       LayoutNode `json:"layout"`
	ActivePaneID PaneID     `json:"activePaneId,omitempty"`
}

// State is the durable workspace snapshot. ActivePaneID is stored per tab so
// switching tabs restores that tab's focused pane independently.
type State struct {
	Version   int    `json:"version"`
	VaultPath string `json:"vaultPath,omitempty"`
	Tabs      []Tab  `json:"tabs,omitempty"`
	ActiveTab TabID  `json:"activeTab,omitempty"`
}

var ErrInvalidState = errors.New("workspace: invalid state")

func Leaf(pane Pane) LayoutNode { return LayoutNode{Pane: &pane} }

func Branch(axis Axis, ratio float64, first, next LayoutNode) LayoutNode {
	return LayoutNode{Split: &Split{Axis: axis, Ratio: ratio, First: first, Next: next}}
}

func (s State) ActiveDocument() (Document, bool) {
	for _, tab := range s.Tabs {
		if tab.ID == s.ActiveTab {
			if doc, ok := documentForPane(tab.Layout, tab.ActivePaneID); ok {
				return doc, true
			}
			return firstDocument(tab.Layout)
		}
	}
	return Document{}, false
}

func documentForPane(node LayoutNode, id PaneID) (Document, bool) {
	if node.Pane != nil {
		if node.Pane.ID == id {
			return node.Pane.Document, true
		}
		return Document{}, false
	}
	if doc, ok := documentForPane(node.Split.First, id); ok {
		return doc, true
	}
	return documentForPane(node.Split.Next, id)
}

func firstDocument(node LayoutNode) (Document, bool) {
	if node.Pane != nil {
		return node.Pane.Document, true
	}
	return firstDocument(node.Split.First)
}

func (s State) Validate() error {
	if s.Version != 1 {
		return fmt.Errorf("%w: unsupported version %d", ErrInvalidState, s.Version)
	}
	if len(s.Tabs) > 32 {
		return fmt.Errorf("%w: too many tabs", ErrInvalidState)
	}
	if len(s.Tabs) == 0 {
		if s.ActiveTab != "" {
			return fmt.Errorf("%w: active tab is absent", ErrInvalidState)
		}
		return nil
	}
	tabs := make(map[TabID]struct{}, len(s.Tabs))
	activeFound := false
	for _, tab := range s.Tabs {
		if tab.ID == "" {
			return fmt.Errorf("%w: tab id is empty", ErrInvalidState)
		}
		if _, exists := tabs[tab.ID]; exists {
			return fmt.Errorf("%w: duplicate tab id %q", ErrInvalidState, tab.ID)
		}
		tabs[tab.ID] = struct{}{}
		if tab.ID == s.ActiveTab {
			activeFound = true
		}
		panes := make(map[PaneID]struct{})
		count := 0
		if err := validateNode(tab.Layout, 0, &count, panes); err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("%w: tab %q has no panes", ErrInvalidState, tab.ID)
		}
		if tab.ActivePaneID != "" {
			if _, exists := panes[tab.ActivePaneID]; !exists {
				return fmt.Errorf("%w: active pane %q is absent from tab %q", ErrInvalidState, tab.ActivePaneID, tab.ID)
			}
		}
	}
	if !activeFound {
		return fmt.Errorf("%w: active tab %q is absent", ErrInvalidState, s.ActiveTab)
	}
	return nil
}

func validateNode(node LayoutNode, depth int, count *int, panes map[PaneID]struct{}) error {
	if depth > 16 {
		return fmt.Errorf("%w: layout is too deep", ErrInvalidState)
	}
	if (node.Pane == nil) == (node.Split == nil) {
		return fmt.Errorf("%w: layout node must contain exactly one variant", ErrInvalidState)
	}
	if pane := node.Pane; pane != nil {
		if pane.ID == "" || pane.Document.Path == "" || pane.Document.CursorOffset < 0 {
			return fmt.Errorf("%w: pane id, document path, or cursor offset is invalid", ErrInvalidState)
		}
		if _, exists := panes[pane.ID]; exists {
			return fmt.Errorf("%w: duplicate pane id %q", ErrInvalidState, pane.ID)
		}
		panes[pane.ID] = struct{}{}
		*count++
		if *count > 64 {
			return fmt.Errorf("%w: too many panes in a tab", ErrInvalidState)
		}
		return nil
	}
	split := node.Split
	if (split.Axis != Horizontal && split.Axis != Vertical) || math.IsNaN(split.Ratio) || split.Ratio <= 0 || split.Ratio >= 1 {
		return fmt.Errorf("%w: split axis or ratio is invalid", ErrInvalidState)
	}
	if err := validateNode(split.First, depth+1, count, panes); err != nil {
		return err
	}
	return validateNode(split.Next, depth+1, count, panes)
}

// Repository persists a portable workspace snapshot.
type Repository interface {
	Load() (State, error)
	Save(State) error
}
