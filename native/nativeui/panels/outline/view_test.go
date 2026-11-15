package outline

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/ui/scrollbar"
)

func TestRenderClickEmitsSelectHeadingAction(t *testing.T) {
	var actions []Action
	scroll := ui.ScrollState{}
	indicator := scrollbar.Indicator{}
	model := ViewModel{
		Headings:      []Heading{{Level: 1, Text: "Overview", Offset: 12}, {Level: 2, Text: "Details", Offset: 48}},
		SelectedRange: SelectedRange{Start: 0, End: 0},
		Scroll:        ScrollPort{State: &scroll, Indicator: &indicator},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, model, func(action Action) { actions = append(actions, action) })
	}, 360, 180)
	tester.Frame()
	if err := tester.Click("Details"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0] != (Action{Kind: SelectHeading, Offset: 48}) {
		t.Fatalf("actions = %+v, want select-heading at byte offset 48", actions)
	}
}

func TestRenderReceivesSelectedRangeAndScrollPort(t *testing.T) {
	headings := make([]Heading, 24)
	for i := range headings {
		headings[i] = Heading{Level: i%3 + 1, Text: fmt.Sprintf("Heading %02d", i), Offset: i * 10}
	}
	scroll := ui.ScrollState{}
	indicator := scrollbar.Indicator{}
	model := ViewModel{Headings: headings, SelectedRange: SelectedRange{Start: 4, End: 6}, Scroll: ScrollPort{State: &scroll, Indicator: &indicator}}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, model, nil) }, 360, 180)
	tester.Frame()
	if !tester.HasText("Heading 04") || !tester.HasText("Heading 06") {
		t.Fatalf("selected rows missing: %q", tester.Texts())
	}
	tester.Scroll(120, 120, 0, 140)
	tester.Frame()
	if scroll.Y <= 0 {
		t.Fatalf("injected scroll state did not receive wheel position: %+v", scroll)
	}
}

func TestRenderEmptyOutline(t *testing.T) {
	tester := ui.NewTester(func(c *ui.Context) { Render(c, ViewModel{}, nil) }, 320, 160)
	tester.Frame()
	if !tester.HasText(locales.DocumentNoHeading()) {
		t.Fatalf("empty state missing: %q", tester.Texts())
	}
}

func TestSelectedRangeIsInclusiveAndRejectsInvalidBounds(t *testing.T) {
	r := SelectedRange{Start: 2, End: 4}
	if !r.Contains(2, 8) || !r.Contains(4, 8) || r.Contains(1, 8) || r.Contains(5, 8) {
		t.Fatal("range did not use inclusive row bounds")
	}
	if (SelectedRange{Start: 4, End: 2}).Contains(3, 8) {
		t.Fatal("reversed range selected a row")
	}
	if (SelectedRange{Start: 2, End: 9}).Contains(3, 8) {
		t.Fatal("out-of-bounds range selected a row")
	}
}
