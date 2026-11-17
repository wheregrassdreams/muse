package paragraphui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFoldAdapterTargetsTitleAndHoveredChevron(t *testing.T) {
	view := FoldHeadingView{
		SourceStart: 12,
		Foldable:    true,
		Chevron:     ui.Rect{X: 8, Y: 10, W: 10, H: 10},
		Lines:       []HeadingLine{{Rect: ui.Rect{X: 20, Y: 10, W: 100, H: 20}, First: true}},
	}
	frame := FoldFrame{Headings: []FoldHeadingView{view}, HoveredStart: 12}
	if got, ok := HeadingAt(frame, 50, 18); !ok || got != (FoldAction{Kind: FoldHeadingAction, SourceStart: 12}) {
		t.Fatalf("title hit = %#v, %v", got, ok)
	}
	if got, ok := ChevronAt(frame, 12, 15); !ok || got != (FoldAction{Kind: ToggleFoldAction, SourceStart: 12}) {
		t.Fatalf("hovered chevron hit = %#v, %v", got, ok)
	}
	frame.HoveredStart = -1
	if got, ok := ChevronAt(frame, 12, 15); ok {
		t.Fatalf("chevron hit without heading hover = %#v", got)
	}
}

func TestTypedBreakProjectionUsesDetachedEditorFacts(t *testing.T) {
	input := BreakProjectionInput{Start: 4, End: 5, SoftBreak: true, CollapseSoft: true}
	if ProjectBreak(input).Newline() {
		t.Fatal("unmarked soft break should collapse")
	}
	input.EditedLine = true
	if !ProjectBreak(input).Newline() {
		t.Fatal("explicitly edited soft break must keep its line")
	}
}
