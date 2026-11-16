package editor

import "testing"

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
	if frame.Source != e.Source() || frame.Revision != e.Revision() || frame.Document == nil || frame.Rendered == nil {
		t.Fatalf("frame does not reflect production editor state: source=%q rev=%d document=%v render=%v", frame.Source, frame.Revision, frame.Document != nil, frame.Rendered != nil)
	}
}

func TestSessionDispatchUsesProductionEditorIncrementalPath(t *testing.T) {
	e := New("first\n\nsecond\n", Options{})
	e.Select(len("first\n\n"), len("first\n\n"))
	result, err := e.Session.Dispatch(InputEvent{Kind: InputInsertText, Text: "typed"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || e.Source() != "first\n\ntypedsecond\n" {
		t.Fatalf("dispatch result=%+v source=%q", result, e.Source())
	}
	if e.p.src != e.Source() || e.p.doc.Source() != e.p.maskedSource() {
		t.Fatalf("production parser did not follow session edit: source=%q parser=%q mask=%q", e.p.src, e.p.doc.Source(), e.p.maskedSource())
	}
	if result, err := e.Session.Dispatch(InputEvent{Kind: InputUndo}); err != nil || !result.Changed || e.Source() != "first\n\nsecond\n" {
		t.Fatalf("session undo result=%+v source=%q err=%v", result, e.Source(), err)
	}
}

func TestSessionOperationDispatchUsesProductionEditor(t *testing.T) {
	e := New("one two", Options{})
	e.Select(0, 0)
	if handled := e.Session.ApplyOperation(EditorOperation{Kind: OperationMoveHorizontal, Direction: 1}); !handled {
		t.Fatal("production session did not dispatch editor operation")
	}
	if got := e.Session.Selection().Head; got != 1 {
		t.Fatalf("session selection after operation = %d, want 1", got)
	}
	if handled := NewSession("one two").ApplyOperation(EditorOperation{Kind: OperationMoveHorizontal, Direction: 1}); handled {
		t.Fatal("source-only session reported an editor operation as handled")
	}
}
