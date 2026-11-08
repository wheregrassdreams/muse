package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCommentsDrawNothing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"前 %%注释%% 后\n", "前  后"},
		// A comment-only line leaves no leaf at all: the parser reads a blank
		// line there.
		{"%%整段注释%%\n", ""},
		{"a %%b%% c %%d%% e\n", "a  c  e"},
		// A marker that never closes stays as text: an editor must not hide
		// the rest of a note because a %% was half typed.
		{"未闭合 %% 保留\n", "未闭合 %% 保留"},
	} {
		got := shown(t, tc.src)
		if tc.want == "" {
			if len(got) != 0 {
				t.Errorf("%q shows %q, want no text at all", tc.src, got)
			}
			continue
		}
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%q shows %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestBlockCommentCollapses(t *testing.T) {
	// A block comment takes its lines AND the break after it with it, so it
	// leaves no blank line where it was.
	got := shown(t, "%%\n块注释\n%%\n后文\n")
	if len(got) != 1 || got[0] != "后文" {
		t.Fatalf("a block comment shows %q", got)
	}
	// An inline comment at the end of a line keeps that line's break.
	if got := shown(t, "前 %%注释%%\n后\n"); len(got) != 1 || !strings.Contains(got[0], "\n") || !strings.HasPrefix(got[0], "前") {
		t.Fatalf("an inline comment shows %q", got)
	}
}

func TestCommentRevealsAtTheCaret(t *testing.T) {
	h := newHarness(t, "前 %%注释%% 后")
	h.caretAt(0)
	if h.shows("%%") {
		t.Fatal("the comment shows away from the caret")
	}
	h.caretAt(6)
	if !h.shows("%%注释%%") {
		t.Fatalf("the comment is hidden at the caret: %q", h.e.leaves[0].view.disp.text)
	}
}

func TestCommentsSkipCodeSpans(t *testing.T) {
	// %% inside inline code is the code's, not a comment's.
	got := shown(t, "`%%x%%` 后\n")
	if len(got) != 1 {
		t.Fatalf("shows %q", got)
	}
	if !strings.Contains(got[0], "%%x%%") {
		t.Fatalf("the code span shows %q", got[0])
	}
}

func TestCommentContentNeverBecomesMarkup(t *testing.T) {
	// The parser never reads the comment, so its bold and its link are just
	// bytes — nothing escapes, nothing renders.
	if got := shown(t, "%%注释里有 **加粗** 和 [[链接]]%%\n"); len(got) != 0 {
		t.Fatalf("a comment's markdown shows: %q", got)
	}
	// A blank line inside a comment does not split it into two notes either.
	got := shown(t, "%%\n注释\n\n更多\n%%\n后文\n")
	if len(got) != 1 || got[0] != "后文" {
		t.Fatalf("a comment with a blank line shows %q", got)
	}
}

func TestEditingInsideACommentKeepsItHidden(t *testing.T) {
	h := newHarness(t, "%%注释%% 后")
	h.caretAt(0)
	h.typ("x") // "x%%注释%% 后"
	h.caretAt(len(h.e.Source()))
	if h.shows("注释") {
		t.Fatalf("the comment shows after an edit: %q", h.e.leaves[0].view.disp.text)
	}
	if !strings.HasPrefix(h.e.Source(), "x%%") {
		t.Fatalf("the edit is lost: %q", h.e.Source())
	}
}

func TestEnterInsideACommentKeepsItWhole(t *testing.T) {
	h := newHarness(t, "%%\n注释\n%%\n后文")
	h.caretAt(2)
	h.key(0, ui.KeyEnter)
	// The blank line the Enter added is inside the comment: the comment still
	// pairs, still hides, and the text after it is untouched.
	if got := h.e.Source(); got != "%%\n\n注释\n%%\n后文" {
		t.Fatalf("source %q", got)
	}
	var out []string
	for _, u := range h.e.units() {
		if u.isText() && u.view != nil {
			out = append(out, u.view.disp.text)
		}
	}
	if len(out) != 1 || out[0] != "后文" {
		t.Fatalf("shows %q", out)
	}
}
