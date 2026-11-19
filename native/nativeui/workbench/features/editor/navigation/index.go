// Package navigation adapts the standalone text-navigation engine to the
// UTF-16 coordinates used by MyGo's text layout. Tokenization and word
// semantics remain owned by muse-editor/text/navigation.
package navigation

import (
	"container/list"
	"sort"
	"strings"
	"unicode/utf8"

	textnav "github.com/wheregrassdreams/muse-editor/text/navigation"
)

const (
	cacheEntries = 8
	cacheBudget  = 256 << 10
)

// WordIndex holds an engine index plus the coordinate map between its UTF-8
// byte offsets and MyGo's UTF-16 display offsets.
type WordIndex struct {
	index    *textnav.Index
	bytes    []int
	utf16    []int
	bigWords []textnav.Range
	textCost int
}

func newWordIndex(text string) *WordIndex {
	text = strings.Clone(text)
	w := &WordIndex{index: textnav.New(text), textCost: len(text)}
	w.bytes = make([]int, 0, utf8.RuneCountInString(text)+1)
	w.utf16 = make([]int, 0, cap(w.bytes))
	w.bytes = append(w.bytes, 0)
	w.utf16 = append(w.utf16, 0)
	for byteOff, u16 := 0, 0; byteOff < len(text); {
		r, size := utf8.DecodeRuneInString(text[byteOff:])
		byteOff += size
		if r > 0xffff {
			u16 += 2
		} else {
			u16++
		}
		w.bytes = append(w.bytes, byteOff)
		w.utf16 = append(w.utf16, u16)
	}
	return w
}

func (w *WordIndex) NextStart(off int) (int, bool)        { return w.index.NextStart(off) }
func (w *WordIndex) PrevStart(off int) (int, bool)        { return w.index.PrevStart(off) }
func (w *WordIndex) WordEnd(off int) (int, bool)          { return w.index.WordEnd(off) }
func (w *WordIndex) NextEnd(off int) (int, bool)          { return w.index.NextEnd(off) }
func (w *WordIndex) WordAt(off int) (textnav.Range, bool) { return w.index.WordAt(off) }
func (w *WordIndex) TokenCount() int                      { return w.index.TokenCount() }

// ByteAtUTF16 maps a UTF-16 display offset to an engine byte offset. If the
// offset splits a surrogate pair, upper chooses the next boundary.
func (w *WordIndex) ByteAtUTF16(off int, upper bool) int {
	off = min(max(off, 0), w.utf16[len(w.utf16)-1])
	i := sort.SearchInts(w.utf16, off)
	if i < len(w.utf16) && w.utf16[i] == off {
		return w.bytes[i]
	}
	if upper {
		return w.bytes[min(i, len(w.bytes)-1)]
	}
	return w.bytes[max(i-1, 0)]
}

// UTF16AtByte maps an engine byte offset to a display offset. If the offset
// is inside a UTF-8 rune, upper chooses the next boundary.
func (w *WordIndex) UTF16AtByte(off int, upper bool) int {
	off = min(max(off, 0), w.bytes[len(w.bytes)-1])
	i := sort.SearchInts(w.bytes, off)
	if i < len(w.bytes) && w.bytes[i] == off {
		return w.utf16[i]
	}
	if upper {
		return w.utf16[min(i, len(w.utf16)-1)]
	}
	return w.utf16[max(i-1, 0)]
}

func (w *WordIndex) ensureBigWords() {
	if w.bigWords != nil {
		return
	}
	for _, token := range w.index.Tokens() {
		if token.Kind == textnav.KindWhitespace {
			continue
		}
		if n := len(w.bigWords); n > 0 && w.bigWords[n-1].End == token.Start {
			w.bigWords[n-1].End = token.End
		} else {
			w.bigWords = append(w.bigWords, textnav.Range{Start: token.Start, End: token.End})
		}
	}
}

func (w *WordIndex) BigWordCount() int {
	w.ensureBigWords()
	return len(w.bigWords)
}

func (w *WordIndex) BigWordStart(off, direction int) (textnav.Range, bool) {
	w.ensureBigWords()
	dir := textnav.Forward
	if direction < 0 {
		dir = textnav.Backward
	}
	return textnav.BigWordStart(w.bigWords, off, dir)
}

func (w *WordIndex) BigWordEnd(off int, next bool) (textnav.Range, bool) {
	w.ensureBigWords()
	return textnav.BigWordEnd(w.bigWords, off, next)
}

// WordIndexCache is a small LRU for recently rendered display strings. It is
// deliberately local to the UI adapter: document navigation policy belongs
// to the engine, while this cache bounds projection churn.
type WordIndexCache struct {
	entries map[string]*list.Element
	lru     list.List
	cost    int
}

type cacheEntry struct {
	key   string
	value *WordIndex
	cost  int
}

func (c *WordIndexCache) Get(text string) *WordIndex {
	if c.entries == nil {
		c.entries = make(map[string]*list.Element)
	}
	if el := c.entries[text]; el != nil {
		c.lru.MoveToFront(el)
		return el.Value.(cacheEntry).value
	}
	w := newWordIndex(text)
	// Account for the source/key, both coordinate tables, the engine tokens,
	// and a worst case BigWord range per token; this stays conservative if the
	// adapter's lazily built Vim projection is used after insertion.
	cost := len(text) + len(w.bytes)*16 + w.index.TokenCount()*40
	if cost > cacheBudget {
		return w
	}
	entry := cacheEntry{key: w.index.Text(), value: w, cost: cost}
	el := c.lru.PushFront(entry)
	c.entries[entry.key] = el
	c.cost += cost
	for c.lru.Len() > cacheEntries || c.cost > cacheBudget {
		old := c.lru.Back()
		removed := old.Value.(cacheEntry)
		delete(c.entries, removed.key)
		c.cost -= removed.cost
		c.lru.Remove(old)
	}
	return w
}

func (c *WordIndexCache) Len() int  { return c.lru.Len() }
func (c *WordIndexCache) Cost() int { return c.cost }

func (c *WordIndexCache) Reset() {
	c.entries = nil
	c.lru.Init()
	c.cost = 0
}
