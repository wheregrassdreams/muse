package table

import "testing"

func TestPlanShortcutBuildsRowsAndPlacesCaretInFirstBodyCell(t *testing.T) {
	plan, ok := PlanShortcut("| name | value |", "> ")
	if !ok {
		t.Fatal("PlanShortcut rejected a valid single pipe row")
	}
	want := "\n> | --- | --- |\n> |   |   |"
	if plan.Insert != want {
		t.Fatalf("insert = %q, want %q", plan.Insert, want)
	}
	if plan.Caret != len(plan.Insert)-len("|   |   |")+2 {
		t.Fatalf("caret = %d, want first body cell offset", plan.Caret)
	}
}

func TestPlanShortcutKeepsEscapedPipesInsideCells(t *testing.T) {
	plan, ok := PlanShortcut("| a \\| b | c |", "")
	if !ok {
		t.Fatal("PlanShortcut rejected a row with an escaped pipe")
	}
	if got, want := plan.Insert, "\n| --- | --- |\n|   |   |"; got != want {
		t.Fatalf("insert = %q, want %q", got, want)
	}
}

func TestPlanShortcutRejectsMultilineOrNonPipeRows(t *testing.T) {
	for _, row := range []string{"", "abc", "| |\n| x |", "||"} {
		if _, ok := PlanShortcut(row, ""); ok {
			t.Errorf("PlanShortcut(%q) succeeded", row)
		}
	}
}
