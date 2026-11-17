// Package navigation provides UI-independent text movement and target queries.
// All offsets are half-open UTF-8 byte ranges into the original string.
package navigation

import (
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/go-ego/gse"
	"github.com/go-text/typesetting/segmenter"
)

// Kind classifies a token without assigning it any keybinding semantics.
type Kind uint8

const (
	KindWord Kind = iota
	KindNumber
	KindIdentifier
	KindWhitespace
	KindPunctuation
	KindSymbol
)

// Token is a source-preserving UTF-8 byte range.
type Token struct {
	Start, End int
	Kind       Kind
}

// Range is a half-open UTF-8 byte range.
type Range struct{ Start, End int }

// Span is one word range returned by a segmentation provider.
type Span = Range

// GraphemeBoundaries returns every extended-grapheme boundary in UTF-8 byte
// coordinates, including 0 and len(source). The returned slice is detached.
func GraphemeBoundaries(source string) []int {
	var seg segmenter.Segmenter
	_ = seg.InitWithBytes([]byte(source))
	edges := []int{0}
	it := seg.GraphemeIterator()
	for it.Next() {
		g := it.Grapheme()
		end := g.OffsetInBytes + g.LengthInBytes
		if end > edges[len(edges)-1] {
			edges = append(edges, end)
		}
	}
	if edges[len(edges)-1] != len(source) {
		edges = append(edges, len(source))
	}
	return edges
}

// IsRuneBoundary reports whether offset is a UTF-8 rune boundary in source.
func IsRuneBoundary(source string, offset int) bool {
	return offset >= 0 && offset <= len(source) && (offset == len(source) || utf8.RuneStart(source[offset]))
}

// IsWordRune reports whether r belongs to the editor's ordinary word class.
func IsWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// NextGraphemeBoundary returns the first grapheme boundary strictly after
// offset, clamped to the source length.
func NextGraphemeBoundary(source string, offset int) int {
	offset = min(max(offset, 0), len(source))
	edges := GraphemeBoundaries(source)
	i := sort.Search(len(edges), func(i int) bool { return edges[i] > offset })
	if i == len(edges) {
		return len(source)
	}
	return edges[i]
}

// PreviousGraphemeBoundary returns the last grapheme boundary strictly before
// offset, clamped to the source start.
func PreviousGraphemeBoundary(source string, offset int) int {
	offset = min(max(offset, 0), len(source))
	edges := GraphemeBoundaries(source)
	i := sort.Search(len(edges), func(i int) bool { return edges[i] >= offset })
	if i == 0 {
		return 0
	}
	return edges[i-1]
}

// SnapGraphemeBoundary snaps an arbitrary UTF-8 byte offset to the previous
// boundary for negative direction, or the next boundary otherwise.
func SnapGraphemeBoundary(source string, offset, direction int) int {
	offset = min(max(offset, 0), len(source))
	edges := GraphemeBoundaries(source)
	i := sort.Search(len(edges), func(i int) bool { return edges[i] >= offset })
	if i < len(edges) && edges[i] == offset {
		return offset
	}
	if i == 0 {
		return 0
	}
	if i == len(edges) || direction >= 0 {
		return edges[min(i, len(edges)-1)]
	}
	return edges[i-1]
}

// Provider may be replaced by applications with user dictionaries or another
// language model. It receives each contiguous linguistic run; returning no
// spans keeps the run as one generic Unicode word/number token.
type Provider interface {
	Segment(text []byte) []Span
}

// Direction controls candidate target ordering and filtering.
type Direction int8

const (
	Backward Direction = -1
	Forward  Direction = 1
)

// Target is a candidate word/punctuation position for motion plugins.
type Target struct {
	Start, End int
	Kind       Kind
}

// Index is an immutable tokenization of one source string.
type Index struct {
	text   string
	tokens []Token
}

var defaultOnce sync.Once
var defaultProvider Provider
var asyncWarmOnce sync.Once

// New builds an index with the packaged Chinese dictionary. Generic runs stay
// lazy; the first Han run initializes the embedded dictionary once. No files
// are read at runtime.
func New(text string) *Index {
	return NewWithProvider(text, defaultSegmentationProvider())
}

