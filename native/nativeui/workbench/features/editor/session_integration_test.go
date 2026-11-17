package editor

import (
	"testing"

	editormodel "muse/editor"
	"muse/editor/input"
)

func TestProductionEditorUsesSessionSourceSelectionAndHistory(t *testing.T) {
	e := New("old", Options{})
	if e.Session == nil {
		t.Fatal("production editor has no shared session")
	}
	e.Select(1, 2)
	if got := e.Session.Selection(); got.Anchor != 1 || got.Head != 2 {
		t.Fatalf("session selection = %+v, want [1,2)", got)
	}
	e.Replace(1, 2, "X")
	if got := e.Session.Source(); got != "oXd" {
		t.Fatalf("session source after editor edit = %q", got)
	}
	if !e.Undo() || e.Session.Source() != "old" {
		t.Fatalf("session source after editor undo = %q", e.Session.Source())
	}
	if !e.Redo() || e.Session.Source() != "oXd" {
		t.Fatalf("session source after editor redo = %q", e.Session.Source())
	}
	frame := e.Session.Frame()
	if frame.Source != e.Source() || frame.Revision != e.Revision() || frame.Selection != e.Session.Selection() {
		t.Fatalf("frame does not reflect production editor state: source=%q rev=%d selection=%+v", frame.Source, frame.Revision, frame.Selection)
	}
}

func TestSessionDispatchUsesProductionEditorIncrementalPath(t *testing.T) {
	e := New("first\n\nsecond\n", Options{})
	e.Select(len("first\n\n"), len("first\n\n"))
	result, err := e.Session.Dispatch(input.Event{Kind: input.InsertText, Text: "typed"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || e.Source() != "first\n\ntypedsecond\n" {
		t.Fatalf("dispatch result=%+v source=%q", result, e.Source())
	}
	if e.p.Source() != e.Source() || e.p.Parser().Source() != e.p.MaskedSource() {
		t.Fatalf("production parser did not follow session edit: source=%q parser=%q mask=%q", e.p.Source(), e.p.Parser().Source(), e.p.MaskedSource())
	}
	if result, err := e.Session.Dispatch(input.Event{Kind: input.Undo}); err != nil || !result.Changed || e.Source() != "first\n\nsecond\n" {
		t.Fatalf("session undo result=%+v source=%q err=%v", result, e.Source(), err)
	}
}

func TestSessionOperationDispatchUsesProductionEditor(t *testing.T) {
	e := New("one two", Options{})
	e.Select(0, 0)
	if handled := e.Session.ApplyOperation(input.Operation{Kind: input.OperationMoveHorizontal, Direction: 1}); !handled {
		t.Fatal("production session did not dispatch editor operation")
	}
	if got := e.Session.Selection().Head; got != 1 {
		t.Fatalf("session selection after operation = %d, want 1", got)
	}
	if handled := editormodel.NewSession("one two").ApplyOperation(input.Operation{Kind: input.OperationMoveHorizontal, Direction: 1}); handled {
		t.Fatal("source-only session reported an editor operation as handled")
	}
}
