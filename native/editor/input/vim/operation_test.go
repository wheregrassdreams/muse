package vim

import (
	"testing"

	"muse/editor/blocks/paragraph"
	"muse/editor/input"
)

func TestOperationForIntent(t *testing.T) {
	intent := Intent{Kind: IntentFindCharacter, Character: "界", Direction: -1, Till: true, Count: 3}
	got, ok := OperationForIntent(intent)
	want := input.Operation{Kind: input.OperationFindCharacter, Character: "界", Direction: -1, Till: true, Count: 3}
	if !ok || got != want {
		t.Fatalf("OperationForIntent() = (%+v, %v), want (%+v, true)", got, ok, want)
	}
	if _, ok := OperationForIntent(Intent{Kind: IntentMotion, Motion: MotionLeft}); ok {
		t.Fatal("geometry-dependent motion was translated into a generic operation")
	}
}

func TestOperationForCommand(t *testing.T) {
	got, ok := OperationForCommand(Command{Kind: CommandFold, Fold: FoldOpenRecursive})
	want := input.Operation{Kind: input.OperationFoldAtCaret, Fold: paragraph.FoldOpenRecursive}
	if !ok || got != want {
		t.Fatalf("OperationForCommand() = (%+v, %v), want (%+v, true)", got, ok, want)
	}
	if _, ok := OperationForCommand(Command{Kind: CommandVertical, Direction: 1}); ok {
		t.Fatal("geometry-dependent vertical command was translated into an operation")
	}
}