// NewWithProvider builds an index using provider. A nil provider uses the
// packaged default. Injection keeps dictionary/language policy replaceable.
func NewWithProvider(text string, provider Provider) *Index {
	if provider == nil {
		provider = defaultSegmentationProvider()
	}
	i := &Index{text: text}
	i.tokens = tokenize(text, provider)
	return i
}

// WarmUp loads the embedded dictionary synchronously. Applications may call
// it from a startup goroutine so the first Chinese navigation does not pay the
// dictionary initialization cost. Ordinary ASCII/Latin indexing stays lazy.
func WarmUp() error {
	p, ok := defaultSegmentationProvider().(*lazyGSEProvider)
	if !ok {
		return nil
	}
	return p.load()
}

// WarmUpIfNeeded starts one background dictionary load when text contains Han.
// It is safe to call while opening documents or handling source insertions.
func WarmUpIfNeeded(text string) {
	if ready, _ := DefaultStatus(); ready {
		return
	}
	if containsHanContent(text) {
		asyncWarmOnce.Do(func() { go func() { _ = WarmUp() }() })
	}
}

// containsHanContent skips valid Base64 image payloads, which are opaque
// ASCII data and cannot contain text that benefits from the Han dictionary.
// Searching for the delimiter first lets strings.Index jump across a large
// encoded payload instead of classifying every byte in it.
func containsHanContent(text string) bool {
	if !strings.Contains(text, "data:image/") {
		return containsHanString(text)
	}
	for len(text) > 0 {
		start := strings.Index(text, "data:image/")
		if start < 0 {
			return containsHanString(text)
		}
		if containsHanString(text[:start]) {
			return true
		}
		rest := text[start:]
		comma := strings.IndexByte(rest, ',')
		if comma < 0 || comma > 256 || !strings.Contains(rest[:comma], ";base64") {
			return containsHanString(rest)
		}
		body := rest[comma+1:]
		end := strings.IndexByte(body, ')')
		if i := strings.IndexByte(body, '\r'); i >= 0 && (end < 0 || i < end) {
			end = i
		}
		if i := strings.IndexByte(body, '\n'); i >= 0 && (end < 0 || i < end) {
			end = i
		}
		if end < 0 {
			return containsHanString(rest)
		}
		text = body[end+1:]
	}
	return false
}

