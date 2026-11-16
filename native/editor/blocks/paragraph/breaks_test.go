package paragraph

import "testing"

func TestShouldTrackBlankInput(t *testing.T) {
	base := BlankInput{Enabled: true, CollapseSoftBreaks: true, CollapsedSelection: true, BlankSourceLine: true, Text: "hello"}
	if !ShouldProcessInput(base) {
		t.Fatal("eligible input was not processed")
	}
	if !ShouldTrackBlankInput(base) {
		t.Fatal("eligible paragraph input was not tracked")
	}
	phantom := base
	phantom.BlankSourceLine = false
	phantom.PhantomParagraph = true
	if !ShouldTrackBlankInput(phantom) {
		t.Fatal("eligible phantom paragraph input was not tracked")
	}
	for name, change := range map[string]func(*BlankInput){
		"disabled":         func(in *BlankInput) { in.Enabled = false },
		"source newlines":  func(in *BlankInput) { in.CollapseSoftBreaks = false },
		"range selection":  func(in *BlankInput) { in.CollapsedSelection = false },
		"nonblank line":    func(in *BlankInput) { in.BlankSourceLine = false },
		"no paragraph":     func(in *BlankInput) { in.BlankSourceLine = false; in.PhantomParagraph = false },
		"whitespace input": func(in *BlankInput) { in.Text = " \t\n" },
	} {
		t.Run(name, func(t *testing.T) {
			in := base
			change(&in)
			if ShouldTrackBlankInput(in) {
				t.Fatal("ineligible paragraph input was tracked")
			}
		})
	}
}

func TestPlanBlankInputReturnsViewLocalBreakAnchors(t *testing.T) {
	source := "before\n\nafter"
	plan := PlanBlankInput(BlankInput{
		Enabled: true, CollapseSoftBreaks: true, CollapsedSelection: true,
		BlankSourceLine: true, Source: source, Caret: len("before\n"), Text: "x",
	})
	if !plan.BeginHistory || len(plan.RememberBreaks) != 2 || plan.RememberBreaks[0] != len("before") || plan.RememberBreaks[1] != len("before\n") {
		t.Fatalf("PlanBlankInput() = %+v", plan)
	}
	phantom := PlanBlankInput(BlankInput{
		Enabled: true, CollapseSoftBreaks: true, CollapsedSelection: true,
		PhantomParagraph: true, Text: "x",
	})
	if !phantom.BeginHistory || len(phantom.RememberBreaks) != 0 {
		t.Fatalf("phantom PlanBlankInput() = %+v", phantom)
	}
}

func TestBreakProjection(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   BreakProjection
		want bool
	}{
		{"collapsed soft break", BreakProjection{SoftBreak: true, CollapseSoftBreaks: true}, false},
		{"ordinary hard break", BreakProjection{CollapseSoftBreaks: true}, true},
		{"preserved soft break", BreakProjection{SoftBreak: true, CollapseSoftBreaks: true, PreserveLine: true}, true},
		{"editing soft break", BreakProjection{SoftBreak: true, CollapseSoftBreaks: true, EditingLine: true}, true},
		{"structural soft break", BreakProjection{SoftBreak: true, CollapseSoftBreaks: true, StructuralBreak: true}, true},
		{"revealed source soft break", BreakProjection{SoftBreak: true, CollapseSoftBreaks: true, RevealSource: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Newline(); got != tc.want {
				t.Fatalf("Newline()=%v want %v", got, tc.want)
			}
		})
	}
}

func TestBreaksRememberAndDeduplicateSourceAnchors(t *testing.T) {
	source := "one\ntwo\n"
	var breaks Breaks
	breaks.Remember(source, 3)
	breaks.Remember(source, 3)
	breaks.Remember(source, 0)
	if breaks.Len() != 1 || !breaks.Contains(3, 4) || breaks.Contains(0, 3) {
		t.Fatalf("remembered breaks = %v", breaks.Snapshot())
	}
}

func TestBreaksMoveSurvivingAndInsertedOffsets(t *testing.T) {
	var breaks Breaks
	breaks.Restore([]int{2, 5, 9})
	breaks.ApplyEdit(4, 6, "x\ny", true)
	got := breaks.Snapshot()
	want := []int{2, 5, 10}
	if len(got) != len(want) {
		t.Fatalf("break offsets = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("break offsets = %v, want %v", got, want)
		}
	}
}

func TestBreaksRestoreSortsAndKeyUsesRelativeOffsets(t *testing.T) {
	var breaks Breaks
	breaks.Restore([]int{18, 11, 14})
	if got, want := breaks.Key(10, 19), string([]byte{1, 4, 8}); got != want {
		t.Fatalf("Key() bytes = %v, want %v", []byte(got), []byte(want))
	}
	if got := breaks.Snapshot(); got[0] != 11 || got[1] != 14 || got[2] != 18 {
		t.Fatalf("Restore() order = %v", got)
	}
}
