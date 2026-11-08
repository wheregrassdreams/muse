package mdedit

import "testing"

func TestTagsAreTagged(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"见 #项目 吧\n", "见 «a:#项目» 吧"},
		{"#项目/子 嵌套\n", "«a:#项目/子» 嵌套"},
		{"行首 #tag 和 #b\n", "行首 «a:#tag» 和 «a:#b»"},
		// Not tags: inside a word, a lone #, or one that starts with a
		// separator.
		{"a#b 不是\n", "a#b 不是"},
		{"x # 空的\n", "x # 空的"},
		{"#- x\n", "#- x"},
	} {
		got := shown(t, tc.src)
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%q shows %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestTagStopsAtPunctuation(t *testing.T) {
	got := shown(t, "看 #tag, 这个\n")
	if len(got) != 1 || got[0] != "看 «a:#tag», 这个" {
		t.Fatalf("shows %q", got)
	}
}

func TestEscapedHashIsText(t *testing.T) {
	got := shown(t, `\#tag`+"\n")
	if len(got) != 1 || got[0] != "#tag" {
		t.Fatalf("shows %q", got)
	}
}
