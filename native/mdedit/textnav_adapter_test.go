package mdedit

import (
	"strings"
	"testing"
)

func TestWordIndexDisplayAdapterKeepsUTF16AndByteOffsetsDistinct(t *testing.T) {
	e := New("👩‍💻 e\u0301中文", Options{})
	display := "👩‍💻 e\u0301中文"
	w := e.wordsForDisplay(display)
	for _, u16 := range []int{0, 5, 6, 8, 10} {
		b := w.byteAtUTF16(u16, false)
		if got := w.utf16AtByte(b, false); got != u16 {
			t.Fatalf("UTF-16 %d mapped through byte %d back to %d", u16, b, got)
		}
	}
	if w.byteAtUTF16(5, false) != len("👩‍💻") {
		t.Fatalf("emoji UTF-16 end mapped to byte %d", w.byteAtUTF16(5, false))
	}
	at := strings.Index(e.Source(), "中文")
	a, z := e.wordRangeAt(at)
	if got := e.Source()[a:z]; got != "中文" {
		t.Fatalf("word range after astral/combining text = %q", got)
	}
}

func TestWordDisplayCacheReusesRecentParagraphsAndIsBounded(t *testing.T) {
	e := New("", Options{})
	first := e.wordsForDisplay("alpha")
	_ = e.wordsForDisplay("中文")
	if got := e.wordsForDisplay("alpha"); got != first {
		t.Fatal("recent display index was not reused")
	}
	for n := 0; n < 12; n++ {
		_ = e.wordsForDisplay(strings.Repeat("x", n+1))
	}
	if len(e.wordIndexCache) != 8 {
		t.Fatalf("display cache length = %d, want bounded length 8", len(e.wordIndexCache))
	}
	if e.wordIndexCacheCost > wordIndexCacheBudget {
		t.Fatalf("display cache cost = %d, over %d byte budget", e.wordIndexCacheCost, wordIndexCacheBudget)
	}
}

func TestZeroSourceLinkDecorationIsNotAWordTarget(t *testing.T) {
	const src = "before [label](https://example.com) after"
	e := New(src, Options{})
	e.layoutIfNeeded()
	u := e.units()[0]
	v := e.unitView(u, 0)
	iconAt := -1
	for _, p := range v.disp.pieces {
		if p.kind == pDeco && strings.Contains(p.text, "↗") {
			iconAt = p.s0 + u.lineStart
			break
		}
	}
	if iconAt < 0 {
		t.Fatal("external-link decoration not found")
	}
	a, z := e.wordRangeAt(iconAt)
	if a != z {
		t.Fatalf("zero-source icon selected source range [%d,%d): %q", a, z, src[a:z])
	}
	labelAt := strings.Index(src, "label")
	if got, want := e.step(labelAt, 1, true), strings.Index(src, "after"); got != want {
		t.Fatalf("word motion stopped on decoration: got %d want source word %d", got, want)
	}
}
