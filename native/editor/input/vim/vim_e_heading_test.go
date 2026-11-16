package editor

import (
	"strings"
	"testing"
)

func TestVimEAdvancesPastCurrentWordAtRevealedHeadingEnd(t *testing.T) {
	const src = "#### 三级标题"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	caret := strings.Index(src, "级")
	h.caretAt(caret)
	if l := h.e.leafAt(caret); l == nil || l.view == nil || !strings.Contains(l.view.disp.text, "####") {
		t.Fatal("fixture did not reveal the heading source markers")
	}

	wordStart, wordEnd := h.e.wordRangeAt(caret)
	if src[wordStart:wordEnd] != "三级" {
		t.Fatalf("fixture no longer puts caret at the last character of `三级`: word=%q range=[%d,%d)", src[wordStart:wordEnd], wordStart, wordEnd)
	}

	sendVim(h, "e")
	if h.e.caret != strings.Index(src, "题") {
		t.Fatalf("e from last character of a heading word landed at %d (%q), want end of next word at %d", h.e.caret, src[h.e.caret:], strings.Index(src, "题"))
	}

	// At the end of the final word, `e` must stay put instead of falling back
	// to the previous grapheme when there is no next token.
	const tail = "#### 末尾"
	h2 := newHarness(t, tail)
	h2.e.SetKeymap("vim")
	last := strings.Index(tail, "尾")
	h2.caretAt(last)
	sendVim(h2, "e")
	if h2.e.caret != last {
		t.Fatalf("e at final word moved backward/away: got %d want %d", h2.e.caret, last)
	}
}

func TestVimDeAtCurrentWordEndKeepsInclusiveCurrentWordSemantics(t *testing.T) {
	const src = "#### 三级标题"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.caretAt(strings.Index(src, "级"))
	sendVim(h, "de")
	if got, want := h.e.Source(), "#### 三标题"; got != want {
		t.Fatalf("de at current word end changed source to %q, want %q", got, want)
	}
}
