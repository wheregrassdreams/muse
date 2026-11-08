package mdedit

import (
	"strings"
	"testing"
)

// testFullMask returns the masked source a full parse would read.
func testFullMask(src string) []byte {
	return []byte(parserText(maskFrontMatter(src, frontMatterEnd(src))))
}

// at returns the offset of sub in s.
func at(t *testing.T, s, sub string) int {
	t.Helper()
	i := strings.Index(s, sub)
	if i < 0 {
		t.Fatalf("%q does not hold %q", s, sub)
	}
	return i
}

// TestEditMaskMatchesFullMask checks that applying one edit to a mask leaves
// exactly the mask a full parse of the edited source would make. The
// replacement it reports must also be the one the parser is given.
func TestEditMaskMatchesFullMask(t *testing.T) {
	cases := []struct {
		name     string
		old      string
		from, to int
		text     string
		ok       bool
	}{
		{
			name: "insert beside CJK punctuation",
			old:  "前缀 **注意：**内容\n尾部\n",
			from: 0, // set below
			text: "新",
			ok:   true,
		},
		{
			name: "insert a newline",
			old:  "abc\ndef\nghi",
			from: 1,
			to:   1,
			text: "\n",
			ok:   true,
		},
		{
			name: "delete a newline",
			old:  "abc\ndef\nghi",
			from: 3,
			to:   4,
			ok:   true,
		},
		{
			name: "replace across lines",
			old:  "abc\ndef\nghi",
			from: 2,
			to:   6,
			text: "X\nY",
			ok:   true,
		},
		{
			name: "append after a last line with no newline",
			old:  "abc",
			from: 3,
			to:   3,
			text: "\nxyz",
			ok:   true,
		},
		{
			name: "append after a last newline",
			old:  "abc\n",
			from: 4,
			to:   4,
			text: "xyz",
			ok:   true,
		},
		{
			name: "a bracket becomes TeX",
			old:  `\[1\] 文本`,
			from: 0, // set below
			to:   0, // set below
			text: `\frac{a}{b}`,
			ok:   true,
		},
		{
			name: "edit after front matter",
			old:  "---\ntitle: x\n---\nbody\n",
			from: 0, // set below
			text: "B",
			ok:   true,
		},
		{
			name: "edit at the start may change front matter",
			old:  "abc",
			from: 0,
			text: "x",
			ok:   false,
		},
		{
			name: "edit inside front matter",
			old:  "---\ntitle: x\n---\nbody\n",
			from: 0, // set below
			text: "T",
			ok:   false,
		},
	}
	for i := range cases {
		c := &cases[i]
		c.from, c.to = editRange(t, c.old, c.name, c.from, c.to)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			newSrc := c.old[:c.from] + c.text + c.old[c.to:]
			oldMask := testFullMask(c.old)
			// editMask may rewrite its input in place, so keep a copy for
			// reconstructing what the parser was given.
			before := append([]byte(nil), oldMask...)
			// The comments of the new source: they stand where the edit and
			// the old ones leave them (the cases that reach a comment give
			// the whole source back).
			cmts := shiftCmts(commentRegions(c.old), c.from, c.to, len(c.text))
			next, mFrom, mTo, repl, ok := editMask(oldMask, newSrc, c.from, c.to, frontMatterEnd(c.old), cmts)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if want := testFullMask(newSrc); string(next) != string(want) {
				t.Fatalf("mask after edit = %q, want %q", next, want)
			}
			if len(next) != len(newSrc) {
				t.Fatalf("mask is %d bytes, source is %d", len(next), len(newSrc))
			}
			if got := string(before[:mFrom]) + repl + string(before[mTo:]); got != string(next) {
				t.Fatalf("parser edit %q + %q + %q = %q, want %q",
					before[:mFrom], repl, before[mTo:], got, next)
			}
		})
	}
}

// editRange fills the offsets of a case from the substrings its setup names.
func editRange(t *testing.T, s, name string, from, to int) (int, int) {
	t.Helper()
	switch name {
	case "insert beside CJK punctuation":
		i := at(t, s, "内容")
		return i, i
	case "a bracket becomes TeX":
		i := at(t, s, "1")
		return i, i + 1
	case "edit after front matter":
		i := at(t, s, "body")
		return i, i
	case "edit inside front matter":
		i := at(t, s, "title")
		return i, i
	}
	return from, to
}

// shiftCmts returns the comment regions of a source an edit left alone,
// moved by its length: what applyPending holds once it takes the edit.
func shiftCmts(cmts []cmt, from, to, newLen int) []cmt {
	d := newLen - (to - from)
	out := make([]cmt, len(cmts))
	for i, c := range cmts {
		if c.from >= to {
			c.from += d
			c.to += d
		}
		out[i] = c
	}
	return out
}
