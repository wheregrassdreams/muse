package workspace

import "testing"

func TestActiveDocumentUsesActiveTabAndPane(t *testing.T) {
	state := State{
		Version: 1, ActiveTab: "second",
		Tabs: []Tab{
			{ID: "first", ActivePaneID: "a", Layout: Leaf(Pane{ID: "a", Document: Document{Path: "/first.md"}})},
			{ID: "second", ActivePaneID: "b", Layout: Branch(Horizontal, .5,
				Leaf(Pane{ID: "a", Document: Document{Path: "/left.md"}}),
				Leaf(Pane{ID: "b", Document: Document{Path: "/right.md"}}),
			)},
		},
	}
	doc, ok := state.ActiveDocument()
	if !ok || doc.Path != "/right.md" {
		t.Fatalf("ActiveDocument() = %+v, %v", doc, ok)
	}
}

func TestValidateRejectsMalformedAndUnboundedLayouts(t *testing.T) {
	tests := map[string]State{
		"unsupported version": {Version: 2},
		"active tab missing":  {Version: 1, ActiveTab: "missing"},
		"duplicate pane id": {
			Version: 1, ActiveTab: "main",
			Tabs: []Tab{{ID: "main", Layout: Branch(Vertical, .5,
				Leaf(Pane{ID: "same", Document: Document{Path: "/one.md"}}),
				Leaf(Pane{ID: "same", Document: Document{Path: "/two.md"}}),
			)}},
		},
		"invalid split ratio": {
			Version: 1, ActiveTab: "main",
			Tabs: []Tab{{ID: "main", Layout: Branch(Horizontal, 1,
				Leaf(Pane{ID: "a", Document: Document{Path: "/one.md"}}),
				Leaf(Pane{ID: "b", Document: Document{Path: "/two.md"}}),
			)}},
		},
	}
	for name, state := range tests {
		t.Run(name, func(t *testing.T) {
			if err := state.Validate(); err == nil {
				t.Fatal("Validate() accepted malformed state")
			}
		})
	}
}
