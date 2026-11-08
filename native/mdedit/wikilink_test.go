package mdedit

import "testing"

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
	// 项 starts at byte 6, so 8 is inside the label.
	h.caretAt(8)
	if !h.shows("[[项目笔记]]") {
		t.Fatalf("the link is hidden at the caret: %q", h.e.leaves[0].view.disp.text)
	}
}

func TestWikiLinkAliasRevealsItsTarget(t *testing.T) {
	h := newHarness(t, "[[项目笔记|别名]] x")
	h.caretAt(len(h.e.src) - 1)
	if h.shows("项目笔记") {
		t.Fatal("the target shows away from the caret")
	}
	h.caretAt(3)
	if !h.shows("[[项目笔记|") {
		t.Fatalf("the target is hidden at the caret: %q", h.e.leaves[0].view.disp.text)
	}
}

func TestWikiLinksPairTheBrackets(t *testing.T) {
	src := "a [[one]] b [[two|2]] c [[three"
	got := wikiLinks(src, 0, len(src))
	if len(got) != 2 {
		t.Fatalf("found %d links", len(got))
	}
	if got[0].target != "one" || got[1].target != "two" {
		t.Fatalf("targets %q %q", got[0].target, got[1].target)
	}
	if l := src[got[1].labelFrom:got[1].labelTo]; l != "2" {
		t.Fatalf("label %q", l)
	}
	if _, ok := wikiAt(src, 0, len(src), 5); !ok {
		t.Fatal("a click inside the first link finds none")
	}
	if _, ok := wikiAt(src, 0, len(src), 0); ok {
		t.Fatal("a click outside every link opens one")
	}
}
