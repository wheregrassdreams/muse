package workspace

import (
	"math"
	"reflect"
	"testing"

	domain "muse/workspace"
)

func TestSetSplitRatioRejectsInvalidValuesWithoutChangingState(t *testing.T) {
	session, err := NewSession(nil, splitWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	before := session.Snapshot()

	for _, ratio := range []float64{0, 1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := session.SetSplitRatio("main", "root", ratio); err == nil {
			t.Fatalf("SetSplitRatio(%v) unexpectedly succeeded", ratio)
		}
		if after := session.Snapshot(); !reflect.DeepEqual(after, before) {
			t.Fatalf("invalid ratio %v changed state: got %+v, want %+v", ratio, after, before)
		}
	}
}

func TestUpdateDocumentRejectsEmptyPathWithoutChangingState(t *testing.T) {
	session, err := NewSession(nil, splitWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	before := session.Snapshot()
	if err := session.UpdateDocument("main", "left", domain.Document{Path: "  ", CursorOffset: 12}); err == nil {
		t.Fatal("UpdateDocument accepted an empty path")
	}
	if after := session.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("invalid document changed state: got %+v, want %+v", after, before)
	}
}

func TestTabsActivateMoveAndCloseRestoreRecentTab(t *testing.T) {
	session, err := NewSession(nil, splitWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	second := domain.Tab{
		ID: "second", ActivePaneID: "second-pane",
		Layout: domain.Leaf(domain.Pane{ID: "second-pane", Document: domain.Document{Path: "/second.md"}}),
	}
	if err := session.CreateTab(second, true); err != nil {
		t.Fatal(err)
	}
	if got := session.Snapshot().ActiveTab; got != "second" {
		t.Fatalf("CreateTab active tab = %q, want second", got)
	}
	if _, err := session.ActivateTab("main"); err != nil {
		t.Fatal(err)
	}
	if err := session.MoveTab("second", 0); err != nil {
		t.Fatal(err)
	}
	state := session.Snapshot()
	if state.Tabs[0].ID != "second" || state.ActiveTab != "main" {
		t.Fatalf("MoveTab result = order [%q, %q], active %q", state.Tabs[0].ID, state.Tabs[1].ID, state.ActiveTab)
	}
	active, err := session.CloseTab("main")
	if err != nil {
		t.Fatal(err)
	}
	if active != "second" || session.Snapshot().ActiveTab != "second" {
		t.Fatalf("CloseTab selected %q, snapshot active %q; want second", active, session.Snapshot().ActiveTab)
	}
}

func TestClosePaneRestoresMostRecentlyFocusedSurvivor(t *testing.T) {
	session, err := NewSession(nil, splitWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Focus("main", "right"); err != nil {
		t.Fatal(err)
	}
	if err := session.Focus("main", "left"); err != nil {
		t.Fatal(err)
	}
	focused, err := session.ClosePane("main", "left")
	if err != nil {
		t.Fatal(err)
	}
	if focused != "right" {
		t.Fatalf("ClosePane focus = %q, want right", focused)
	}
	state := session.Snapshot()
	if state.Tabs[0].ActivePaneID != "right" || state.Tabs[0].Layout.Pane == nil || state.Tabs[0].Layout.Pane.ID != "right" {
		t.Fatalf("ClosePane did not collapse tree and preserve focus: %+v", state.Tabs[0])
	}
}

func splitWorkspace() domain.State {
	return domain.State{
		Version: 1, ActiveTab: "main",
		Tabs: []domain.Tab{{
			ID: "main", ActivePaneID: "left",
			Layout: domain.Branch(domain.Horizontal, 0.5,
				domain.Leaf(domain.Pane{ID: "left", Document: domain.Document{Path: "/left.md"}}),
				domain.Leaf(domain.Pane{ID: "right", Document: domain.Document{Path: "/right.md"}}),
			),
		}},
	}
}
