package editor

import (
	"testing"

	inlineblock "github.com/wheregrassdreams/muse-editor/markdown/inline"
)

func TestWikiLinkShowsItsLabel(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"见 [[项目笔记]] 吧\n", "见 «a:项目笔记» 吧"},
		{"[[项目笔记|别名]]\n", "«a:别名»"},
		// Half-typed and empty links stay as written.
		{"半截 [[笔记\n", "半截 [[笔记"},
		{"[[]] 空\n", "[[]] 空"},
		// An escaped opener is text.
		{`\[[笔记]]`, `[[笔记]]`},
	} {
		got := shown(t, tc.src)
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%q shows %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestWikiLinkRevealsAtTheCaret(t *testing.T) {
	h := newHarness(t, "见 [[项目笔记]] 吧")
	h.caretAt(0)
	if h.shows("[[") {
		t.Fatal("the brackets show away from the caret")
	}
	// Offsets are UTF-8 byte positions; 9 is the boundary after 项.
	h.caretAt(9)
	if h.e.Caret() != 9 {
		t.Fatalf("caret offset = %d, want a valid in-link rune boundary", h.e.Caret())
	}
	if !h.shows("[[项目笔记]]") {
		t.Fatalf("the link is hidden at the caret: %q", h.e.leaves[0].view.disp.text)
	}
}

func TestWikiLinkAliasRevealsItsTarget(t *testing.T) {
	h := newHarness(t, "[[项目笔记|别名]] x")
	h.caretAt(len(h.e.Source()) - 1)
	if h.shows("项目笔记") {
		t.Fatal("the target shows away from the caret")
	}
	h.caretAt(5)
	if h.e.Caret() != 5 {
		t.Fatalf("caret offset = %d, want a valid in-link rune boundary", h.e.Caret())
	}
	if !h.shows("[[项目笔记|") {
		t.Fatalf("the target is hidden at the caret: %q", h.e.leaves[0].view.disp.text)
	}
}

func TestWikiLinksPairTheBrackets(t *testing.T) {
	src := "a [[one]] b [[two|2]] c [[three"
	got := inlineblock.ParseWikiLinks(src, 0, len(src))
	if len(got) != 2 {
		t.Fatalf("found %d links", len(got))
	}
	if got[0].Target != "one" || got[1].Target != "two" {
		t.Fatalf("targets %q %q", got[0].Target, got[1].Target)
	}
	if l := src[got[1].LabelFrom:got[1].LabelTo]; l != "2" {
		t.Fatalf("label %q", l)
	}
	if _, ok := inlineblock.WikiLinkAt(src, 0, len(src), 5); !ok {
		t.Fatal("a click inside the first link finds none")
	}
	if _, ok := inlineblock.WikiLinkAt(src, 0, len(src), 0); ok {
		t.Fatal("a click outside every link opens one")
	}
}
