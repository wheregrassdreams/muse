package source

import (
	"strings"
	"testing"

	"muse/markdown/syntax"
)

func TestSourceDocumentPreservesOffsetsAcrossMaskedIncrementalEdit(t *testing.T) {
	source := "---\ntitle: test\n---\n\nvisible %% hidden %% text\n"
	frontmatterEnd := strings.Index(source[4:], "---\n") + 8
	doc := New(syntax.IncrementalDefault, nil)
	doc.Set(source, frontmatterEnd, nil)
	want := blankComments(maskFrontMatter(source, frontmatterEnd), CommentsFor(source))
	if got := doc.MaskedSource(); got != want {
		t.Fatalf("masked source=%q, want %q", got, want)
	}
	if got := doc.Parser().Source(); got != want {
		t.Fatalf("parser source=%q, want %q", got, want)
	}

	from := strings.Index(source, "visible")
	next := strings.Replace(source, "visible", "shown", 1)
	if !doc.Edit(next, from, from+len("visible"), "shown") {
		t.Fatal("safe text edit unexpectedly required a full reset")
	}
	if doc.Source() != next {
		t.Fatal("incremental source state did not advance")
	}
	want = blankComments(maskFrontMatter(next, frontmatterEnd), CommentsFor(next))
	if got := doc.MaskedSource(); got != want {
		t.Fatalf("edited masked source=%q, want %q", got, want)
	}
}

func TestSourceDocumentFallsBackAtCommentMarkerEdit(t *testing.T) {
	before := "one\nvisible text\n"
	doc := New(syntax.IncrementalDefault, nil)
	doc.Set(before, 0, nil)
	after := "one\n%%visible text\n"
	if doc.Edit(after, 4, 4, "%%") {
		t.Fatal("comment marker edit must require a full source sync")
	}
	if doc.Source() != before {
		t.Fatal("failed incremental edit mutated source state")
	}
}

func TestSourceDocumentAppliesSequentialCurrentCoordinateEdits(t *testing.T) {
	doc := New(syntax.IncrementalDefault, nil)
	doc.Set("alpha beta gamma\n", 0, nil)
	if !doc.ApplyEdit(6, 10, "BETA") {
		t.Fatal("first transaction edit was not applied incrementally")
	}
	if !doc.ApplyEdit(11, 12, "G") {
		t.Fatal("second transaction edit was not applied incrementally")
	}
	if got, want := doc.Source(), "alpha BETA Gamma\n"; got != want {
		t.Fatalf("source after sequential edits=%q, want %q", got, want)
	}
	if got, want := doc.MaskedSource(), "alpha BETA Gamma\n"; got != want {
		t.Fatalf("parser source after sequential edits=%q, want %q", got, want)
	}
}

func TestSourceDocumentApplyEditRejectsInvalidRangeWithoutMutation(t *testing.T) {
	doc := New(syntax.IncrementalDefault, nil)
	doc.Set("alpha beta\n", 0, nil)
	if doc.ApplyEdit(8, 4, "x") {
		t.Fatal("reversed edit range unexpectedly succeeded")
	}
	if got, want := doc.Source(), "alpha beta\n"; got != want {
		t.Fatalf("invalid edit changed source to %q, want %q", got, want)
	}
}
