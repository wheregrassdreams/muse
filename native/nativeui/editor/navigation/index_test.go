package navigation

import (
	"strings"
	"testing"
)

func TestWordIndexMapsUTF16AndEngineByteOffsets(t *testing.T) {
	w := newWordIndex("👩‍💻 e\u0301中文")
	for _, u16 := range []int{0, 5, 6, 8, 10} {
		b := w.ByteAtUTF16(u16, false)
		if got := w.UTF16AtByte(b, false); got != u16 {
			t.Fatalf("UTF-16 %d mapped through byte %d back to %d", u16, b, got)
		}
	}
	if got := w.ByteAtUTF16(5, false); got != len("👩‍💻") {
		t.Fatalf("emoji UTF-16 end mapped to byte %d", got)
	}
}

func TestWordIndexCacheIsRecentAndBounded(t *testing.T) {
	var cache WordIndexCache
	first := cache.Get("alpha")
	_ = cache.Get("中文")
	if got := cache.Get("alpha"); got != first {
		t.Fatal("recent display index was not reused")
	}
	for n := 0; n < 12; n++ {
		_ = cache.Get(strings.Repeat("x", n+1))
	}
	if cache.Len() != cacheEntries {
		t.Fatalf("cache length = %d, want %d", cache.Len(), cacheEntries)
	}
	if cache.Cost() > cacheBudget {
		t.Fatalf("cache cost = %d, exceeds %d byte budget", cache.Cost(), cacheBudget)
	}
}

func TestBigWordAdapterGroupsAdjacentEngineTokens(t *testing.T) {
	w := newWordIndex("one, two")
	if got := w.BigWordCount(); got != 2 {
		t.Fatalf("big word count = %d, want punctuation grouped into two WORDs", got)
	}
	r, ok := w.BigWordEnd(0, false)
	if !ok || w.index.Text()[r.Start:r.End] != "one," {
		t.Fatalf("current WORD = %q, %v", w.index.Text()[r.Start:r.End], ok)
	}
}
