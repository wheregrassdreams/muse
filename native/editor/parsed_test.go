package mdedit

import (
	"strings"
	"testing"
)

func TestParserSourceDefersMaskedByteCopyUntilIncrementalEdit(t *testing.T) {
	src := "---\ntitle: test\n---\n\n**注意：**text\n"
	fm := frontMatterEnd(src)
	var p parserSource
	p.set(src, fm)
	if p.maskIsSource || p.mask != nil {
		t.Fatal("masked parser bytes were copied eagerly")
	}
	want := parserText(maskFrontMatter(src, fm))
	if got := p.maskedSource(); got != want || p.doc.Source() != want {
		t.Fatalf("deferred mask = %q, parser source = %q, want %q", got, p.doc.Source(), want)
	}

	from := strings.Index(src, "text")
	next := strings.Replace(src, "text", "words", 1)
	if !p.edit(next, &pendingEdit{from: from, to: from + len("text"), text: "words"}) {
		t.Fatal("a masked source could not materialize its edit buffer")
	}
	if p.mask == nil || p.doc.Source() != p.maskedSource() || p.src != next {
		t.Fatal("the first edit did not materialize and advance the mask")
	}
	want = parserText(maskFrontMatter(next, fm))
	if got := p.maskedSource(); got != want {
		t.Fatalf("edited mask = %q, want %q", got, want)
	}
}

func TestParserSourceAliasesAnUnchangedMaskAndEditsItIncrementally(t *testing.T) {
	var p parserSource
	src := "first line\nplain text\n"
	p.set(src, 0)
	if !p.maskIsSource || p.mask != nil || p.maskedSource() != src {
		t.Fatal("unchanged parser source was copied into a separate mask")
	}

	next := "first line\nplain words\n"
	edit := &pendingEdit{from: 17, to: 21, text: "words"}
	if !p.edit(next, edit) {
		t.Fatal("plain edit could not advance the aliased parser source")
	}
	if !p.maskIsSource || p.mask != nil || p.src != next || p.doc.Source() != next {
		t.Fatalf("identity mask diverged after edit: source=%q parser=%q identity=%v", p.src, p.doc.Source(), p.maskIsSource)
	}
}

func TestParserSourceAliasedMaskFallsBackWhenEditedLineNeedsMasking(t *testing.T) {
	var p parserSource
	src := "first line\nplain\n"
	p.set(src, 0)
	next := "first line\n\\[1\\] 文本\n"
	edit := &pendingEdit{from: 11, to: 16, text: `\[1\] 文本`}
	if p.edit(next, edit) {
		t.Fatal("edit that changes parser-facing text stayed on the identity-mask path")
	}
	if p.src != src || p.doc.Source() != src || !p.maskIsSource {
		t.Fatalf("failed edit changed parser state: source=%q parser=%q identity=%v", p.src, p.doc.Source(), p.maskIsSource)
	}
}