// containsHanString skips the common ASCII path without decoding a rune or
// consulting the Unicode tables for every byte. Large pasted payloads are
// usually ASCII and should not pay a full Unicode classification pass just to
// decide whether Chinese navigation needs its dictionary warmed.
func containsHanString(text string) bool {
	for i := 0; i < len(text); {
		if text[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if unicode.Is(unicode.Han, r) {
			return true
		}
		i += size
	}
	return false
}

// DefaultStatus reports whether the packaged dictionary has finished loading
// and any initialization error. An error also means navigation uses the
// deterministic whole-run fallback provider.
func DefaultStatus() (ready bool, err error) {
	p, ok := defaultSegmentationProvider().(*lazyGSEProvider)
	if !ok {
		return true, nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ready, p.err
}

func defaultSegmentationProvider() Provider {
	defaultOnce.Do(func() { defaultProvider = &lazyGSEProvider{} })
	return defaultProvider
}

// Text returns the unchanged input string.
func (i *Index) Text() string { return i.text }

// Tokens returns a copy so callers cannot mutate the immutable index.
func (i *Index) Tokens() []Token { return append([]Token(nil), i.tokens...) }

// TokenCount reports the number of indexed tokens without copying them.
func (i *Index) TokenCount() int { return len(i.tokens) }

// NextStart returns the next non-whitespace token start strictly after off.
func (i *Index) NextStart(off int) (int, bool) {
	n := sort.Search(len(i.tokens), func(n int) bool { return i.tokens[n].Start > off })
	for ; n < len(i.tokens); n++ {
		if i.tokens[n].Kind != KindWhitespace {
			return i.tokens[n].Start, true
		}
	}
	return 0, false
}

// PrevStart returns the previous non-whitespace token start strictly before off.
func (i *Index) PrevStart(off int) (int, bool) {
	n := sort.Search(len(i.tokens), func(n int) bool { return i.tokens[n].Start >= off }) - 1
	for ; n >= 0; n-- {
		t := i.tokens[n]
		if t.Start < off && t.Kind != KindWhitespace {
			return t.Start, true
		}
	}
	return 0, false
}

// WordEnd returns the exclusive end of the token containing off, or the next
// non-whitespace token when off lies in whitespace.
func (i *Index) WordEnd(off int) (int, bool) {
	n := sort.Search(len(i.tokens), func(n int) bool { return i.tokens[n].End > off })
	for ; n < len(i.tokens); n++ {
		if i.tokens[n].Kind != KindWhitespace {
			return i.tokens[n].End, true
		}
	}
	return 0, false
}

// NextEnd returns the exclusive end of the next non-whitespace token after
// the token containing off. If off is in whitespace, it returns the next one.
func (i *Index) NextEnd(off int) (int, bool) {
	n := sort.Search(len(i.tokens), func(n int) bool { return i.tokens[n].End > off })
	if n < len(i.tokens) && i.tokens[n].Start <= off && i.tokens[n].Kind != KindWhitespace {
		n++
	}
	for ; n < len(i.tokens); n++ {
		if i.tokens[n].Kind != KindWhitespace {
			return i.tokens[n].End, true
		}
	}
	return 0, false
}

// WordAt returns the non-whitespace token containing off.
func (i *Index) WordAt(off int) (Range, bool) {
	n := sort.Search(len(i.tokens), func(n int) bool { return i.tokens[n].End > off })
	if n == len(i.tokens) || i.tokens[n].Start > off || i.tokens[n].Kind == KindWhitespace {
		return Range{}, false
	}
	return Range{Start: i.tokens[n].Start, End: i.tokens[n].End}, true
}

// Targets returns tokens of the requested kinds fully inside region. Forward
// targets start at/after region.Start; backward targets end at/before End.
// An empty kinds list includes every non-whitespace token.
func (i *Index) Targets(region Range, direction Direction, kinds ...Kind) []Target {
	allowed := make(map[Kind]bool, len(kinds))
	for _, k := range kinds {
		allowed[k] = true
	}
	if direction == 0 {
		return nil
	}
	include := func(t Token) bool {
		if t.Kind == KindWhitespace || t.Start < region.Start || t.End > region.End {
			return false
		}
		return len(allowed) == 0 || allowed[t.Kind]
	}
	out := make([]Target, 0)
	if direction < 0 {
		n := sort.Search(len(i.tokens), func(n int) bool { return i.tokens[n].Start >= region.End }) - 1
		for ; n >= 0 && i.tokens[n].Start >= region.Start; n-- {
			t := i.tokens[n]
			if include(t) {
				out = append(out, Target{Start: t.Start, End: t.End, Kind: t.Kind})
			}
		}
		return out
	}
	start := sort.Search(len(i.tokens), func(n int) bool { return i.tokens[n].Start >= region.Start })
	for _, t := range i.tokens[start:] {
		if t.Start >= region.End {
			break
		}
		if include(t) {
			out = append(out, Target{Start: t.Start, End: t.End, Kind: t.Kind})
		}
	}
	return out
}

type grapheme struct {
	start, end int
	first      rune
}

func tokenize(text string, provider Provider) []Token {
	if text == "" {
		return nil
	}
	var seg segmenter.Segmenter
	_ = seg.InitWithBytes([]byte(text))
	clusters := make([]grapheme, 0, utf8.RuneCountInString(text))
	it := seg.GraphemeIterator()
	for it.Next() {
		g := it.Grapheme()
		start, end := g.OffsetInBytes, g.OffsetInBytes+g.LengthInBytes
		r, _ := utf8.DecodeRuneInString(text[start:end])
		clusters = append(clusters, grapheme{start: start, end: end, first: r})
	}
	if len(clusters) == 0 {
		return []Token{{Start: 0, End: len(text), Kind: KindSymbol}}
	}
	graphemeBoundaries := map[int]bool{0: true, len(text): true}
	for _, c := range clusters {
		graphemeBoundaries[c.start], graphemeBoundaries[c.end] = true, true
	}
	tokens := make([]Token, 0, len(clusters))
	for n := 0; n < len(clusters); {
		c := clusters[n]
		switch {
		case unicode.IsSpace(c.first):
			end := n + 1
			for end < len(clusters) && unicode.IsSpace(clusters[end].first) {
				end++
			}
			tokens = append(tokens, Token{c.start, clusters[end-1].end, KindWhitespace})
			n = end
		case isWordCluster(c.first):
			end := n + 1
			for end < len(clusters) && isWordCluster(clusters[end].first) {
				end++
			}
			a, z := c.start, clusters[end-1].end
			spans := provider.Segment([]byte(text[a:z]))
			if len(spans) > 0 {
				appendProviderSpans(&tokens, text, a, z, spans, graphemeBoundaries)
			} else {
				tokens = append(tokens, Token{a, z, classifyRun(text[a:z])})
			}
			n = end
		default:
			kind := KindSymbol
			if unicode.IsPunct(c.first) {
				kind = KindPunctuation
			}
			tokens = append(tokens, Token{c.start, c.end, kind})
			n++
		}
	}
	return tokens
}

func appendProviderSpans(tokens *[]Token, text string, start, end int, spans []Span, graphemeBoundaries map[int]bool) {
	last := 0
	for _, span := range spans {
		if span.Start < last || span.End <= span.Start || span.End > end-start ||
			!utf8.ValidString(text[start+span.Start:start+span.End]) ||
			!graphemeBoundaries[start+span.Start] || !graphemeBoundaries[start+span.End] {
			continue
		}
		if span.Start > last {
			*tokens = append(*tokens, Token{start + last, start + span.Start, KindWord})
		}
		*tokens = append(*tokens, Token{start + span.Start, start + span.End, classifyRun(text[start+span.Start : start+span.End])})
		last = span.End
	}
	if last < end-start {
		*tokens = append(*tokens, Token{start + last, end, classifyRun(text[start+last : end])})
	}
}

func classifyRun(s string) Kind {
	allDigits := s != ""
	hasUnderscore, hasAlpha, hasDigit := false, false, false
	for _, r := range s {
		switch {
		case r == '_':
			hasUnderscore = true
			allDigits = false
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsLetter(r):
			allDigits = false
			hasAlpha = true
		default:
			allDigits = false
		}
	}
	if allDigits {
		return KindNumber
	}
	if hasUnderscore || hasAlpha && hasDigit {
		return KindIdentifier
	}
	return KindWord
}

func isWordCluster(r rune) bool { return IsWordRune(r) }

type lazyGSEProvider struct {
	once     sync.Once
	mu       sync.RWMutex
	provider Provider
	ready    bool
	err      error
}

func (p *lazyGSEProvider) load() error {
	p.once.Do(func() {
		provider, err := newGSEProvider()
		p.mu.Lock()
		p.provider, p.err, p.ready = provider, err, true
		p.mu.Unlock()
	})
	p.mu.RLock()
	err := p.err
	p.mu.RUnlock()
	return err
}

func (p *lazyGSEProvider) Segment(text []byte) []Span {
	if !containsHan(text) {
		return nil
	}
	p.load()
	p.mu.RLock()
	provider := p.provider
	p.mu.RUnlock()
	return provider.Segment(text)
}

type gseProvider struct{ seg *gse.Segmenter }

func newGSEProvider() (Provider, error) {
	seg := &gse.Segmenter{SkipLog: true, SkipSubSeg: true}
	if err := seg.LoadDictEmbed("zh_s"); err != nil {
		return fallbackProvider{}, err
	}
	return gseProvider{seg: seg}, nil
}

func (p gseProvider) Segment(text []byte) []Span {
	if !containsHan(text) {
		return nil
	}
	segments := p.seg.Segment(text)
	spans := make([]Span, 0, len(segments))
	for n := range segments {
		spans = append(spans, Span{Start: segments[n].Start(), End: segments[n].End()})
	}
	return spans
}

func containsHan(text []byte) bool {
	for len(text) > 0 {
		r, size := utf8.DecodeRune(text)
		if unicode.Is(unicode.Han, r) {
			return true
		}
		text = text[size:]
	}
	return false
}

// fallbackProvider preserves a run as one token if embedded dictionary setup
// ever fails, so navigation remains deterministic and source-safe offline.
type fallbackProvider struct{}

func (fallbackProvider) Segment(text []byte) []Span {
	if len(text) == 0 {
		return nil
	}
	return []Span{{Start: 0, End: len(text)}}
}
