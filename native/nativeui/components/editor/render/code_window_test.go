package render

import "testing"

func TestCodeWindowKeepsWholeBlockAtAndBelowBudget(t *testing.T) {
	for _, size := range []int{CodeBlockBudgetBytes - 1, CodeBlockBudgetBytes} {
		window := CodeWindow(100, 100+size, -1, CodeBlockBudgetBytes)
		got, ok := window.RangeAt(0)
		if window.Compact() || window.RangeCount() != 1 || !ok || got != (CodeRange{Start: 100, End: 100 + size}) {
			t.Fatalf("CodeWindow(size=%d) = %+v", size, window)
		}
	}
}

func TestCodeWindowCentersOnCaretAndClampsAtEdges(t *testing.T) {
	const start, end, budget = 50, 50 + CodeBlockBudgetBytes*3, CodeBlockBudgetBytes
	cases := []struct {
		name  string
		caret int
		want  CodeRange
	}{
		{name: "near start", caret: start, want: CodeRange{Start: start, End: start + budget}},
		{name: "middle", caret: start + budget, want: CodeRange{Start: start + budget/2, End: start + budget/2 + budget}},
		{name: "at end", caret: end, want: CodeRange{Start: end - budget, End: end}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			window := CodeWindow(start, end, tc.caret, budget)
			got, ok := window.RangeAt(0)
			if !window.Compact() || window.RangeCount() != 1 || !ok || got != tc.want {
				t.Fatalf("CodeWindow(caret=%d) = %+v; want %+v", tc.caret, window, tc.want)
			}
		})
	}
}

func TestCodeWindowWithoutCaretKeepsBothEnds(t *testing.T) {
	const start, end, budget = 25, 25 + CodeBlockBudgetBytes*2, CodeBlockBudgetBytes
	window := CodeWindow(start, end, -1, budget)
	wantFirst := CodeRange{Start: start, End: start + budget/2}
	wantSecond := CodeRange{Start: end - budget/2, End: end}
	first, firstOK := window.RangeAt(0)
	second, secondOK := window.RangeAt(1)
	if !window.Compact() || window.RangeCount() != 2 || !firstOK || !secondOK || first != wantFirst || second != wantSecond {
		t.Fatalf("CodeWindow(no caret) = %+v; want [%+v %+v]", window, wantFirst, wantSecond)
	}
}

func TestCodeLineInRangesHandlesContentAndEmptyLines(t *testing.T) {
	window := CodeWindow(20, 30, -1, CodeBlockBudgetBytes)
	cases := []struct {
		line CodeLine
		want bool
	}{
		{CodeLine{Start: 19, End: 21, ByteLength: 2}, true},
		{CodeLine{Start: 30, End: 32, ByteLength: 2}, false},
		{CodeLine{Start: 24, End: 24, ByteLength: 0}, true},
		{CodeLine{Start: 30, End: 30, ByteLength: 0}, false},
	}
	for _, tc := range cases {
		if got := CodeLineInRanges(tc.line, window); got != tc.want {
			t.Errorf("CodeLineInRanges(%+v) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestCodeLineInRangesSkipsOmittedMiddle(t *testing.T) {
	window := CodeWindow(0, 100, -1, 20)
	cases := []struct {
		line CodeLine
		want bool
	}{
		{CodeLine{Start: 5, End: 8, ByteLength: 3}, true},
		{CodeLine{Start: 20, End: 25, ByteLength: 5}, false},
		{CodeLine{Start: 95, End: 100, ByteLength: 5}, true},
	}
	for _, tc := range cases {
		if got := CodeLineInRanges(tc.line, window); got != tc.want {
			t.Errorf("CodeLineInRanges(%+v) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestCodeOmittedLabel(t *testing.T) {
	if got, want := CodeOmittedLabel(1234), "… 1234 source bytes omitted …"; got != want {
		t.Fatalf("CodeOmittedLabel() = %q, want %q", got, want)
	}
}
