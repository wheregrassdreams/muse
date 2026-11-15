package inline

import "testing"

func TestMathSessionContentBoundsForSupportedDelimiters(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"$x^2$", "x^2"},
		{"$$x+y$$", "x+y"},
		{`\(x+y\)`, "x+y"},
		{`\[x+y\]`, "x+y"},
		{"$x\\$y$", "x\\$y"},
	} {
		var session MathSession
		session.Begin(0, len(tc.source))
		bounds, ok := session.ContentBounds(tc.source)
		if !ok || tc.source[bounds.Start:bounds.End] != tc.want {
			t.Errorf("ContentBounds(%q) = %+v, %v", tc.source, bounds, ok)
		}
	}
}

func TestMathSessionAdjustsOnlyEditsInsideItsRange(t *testing.T) {
	var session MathSession
	session.Begin(5, 10)
	session.AdjustAfterEdit(7, 8, 3)
	if got, _ := session.Range(); got != (Range{Start: 5, End: 12}) {
		t.Fatalf("range after internal edit = %+v", got)
	}
	session.AdjustAfterEdit(2, 3, 1)
	if session.Active() {
		t.Fatal("external edit retained the session")
	}
}

func TestMathSessionProjectionAndInvalidation(t *testing.T) {
	var session MathSession
	session.Begin(4, 9)
	if _, ok := session.Project(6, 6, 0, 12); !ok {
		t.Fatal("caret inside formula did not project it")
	}
	if _, ok := session.Project(1, 6, 0, 12); ok {
		t.Fatal("speculative view projected a formula outside active caret")
	}
	session.SetPreviewHidden(true)
	if !session.PreviewHidden() {
		t.Fatal("preview hidden state was not retained")
	}
	if session.Validate(12, 10) || session.Active() || session.PreviewHidden() {
		t.Fatal("invalid caret did not clear the complete session")
	}
}
