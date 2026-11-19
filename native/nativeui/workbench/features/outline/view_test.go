package outline

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestRenderClickEmitsSelectHeadingAction(t *testing.T) {
	var actions []Action
	handle := NewHandle()
	handle.SyncActiveHeading([]Heading{{Level: 1, Text: "Overview", Offset: 12}}, 12)
	model := ViewModel{
		Headings: []Heading{{Level: 1, Text: "Overview", Offset: 12}, {Level: 2, Text: "Details", Offset: 48}},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		handle.Render(c, model, func(action Action) { actions = append(actions, action) })
	}, 360, 180)
	tester.Frame()
	if err := tester.Click("Details"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0] != (Action{Kind: SelectHeading, Offset: 48}) {
		t.Fatalf("actions = %+v, want select-heading at byte offset 48", actions)
	}
}

func TestStateOwnsSelectionAndScroll(t *testing.T) {
	headings := make([]Heading, 24)
	for i := range headings {
		headings[i] = Heading{Level: i%3 + 1, Text: fmt.Sprintf("Heading %02d", i), Offset: i * 10}
	}
	handle := NewHandle()
	handle.SyncActiveHeading(headings, 50)
	model := ViewModel{Headings: headings}
	tester := ui.NewTester(func(c *ui.Context) { handle.Render(c, model, nil) }, 360, 180)
	tester.Frame()
	if !tester.HasText("Heading 05") {
		t.Fatalf("active row missing: %q", tester.Texts())
	}
	tester.Scroll(120, 120, 0, 140)
	tester.Frame()
	if handle.state.scroll.Y <= 0 {
		t.Fatalf("panel-owned scroll state did not receive wheel position: %v", handle.state.scroll.Y)
	}
}

func TestRenderSnapshotSynchronizesCaretAndRoutesSelection(t *testing.T) {
	headings := []Heading{
		{Level: 1, Text: "Overview", Offset: 12},
		{Level: 2, Text: "Details", Offset: 48},
	}
	handle := NewHandle()
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		handle.RenderSnapshot(c, Snapshot{Headings: headings, Caret: 48}, func(action Action) {
			actions = append(actions, action)
		})
	}, 360, 180)
	tester.Frame()
	if handle.state.activeRow != 1 {
		t.Fatalf("active row = %d, want caret's heading row 1", handle.state.activeRow)
	}
	if err := tester.Click("Overview"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0] != (Action{Kind: SelectHeading, Offset: 12}) {
		t.Fatalf("actions = %+v, want typed selection at source offset 12", actions)
	}
}

func TestRenderEmptyOutline(t *testing.T) {
	handle := NewHandle()
	tester := ui.NewTester(func(c *ui.Context) { handle.Render(c, ViewModel{}, nil) }, 320, 160)
	tester.Frame()
	if !tester.HasText(locales.DocumentNoHeading()) {
		t.Fatalf("empty state missing: %q", tester.Texts())
	}
}
