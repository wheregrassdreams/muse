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
