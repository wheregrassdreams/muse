package source

import (
	"strings"
	"testing"

	"muse/markdown/syntax"
)

func TestParserSourceDefersMaskedByteCopyUntilIncrementalEdit(t *testing.T) {
	src := "---\ntitle: test\n---\n\n**注意：**text\n"
	fm := FrontmatterEnd(src)
	p := New(syntax.IncrementalDefault, bracketMath)
	p.Set(src, fm, nil)
	if p.identity || p.mask != nil {
		t.Fatal("masked parser bytes were copied eagerly")
	}
	want := bracketMath(blankComments(maskFrontMatter(src, fm), p.comments))
	if got := p.MaskedSource(); got != want || p.doc.Source() != want {
		t.Fatalf("deferred mask = %q, parser source = %q, want %q", got, p.doc.Source(), want)
	}

	from := strings.Index(src, "text")
	next := strings.Replace(src, "text", "words", 1)
	if !p.Edit(next, from, from+len("text"), "words") {
		t.Fatal("a masked source could not materialize its edit buffer")
	}
	if p.mask == nil || p.doc.Source() != p.MaskedSource() || p.source != next {
		t.Fatal("the first edit did not materialize and advance the mask")
	}
	want = bracketMath(blankComments(maskFrontMatter(next, fm), p.comments))
	if got := p.MaskedSource(); got != want {
		t.Fatalf("edited mask = %q, want %q", got, want)
	}
}

func TestParserSourceAliasesAnUnchangedMaskAndEditsItIncrementally(t *testing.T) {
	p := New(syntax.IncrementalDefault, bracketMath)
	src := "first line\nplain text\n"
	p.Set(src, 0, nil)
	if !p.identity || p.mask != nil || p.MaskedSource() != src {
		t.Fatal("unchanged parser source was copied into a separate mask")
	}

	next := "first line\nplain words\n"
	if !p.Edit(next, 17, 21, "words") {
		t.Fatal("plain edit could not advance the aliased parser source")
	}
	if !p.identity || p.mask != nil || p.source != next || p.doc.Source() != next {
		t.Fatalf("identity mask diverged after edit: source=%q parser=%q identity=%v", p.source, p.doc.Source(), p.identity)
	}
}

func TestParserSourceAliasedMaskFallsBackWhenEditedLineNeedsMasking(t *testing.T) {
	p := New(syntax.IncrementalDefault, bracketMath)
	src := "first line\nplain\n"
	p.Set(src, 0, nil)
	next := "first line\n\\[1\\] 文本\n"
	if p.Edit(next, 11, 16, `\[1\] 文本`) {
		t.Fatal("edit that changes parser-facing text stayed on the identity-mask path")
	}
	if p.source != src || p.doc.Source() != src || !p.identity {
		t.Fatalf("failed edit changed parser state: source=%q parser=%q identity=%v", p.source, p.doc.Source(), p.identity)
	}
}
