package editor

import (
	"math"
	"strings"
	"testing"

	"github.com/wheregrassdreams/muse-editor/markdown/source"
	"muse/document/frontmatter"
)

func TestHiddenFrontmatterDoesNotAddAClosingDelimiterBlankLine(t *testing.T) {
	for _, blanks := range []int{0, 1, 2} {
		src := "---\ntitle: preserved\n---\n" + strings.Repeat("\n", blanks) + "# Body\n"
		h := newHarness(t, src)
		opts := h.e.Options()
		opts.HideFrontmatter = true
		h.e.SetOptions(opts)
		h.frame()
		var heading *leaf
		for _, l := range h.e.leaves {
			if l.kind == kHeading {
				heading = l
				break
			}
		}
		want := h.e.padTop + float32(blanks)*h.e.lk.size*h.e.lk.lh
		if heading == nil || math.Abs(float64(heading.y-want)) > .1 || h.e.Source() != src {
			t.Fatalf("%d body blanks: hidden metadata changed body start/source; heading=%+v wantY=%v", blanks, heading, want)
		}
	}
}

func TestHideFrontmatterKeepsSourceOffsetsAndBodyCaret(t *testing.T) {
	src := "---\ntitle: note\ntags: [one]\n---\n# Body\nText.\n"
	e := New(src, Options{})
	if e.p.FrontmatterEnd() == 0 || !hasFrontmatterLeaf(e.leaves) {
		t.Fatal("frontmatter should be visible by default")
	}

	e.SetOptions(Options{HideFrontmatter: true})
	fm := e.p.FrontmatterEnd()
	if e.Source() != src {
		t.Fatal("changing visibility changed the source")
	}
	if hasFrontmatterLeaf(e.leaves) {
		t.Fatal("hidden frontmatter still has a display leaf")
	}
	if e.Caret() < fm || lineStartAt(src, e.Caret()) < fm {
		t.Fatalf("caret = %d, want a body offset at or after frontmatter end %d", e.Caret(), fm)
	}

	caret := e.Caret()
	at := strings.Index(e.Source(), "title: note")
	e.Replace(at, at+len("title: note"), "title: longer note")
	delta := len("title: longer note") - len("title: note")
	if e.Caret() != caret+delta {
		t.Fatalf("caret after metadata edit = %d, want %d", e.Caret(), caret+delta)
	}
	if hasFrontmatterLeaf(e.leaves) {
		t.Fatal("metadata edit exposed the hidden frontmatter")
	}

	e.Undo()
	if e.Source() != src || e.Caret() != caret {
		t.Fatalf("undo source/caret = (%q, %d), want original source and caret %d", e.Source(), e.Caret(), caret)
	}
	if hasFrontmatterLeaf(e.leaves) {
		t.Fatal("undo exposed the hidden frontmatter")
	}

	e.SetOptions(Options{})
	if !hasFrontmatterLeaf(e.leaves) || e.Source() != src || e.Caret() != caret {
		t.Fatal("showing frontmatter did not preserve source and body caret")
	}
}

func TestHideFrontmatterLeavesCaretAtEmptyBody(t *testing.T) {
	src := "---\ntitle: note\n---\n"
	e := New(src, Options{HideFrontmatter: true})
	if hasFrontmatterLeaf(e.leaves) {
		t.Fatal("hidden frontmatter still has a display leaf")
	}
	if e.Caret() < e.p.FrontmatterEnd() {
		t.Fatalf("caret = %d, want an empty body offset at or after %d", e.Caret(), e.p.FrontmatterEnd())
	}
	if len(e.leaves) != 1 {
		t.Fatalf("empty body has %d leaves, want one editable line", len(e.leaves))
	}
	if !e.leaves[0].isLine() || e.leaves[0].start < e.p.FrontmatterEnd() || e.Caret() != e.leaves[0].start {
		t.Fatalf("empty body leaf/caret = (%+v, %d), want one editable line at or after %d", e.leaves[0], e.Caret(), e.p.FrontmatterEnd())
	}
}

func TestHideEmptyFrontmatterKeepsDefaultTwoRulesAndMapsBodyCaret(t *testing.T) {
	src := "---\n---\n# Body\n"
	plain := New(src, Options{})
	if source.FrontmatterEnd(src) != 0 || countRuleLeaves(plain.leaves) != 2 {
		t.Fatalf("default empty frontmatter changed: fm end = %d, rules = %d", source.FrontmatterEnd(src), countRuleLeaves(plain.leaves))
	}

	e := New(src, Options{})
	e.SetOptions(Options{HideFrontmatter: true})
	if len(e.leaves) == 0 || e.leaves[0].kind != kHeading || e.leaves[0].level != 1 {
		t.Fatalf("first visible leaf = %#v, want the body heading", e.leaves)
	}
	if e.Caret() < len("---\n---\n") || lineStartAt(src, e.Caret()) < len("---\n---\n") {
		t.Fatalf("caret = %d, want body offset after empty frontmatter", e.Caret())
	}
}

func TestHideEmptyFrontmatterWithEmptyBody(t *testing.T) {
	src := "---\n---\n"
	e := New(src, Options{})
	e.SetOptions(Options{HideFrontmatter: true})
	if len(e.leaves) != 1 || !e.leaves[0].isLine() {
		t.Fatalf("empty body leaves = %#v, want one editable line", e.leaves)
	}
	if e.Caret() != len(src) || e.leaves[0].start != len(src) {
		t.Fatalf("empty body caret/leaf = %d/%d, want source end %d", e.Caret(), e.leaves[0].start, len(src))
	}
}

func TestFrontmatterParseErrorKeepsSourceVisible(t *testing.T) {
	invalid := "---\ntitle: [unterminated\n---\n# Body\n"
	parsed := frontmatter.Parse(invalid)
	if parsed.Err == nil {
		t.Fatal("fixture should have invalid YAML")
	}
	e := New(invalid, Options{HideFrontmatter: parsed.Err == nil})
	if !hasFrontmatterLeaf(e.leaves) {
		t.Fatal("parse error should keep frontmatter source visible")
	}

	unclosed := "---\ntitle: note\n# Body\n"
	parsed = frontmatter.Parse(unclosed)
	if parsed.Err == nil {
		t.Fatal("fixture should have an unclosed frontmatter error")
	}
	e = New(unclosed, Options{HideFrontmatter: true})
	if e.Source() != unclosed || len(e.leaves) == 0 || e.leaves[0].lineStart != 0 {
		t.Fatal("unclosed frontmatter source should remain visible")
	}
}

func hasFrontmatterLeaf(leaves []*leaf) bool {
	for _, l := range leaves {
		if l.frontMatter {
			return true
		}
	}
	return false
}

func countRuleLeaves(leaves []*leaf) int {
	n := 0
	for _, l := range leaves {
		if l.kind == kRule {
			n++
		}
	}
	return n
}

func TestHiddenFrontmatterDoesNotLeaveVerticalGap(t *testing.T) {
	src := "---\ntitle: note\ntags: [one]\nstatus: draft\ndate: 2026-11-10\n---\n# Body\n"
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.HideFrontmatter = true
	h.e.SetOptions(opts)
	h.frame()
	if len(h.e.leaves) == 0 {
		t.Fatal("missing body")
	}
	if gap := h.e.leaves[0].y - h.e.padTop; gap > h.e.lk.size*2 {
		t.Fatalf("hidden YAML left %.1fpx gap", gap)
	}
}
