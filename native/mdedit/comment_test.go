package mdedit

import (
	"strings"
	"testing"
)

func TestCommentsDrawNothing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"前 %%注释%% 后\n", "前  后"},
		{"%%整段注释%%\n", ""},
		{"a %%b%% c %%d%% e\n", "a  c  e"},
		// A marker that never closes stays as text: an editor must not hide
		// the rest of a note because a %% was half typed.
		{"未闭合 %% 保留\n", "未闭合 %% 保留"},
	} {
		got := shown(t, tc.src)
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%q shows %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestBlockCommentHidesItsLines(t *testing.T) {
	got := shown(t, "%%\n块注释\n%%\n")
	if len(got) != 1 {
		t.Fatalf("shows %q", got)
	}
	if strings.Contains(got[0], "块注释") || strings.Contains(got[0], "%") {
		t.Fatalf("the comment shows %q", got[0])
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
